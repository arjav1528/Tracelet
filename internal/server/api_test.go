package server

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"tracelet.local/tracelet/internal/collector"
	"tracelet.local/tracelet/internal/config"
	"tracelet.local/tracelet/internal/session"
	"tracelet.local/tracelet/internal/storage"
	"tracelet.local/tracelet/internal/trace"
	"tracelet.local/tracelet/testdata/fixtures"
)

var updateAPIFixtures = flag.Bool("update-api-fixtures", false, "rewrite synthetic API response fixtures")

type apiFixture struct {
	server     *Server
	store      *storage.Store
	descriptor session.Descriptor
	config     config.Config
	receiver   *collector.Receiver
}

func newAPI(t *testing.T, withReceiver bool) apiFixture {
	t.Helper()
	root := t.TempDir()
	path, err := config.Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage.Path = filepath.Join(root, "PRIVATE_DATABASE_PATH", "traces.db")
	cfg.Privacy.RedactAttributeKeys = append(cfg.Privacy.RedactAttributeKeys, "PRIVATE_PATTERN")
	lock, err := session.Acquire(filepath.Join(root, "owner.lock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	store, err := storage.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	role := "history"
	var receiver *collector.Receiver
	if withReceiver {
		role = "run"
		receiver, err = collector.Start(cfg, store)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := receiver.Close(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	d, err := session.NewDescriptor(cfg.Project.ID, role)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Start(cfg, d, store, receiver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	d.UIURL = s.URL()
	if receiver != nil {
		d.ReceiverEndpoint = receiver.Endpoint()
	}
	return apiFixture{server: s, store: store, descriptor: d, config: cfg, receiver: receiver}
}

func (f apiFixture) get(t *testing.T, path string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, f.server.URL()+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Tracelet-Token", f.descriptor.Nonce)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	assertHeaders(t, response)
	if response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("non-JSON API response: %s", response.Header.Get("Content-Type"))
	}
	return response.StatusCode, data
}

func ingestAPI(t *testing.T, f apiFixture, request *collectortracepb.ExportTraceServiceRequest) {
	t.Helper()
	if _, err := f.store.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func requireAPIError(t *testing.T, body []byte, code string) {
	t.Helper()
	var value struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("invalid error JSON: %s", body)
	}
	if value.Error.Code != code || value.Error.Message == "" {
		t.Fatalf("error body %s, want %s", body, code)
	}
	for _, secret := range []string{"PRIVATE", fixtures.Secret, "SELECT ", "sqlite", "nonce"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("unsafe API error: %s", body)
		}
	}
}

func TestAPIRequiresTokenEvenWithSameOrigin(t *testing.T) {
	f := newAPI(t, false)
	for _, tc := range []struct {
		name                   string
		tokens, origins, sites []string
		host                   string
		want                   int
		code                   string
	}{
		{name: "native", tokens: []string{f.descriptor.Nonce}, want: 200},
		{name: "browser-originless", tokens: []string{f.descriptor.Nonce}, sites: []string{"same-origin"}, want: 200},
		{name: "browser-origin", tokens: []string{f.descriptor.Nonce}, origins: []string{f.server.URL()}, sites: []string{"same-origin"}, want: 200},
		{name: "no-token", want: 401, code: "unauthorized"},
		{name: "spoofed-origin-no-token", origins: []string{f.server.URL()}, want: 401, code: "unauthorized"},
		{name: "wrong-token", tokens: []string{strings.Repeat("0", 64)}, want: 401, code: "unauthorized"},
		{name: "duplicate-token", tokens: []string{f.descriptor.Nonce, f.descriptor.Nonce}, want: 401, code: "unauthorized"},
		{name: "hostile-origin", tokens: []string{f.descriptor.Nonce}, origins: []string{"https://PRIVATE.invalid"}, want: 403, code: "forbidden"},
		{name: "null-origin", tokens: []string{f.descriptor.Nonce}, origins: []string{"null"}, want: 403, code: "forbidden"},
		{name: "cross-site", tokens: []string{f.descriptor.Nonce}, sites: []string{"cross-site"}, want: 403, code: "forbidden"},
		{name: "rebound-host", tokens: []string{f.descriptor.Nonce}, host: "PRIVATE.invalid:1234", want: 403, code: "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, f.server.URL()+"/api/v1/status", nil)
			if tc.host != "" {
				request.Host = tc.host
			}
			for _, v := range tc.tokens {
				request.Header.Add("X-Tracelet-Token", v)
			}
			for _, v := range tc.origins {
				request.Header.Add("Origin", v)
			}
			for _, v := range tc.sites {
				request.Header.Add("Sec-Fetch-Site", v)
			}
			response := httptest.NewRecorder()
			f.server.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if tc.code != "" {
				requireAPIError(t, response.Body.Bytes(), tc.code)
			}
			assertHeaders(t, response.Result())
		})
	}
}

func TestAPITraceDetailsKeepExactTimesAndRedaction(t *testing.T) {
	f := newAPI(t, false)
	ingestAPI(t, f, fixtures.Request("sensitive"))
	code, data := f.get(t, "/api/v1/traces")
	var page trace.TracePage
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Traces) != 1 {
		t.Fatalf("trace list %d %s", code, data)
	}
	if page.Traces[0].StartTimeUnixNano != strconv.FormatUint(fixtures.StartTimeUnixNano+100, 10) {
		t.Fatal("timestamp lost precision")
	}
	id := page.Traces[0].TraceID
	code, data = f.get(t, "/api/v1/traces/"+id)
	var detail trace.TraceDetail
	if code != 200 || json.Unmarshal(data, &detail) != nil || len(detail.Spans) != 3 || detail.Summary.TraceID != id {
		t.Fatalf("trace detail %d %s", code, data)
	}
	code, data = f.get(t, "/api/v1/traces/"+id+"/spans/0000000000000002")
	var telemetry trace.Telemetry
	if code != 200 || json.Unmarshal(data, &telemetry) != nil {
		t.Fatalf("span detail %d %s", code, data)
	}
	span := telemetry.Request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	if span.StartTimeUnixNano != fixtures.StartTimeUnixNano+200 || len(span.Events) != 1 || len(span.Links) != 1 || len(span.Attributes) == 0 {
		t.Fatal("span detail lost OTLP fields")
	}
	if strings.Contains(string(data), fixtures.Secret) || strings.Contains(string(data), f.descriptor.Nonce) {
		t.Fatal("span response leaked a secret")
	}
}

func TestAPITraceListPaginationSearchAndRevision(t *testing.T) {
	f := newAPI(t, false)
	for i := uint64(1); i <= 3; i++ {
		request := fixtures.Request("late-parent")
		span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
		binary.BigEndian.PutUint64(span.TraceId[8:], i)
		if i == 2 {
			span.Name = "literal ' OR 1=1 --"
		}
		ingestAPI(t, f, request)
	}
	code, data := f.get(t, "/api/v1/traces?limit=1")
	var first trace.TracePage
	if code != 200 || json.Unmarshal(data, &first) != nil || first.NextCursor == "" {
		t.Fatalf("first page %d %s", code, data)
	}
	code, data = f.get(t, "/api/v1/traces?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	var second trace.TracePage
	if code != 200 || json.Unmarshal(data, &second) != nil || second.Traces[0].TraceID == first.Traces[0].TraceID {
		t.Fatalf("second page %d %s", code, data)
	}
	code, data = f.get(t, "/api/v1/traces?q="+url.QueryEscape("' OR 1=1 --"))
	var found trace.TracePage
	if code != 200 || json.Unmarshal(data, &found) != nil || len(found.Traces) != 1 || found.Traces[0].TraceID != "00000000000000000000000000000002" {
		t.Fatalf("literal search %d %s", code, data)
	}
	request := fixtures.Request("late-parent")
	binary.BigEndian.PutUint64(request.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId[8:], 4)
	ingestAPI(t, f, request)
	code, data = f.get(t, "/api/v1/traces?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	if code != 409 {
		t.Fatalf("stale page status %d", code)
	}
	requireAPIError(t, data, "revision_changed")
}

func TestAPITimelinePaginationRequiresSameTraceRevision(t *testing.T) {
	f := newAPI(t, false)
	request := fixtures.Request("simple")
	ingestAPI(t, f, request)
	id := hex.EncodeToString(request.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
	code, data := f.get(t, "/api/v1/traces/"+id+"/spans?limit=1")
	var first trace.SpanPage
	if code != 200 || json.Unmarshal(data, &first) != nil || first.SpanCount != 3 || first.NextCursor == "" {
		t.Fatalf("timeline %d %s", code, data)
	}
	code, data = f.get(t, "/api/v1/traces/"+id+"/spans?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	var second trace.SpanPage
	if code != 200 || json.Unmarshal(data, &second) != nil || second.Spans[0].SpanID == first.Spans[0].SpanID {
		t.Fatalf("timeline page %d %s", code, data)
	}
	late := fixtures.Request("partial")
	late.ResourceSpans[0].ScopeSpans[0].Spans[0].SpanId[7] = 9
	ingestAPI(t, f, late)
	code, data = f.get(t, "/api/v1/traces/"+id+"/spans?limit=1&cursor="+url.QueryEscape(first.NextCursor))
	if code != 409 {
		t.Fatalf("stale timeline status %d", code)
	}
	requireAPIError(t, data, "revision_changed")
}

func TestAPIQueryValidationAndSafeErrors(t *testing.T) {
	f := newAPI(t, false)
	ingestAPI(t, f, fixtures.Request("simple"))
	id := "00000000000000000000000000000001"
	for _, path := range []string{
		"/api/v1/status?extra=PRIVATE", "/api/v1/traces?limit=0", "/api/v1/traces?limit=201", "/api/v1/traces?limit=1&limit=2", "/api/v1/traces?limit=", "/api/v1/traces?sort=PRIVATE", "/api/v1/traces?q=%00", "/api/v1/traces?q=%FF", "/api/v1/traces?q=%zz", "/api/v1/traces?q=" + strings.Repeat("x", 257), "/api/v1/traces?cursor=" + strings.Repeat("x", 4097), "/api/v1/traces?q=" + strings.Repeat("x", 8193), "/api/v1/traces/invalid", "/api/v1/traces/" + id + "?extra=PRIVATE", "/api/v1/traces/" + id + "/spans?limit=2001", "/api/v1/traces/" + id + "/spans?sort=newest", "/api/v1/traces/" + id + "/spans/invalid",
	} {
		code, data := f.get(t, path)
		if code != 400 {
			t.Fatalf("invalid query accepted (%d): %.100s", code, path)
		}
		requireAPIError(t, data, "invalid_query")
	}
	for _, path := range []string{"/api/v1/unknown", "/api/v1/traces/00000000000000000000000000000009", "/api/v1/traces/" + id + "/spans/0000000000000009", "/api/v1/traces/" + id + "/export"} {
		code, data := f.get(t, path)
		if code != 404 {
			t.Fatalf("not found %d: %s", code, path)
		}
		requireAPIError(t, data, "not_found")
	}
	request := httptest.NewRequest(http.MethodPost, f.server.URL()+"/api/v1/traces", nil)
	request.Header.Set("X-Tracelet-Token", f.descriptor.Nonce)
	w := httptest.NewRecorder()
	f.server.ServeHTTP(w, request)
	if w.Code != 405 || w.Header().Get("Allow") != "GET" {
		t.Fatalf("wrong method %d", w.Code)
	}
	requireAPIError(t, w.Body.Bytes(), "method_not_allowed")
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	code, data := f.get(t, "/api/v1/traces")
	if code != 500 {
		t.Fatalf("storage failure %d", code)
	}
	requireAPIError(t, data, "internal_error")
}

func TestAPIStatusHasSafeSettingsAndReceiverState(t *testing.T) {
	for _, running := range []bool{false, true} {
		f := newAPI(t, running)
		ingestAPI(t, f, fixtures.Request("simple"))
		code, data := f.get(t, "/api/v1/status")
		var value struct {
			Project  struct{ ID, Name string }
			Session  struct{ Role string }
			Receiver collector.Stats
			Storage  storage.Usage
			Settings struct {
				RetentionDays int `json:"retention_days"`
			}
		}
		if code != 200 || json.Unmarshal(data, &value) != nil || value.Project.ID != f.config.Project.ID || value.Session.Role != f.descriptor.Role || value.Receiver.Running != running || value.Storage.SpanCount != 3 || value.Settings.RetentionDays != 7 {
			t.Fatalf("status %d %s", code, data)
		}
		for _, private := range []string{f.descriptor.Nonce, f.config.Storage.Path, f.config.Source.WorkspaceRoot, "PRIVATE_PATTERN", "PRIVATE_DATABASE_PATH", "redact_attribute_keys", "nonce"} {
			if strings.Contains(string(data), private) {
				t.Fatalf("status exposed private setting %q", private)
			}
		}
	}
}

func TestAPIRetentionRacingReadsProducesWholeSnapshots(t *testing.T) {
	f := newAPI(t, false)
	ingestAPI(t, f, fixtures.Request("simple"))
	const id = "00000000000000000000000000000001"
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := f.store.Prune(context.Background(), time.Now().Add(8*24*time.Hour))
		if err != nil {
			t.Error(err)
		}
	}()
	for i := 0; i < 12; i++ {
		code, data := f.get(t, "/api/v1/traces/"+id)
		if code == 404 {
			requireAPIError(t, data, "not_found")
			continue
		}
		var detail trace.TraceDetail
		if code != 200 || json.Unmarshal(data, &detail) != nil || len(detail.Spans) != 3 || detail.Summary.SpanCount != 3 {
			t.Fatalf("partial retention snapshot %d %s", code, data)
		}
	}
	wg.Wait()
}

func TestAPIResponseAndConcurrencyBounds(t *testing.T) {
	f := newAPI(t, false)
	for i := 0; i < cap(f.server.slots); i++ {
		f.server.slots <- struct{}{}
	}
	request := httptest.NewRequest(http.MethodGet, f.server.URL()+"/api/v1/status", nil)
	request.Header.Set("X-Tracelet-Token", f.descriptor.Nonce)
	w := httptest.NewRecorder()
	f.server.ServeHTTP(w, request)
	for i := 0; i < cap(f.server.slots); i++ {
		<-f.server.slots
	}
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("unbounded concurrency %d", w.Code)
	}
	requireAPIError(t, w.Body.Bytes(), "unavailable")
	w = httptest.NewRecorder()
	writeJSON(w, map[string]string{"payload": strings.Repeat("x", 32<<20)})
	if w.Code != 413 {
		t.Fatalf("oversized response status %d", w.Code)
	}
	requireAPIError(t, w.Body.Bytes(), "response_too_large")
}

func TestAPIDeadlineAndUnknownRoutesStayProtected(t *testing.T) {
	f := newAPI(t, false)
	for _, path := range []string{"/api/v1", "/api/v1/unknown", "/api/v1/traces/00000000000000000000000000000001/export"} {
		request := httptest.NewRequest(http.MethodGet, f.server.URL()+path, nil)
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, request)
		if w.Code != 401 {
			t.Fatalf("unauthenticated route returned %d", w.Code)
		}
		requireAPIError(t, w.Body.Bytes(), "unauthorized")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, f.server.URL()+"/api/v1/traces", nil).WithContext(ctx)
	request.Header.Set("X-Tracelet-Token", f.descriptor.Nonce)
	w := httptest.NewRecorder()
	f.server.ServeHTTP(w, request)
	if w.Code != 504 {
		t.Fatalf("expired query returned %d", w.Code)
	}
	requireAPIError(t, w.Body.Bytes(), "request_timeout")
}

func TestAPIRepresentativeResponseFixtures(t *testing.T) {
	f := newAPI(t, false)
	ingestAPI(t, f, fixtures.Request("sensitive"))
	for _, sample := range []struct{ file, path string }{
		{"status.json", "/api/v1/status"},
		{"traces.json", "/api/v1/traces"},
		{"trace-detail.json", "/api/v1/traces/00000000000000000000000000000001"},
		{"timeline.json", "/api/v1/traces/00000000000000000000000000000001/spans?limit=1"},
		{"span.json", "/api/v1/traces/00000000000000000000000000000001/spans/0000000000000002"},
		{"error.json", "/api/v1/traces/invalid"},
	} {
		_, data := f.get(t, sample.path)
		var got any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if sample.file == "status.json" {
			// Normalize generated identities and filesystem sizes; retain all schema keys.
			status := got.(map[string]any)
			project := status["project"].(map[string]any)
			project["id"], project["name"] = strings.Repeat("1", 32), "synthetic-project"
			status["session"].(map[string]any)["instance_id"] = strings.Repeat("2", 32)
			status["settings"].(map[string]any)["default_service_name"] = "synthetic-project"
			usage := status["storage"].(map[string]any)
			for key, value := range map[string]float64{"database_bytes": 65536, "wal_bytes": 32768, "shm_bytes": 32768, "journal_bytes": 0, "total_bytes": 131072, "free_page_bytes": 0} {
				if _, exists := usage[key]; !exists {
					t.Fatalf("missing status field %s", key)
				}
				usage[key] = value
			}
		}
		path := filepath.Join("testdata", sample.file)
		if *updateAPIFixtures {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(encoded, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var expected any
		if err := json.Unmarshal(data, &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s response contract changed; review and regenerate synthetic fixtures", sample.file)
		}
	}
}
