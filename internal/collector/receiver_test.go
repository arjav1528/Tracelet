package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/arjav1528/Tracelet/internal/ingestion"
	"github.com/arjav1528/Tracelet/testdata/fixtures"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

type sinkFunc func(context.Context, *collectortracepb.ExportTraceServiceRequest) (ingestion.Result, error)

func (f sinkFunc) Ingest(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (ingestion.Result, error) {
	return f(ctx, request)
}

type recordingSink struct {
	mu       sync.Mutex
	requests []*collectortracepb.ExportTraceServiceRequest
}

func (s *recordingSink) Ingest(_ context.Context, request *collectortracepb.ExportTraceServiceRequest) (ingestion.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, proto.Clone(request).(*collectortracepb.ExportTraceServiceRequest))
	return ingestion.Result{Accepted: spanCount(request)}, nil
}
func (s *recordingSink) received() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.requests {
		n += spanCount(r)
	}
	return n
}
func spanCount(request *collectortracepb.ExportTraceServiceRequest) int {
	n := 0
	for _, r := range request.ResourceSpans {
		for _, s := range r.GetScopeSpans() {
			n += len(s.GetSpans())
		}
	}
	return n
}
func testConfig(t *testing.T) config.Config {
	t.Helper()
	path, err := config.Init(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
func startReceiver(t *testing.T, cfg config.Config, sink ingestion.Sink) *Receiver {
	t.Helper()
	r, err := Start(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return r
}
func send(t *testing.T, client *http.Client, request *http.Request) (*http.Response, []byte) {
	t.Helper()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Content-Type") != "application/x-protobuf" {
		t.Fatalf("unexpected content type %q", response.Header.Get("Content-Type"))
	}
	return response, body
}
func post(t *testing.T, endpoint string, payload []byte) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	return request
}
func marshal(t *testing.T, request *collectortracepb.ExportTraceServiceRequest) []byte {
	t.Helper()
	body, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func zipped(t *testing.T, body []byte) []byte {
	t.Helper()
	var result bytes.Buffer
	writer := gzip.NewWriter(&result)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func TestReceiverAcceptsValidEmptyGzipAndPartial(t *testing.T) {
	sink := &recordingSink{}
	receiver := startReceiver(t, testConfig(t), sink)
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	for _, name := range []string{"valid", "gzip", "empty", "partial", "invalid-only", "invalid-shared", "invalid-scope"} {
		t.Run(name, func(t *testing.T) {
			request := fixtures.Request("simple")
			wantAccepted, wantRejected := 3, 0
			switch name {
			case "empty":
				request = &collectortracepb.ExportTraceServiceRequest{}
				wantAccepted = 0
			case "partial":
				request.ResourceSpans[0].ScopeSpans[0].Spans[1].TraceId = make([]byte, 16)
				wantAccepted, wantRejected = 2, 1
			case "invalid-only":
				for _, span := range request.ResourceSpans[0].ScopeSpans[0].Spans {
					span.SpanId = make([]byte, 8)
				}
				wantAccepted, wantRejected = 0, 3
			case "invalid-shared":
				request.ResourceSpans[0].Resource.Attributes[0].Key = strings.Repeat("x", 257)
				wantAccepted, wantRejected = 0, 3
			case "invalid-scope":
				bad := proto.Clone(request.ResourceSpans[0].ScopeSpans[0]).(*tracepb.ScopeSpans)
				bad.SchemaUrl = strings.Repeat("x", 4097)
				request.ResourceSpans[0].ScopeSpans = append(request.ResourceSpans[0].ScopeSpans, bad)
				wantAccepted, wantRejected = 3, 3
			}
			body := marshal(t, request)
			httpRequest := post(t, receiver.Endpoint(), body)
			if name == "gzip" {
				httpRequest = post(t, receiver.Endpoint(), zipped(t, body))
				httpRequest.Header.Set("Content-Encoding", "gzip")
			}
			before := sink.received()
			response, body := send(t, client, httpRequest)
			if response.StatusCode != 200 {
				t.Fatalf("status=%d body=%x", response.StatusCode, body)
			}
			decoded := &collectortracepb.ExportTraceServiceResponse{}
			if err := proto.Unmarshal(body, decoded); err != nil {
				t.Fatal(err)
			}
			if got := decoded.GetPartialSuccess().GetRejectedSpans(); got != int64(wantRejected) {
				t.Fatalf("rejected=%d want%d", got, wantRejected)
			}
			if wantRejected == 0 && decoded.PartialSuccess != nil {
				t.Fatal("full success populated partial_success")
			}
			if wantRejected > 0 && decoded.PartialSuccess.ErrorMessage == "" {
				t.Fatal("partial success has no diagnostic")
			}
			if sink.received()-before != wantAccepted {
				t.Fatalf("stored %d want%d", sink.received()-before, wantAccepted)
			}
		})
	}
	stats := receiver.Snapshot()
	if stats.AcceptedSpans != 11 || stats.RejectedSpans != 10 || stats.LastReceivedAt == "" {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestReceiverRejectsBadInputAndRecovers(t *testing.T) {
	valid := fixtures.Protocol("valid").Body
	cases := []struct {
		name   string
		status int
		change func(*config.Config, *http.Request)
	}{
		{"method", 405, func(_ *config.Config, r *http.Request) { r.Method = http.MethodGet }},
		{"route", 404, func(_ *config.Config, r *http.Request) { r.URL.Path = "/v1/metrics" }},
		{"media", 415, func(_ *config.Config, r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{"encoding", 415, func(_ *config.Config, r *http.Request) { r.Header.Set("Content-Encoding", "br") }},
		{"protobuf", 400, func(_ *config.Config, r *http.Request) {
			r.Body = io.NopCloser(bytes.NewReader([]byte{0xff}))
			r.ContentLength = 1
		}},
		{"gzip", 400, func(_ *config.Config, r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }},
		{"wire", 413, func(c *config.Config, r *http.Request) {
			c.Limits.MaxWireBytes = int64(len(valid) + 1)
			body := make([]byte, len(valid)+2)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}},
		{"chunked-wire", 413, func(c *config.Config, r *http.Request) {
			c.Limits.MaxWireBytes = int64(len(valid) + 1)
			r.Body = io.NopCloser(bytes.NewReader(make([]byte, len(valid)+2)))
			r.ContentLength = -1
		}},
		{"message-allocation", 413, func(c *config.Config, r *http.Request) {
			c.Limits.MaxPendingBytes = 24 << 10
			request := &collectortracepb.ExportTraceServiceRequest{}
			for i := 0; i < 100; i++ {
				request.ResourceSpans = append(request.ResourceSpans, &tracepb.ResourceSpans{})
			}
			body := marshal(t, request)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}},
		{"decoded", 413, func(c *config.Config, r *http.Request) {
			c.Limits.MaxDecodedBytes = int64(len(valid) + 1)
			body := zipped(t, make([]byte, len(valid)+2))
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			r.Header.Set("Content-Encoding", "gzip")
		}},
		{"span-count", 413, func(c *config.Config, r *http.Request) {
			c.Limits.MaxSpansPerBatch = 3
			request := fixtures.Request("nested")
			body := marshal(t, request)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}},
		{"host", 403, func(_ *config.Config, r *http.Request) { r.Host = "evil.invalid" }},
		{"origin", 403, func(_ *config.Config, r *http.Request) { r.Header.Set("Origin", "http://evil.invalid") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(t)
			request := post(t, "http://127.0.0.1/v1/traces", valid)
			test.change(&cfg, request)
			sink := &recordingSink{}
			receiver := startReceiver(t, cfg, sink)
			endpoint := post(t, receiver.Endpoint(), nil)
			request.URL.Scheme, request.URL.Host = endpoint.URL.Scheme, endpoint.URL.Host
			if test.name != "host" {
				request.Host = request.URL.Host
			}
			client := &http.Client{Timeout: 5 * time.Second}
			defer client.CloseIdleConnections()
			response, body := send(t, client, request)
			if response.StatusCode != test.status {
				t.Fatalf("status=%d want%d body=%x", response.StatusCode, test.status, body)
			}
			failure := &statuspb.Status{}
			if err := proto.Unmarshal(body, failure); err != nil || failure.Message == "" {
				t.Fatalf("not a status response: %v %+v", err, failure)
			}
			if sink.received() != 0 {
				t.Fatal("bad input reached storage")
			}
			response, body = send(t, client, post(t, receiver.Endpoint(), valid))
			if response.StatusCode != 200 {
				t.Fatalf("did not recover: %d %x", response.StatusCode, body)
			}
			if sink.received() != 3 {
				t.Fatal("valid request not stored after rejection")
			}
		})
	}
}

func TestReceiverBackpressureAndCommitBeforeAck(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	sink := sinkFunc(func(ctx context.Context, r *collectortracepb.ExportTraceServiceRequest) (ingestion.Result, error) {
		once.Do(func() { close(started) })
		select {
		case <-release:
			return ingestion.Result{Accepted: spanCount(r)}, nil
		case <-ctx.Done():
			return ingestion.Result{}, ctx.Err()
		}
	})
	cfg := testConfig(t)
	cfg.Limits.MaxConcurrentRequests = 1
	receiver := startReceiver(t, cfg, sink)
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	first := make(chan *http.Response, 1)
	request := post(t, receiver.Endpoint(), fixtures.Protocol("valid").Body)
	errorsCh := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if err != nil {
			errorsCh <- err
			return
		}
		first <- response
	}()
	<-started
	select {
	case response := <-first:
		response.Body.Close()
		t.Fatal("acknowledged before commit")
	case err := <-errorsCh:
		t.Fatal(err)
	default:
	}
	response, _ := send(t, client, post(t, receiver.Endpoint(), fixtures.Protocol("valid").Body))
	if response.StatusCode != 503 || response.Header.Get("Retry-After") == "" {
		t.Fatalf("expected retryable overload: %d", response.StatusCode)
	}
	close(release)
	select {
	case response := <-first:
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("commit response=%d", response.StatusCode)
		}
	case err := <-errorsCh:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("commit did not acknowledge")
	}
	if stats := receiver.Snapshot(); stats.OverloadedRequests != 1 || stats.AcceptedSpans != 3 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestReceiverMasksWriterFailureAndCancellation(t *testing.T) {
	sink := sinkFunc(func(context.Context, *collectortracepb.ExportTraceServiceRequest) (ingestion.Result, error) {
		return ingestion.Result{}, errors.New("synthetic private database detail")
	})
	receiver := startReceiver(t, testConfig(t), sink)
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	response, body := send(t, client, post(t, receiver.Endpoint(), fixtures.Protocol("valid").Body))
	if response.StatusCode != 503 || bytes.Contains(body, []byte("private")) {
		t.Fatalf("unsafe failure: %d %q", response.StatusCode, body)
	}
	canceled := make(chan struct{})
	started := make(chan struct{})
	cancelSink := sinkFunc(func(ctx context.Context, _ *collectortracepb.ExportTraceServiceRequest) (ingestion.Result, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ingestion.Result{}, ctx.Err()
	})
	other := startReceiver(t, testConfig(t), cancelSink)
	ctx, cancel := context.WithCancel(context.Background())
	request := post(t, other.Endpoint(), fixtures.Protocol("valid").Body).WithContext(ctx)
	done := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if response != nil {
			response.Body.Close()
		}
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("client cancellation=%v", err)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("sink context not canceled")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := other.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	if stats := other.Snapshot(); stats.Queue.PendingBatches != 0 || stats.Queue.PendingBytes != 0 || stats.AcceptedSpans != 0 {
		t.Fatalf("cancellation leaked: %+v", stats)
	}
}

func TestReceiverRefusesInvalidStartup(t *testing.T) {
	sink := &recordingSink{}
	cfg := testConfig(t)
	cfg.Collector.Bind = "0.0.0.0"
	if r, err := Start(cfg, sink); err == nil {
		r.Close(context.Background())
		t.Fatal("bound non-loopback")
	}
	cfg = testConfig(t)
	cfg.Limits.MaxConcurrentRequests = 0
	if r, err := Start(cfg, sink); err == nil {
		r.Close(context.Background())
		t.Fatal("accepted zero concurrency")
	}
}
