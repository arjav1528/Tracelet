package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func descriptorFixture(t *testing.T, role string) Descriptor {
	t.Helper()
	d, err := NewDescriptor("11111111111111111111111111111111", role)
	if err != nil {
		t.Fatal(err)
	}
	d.UIURL = "http://127.0.0.1:4319"
	if role == "run" {
		d.ReceiverEndpoint = "http://127.0.0.1:4320/v1/traces"
	}
	return d
}

func TestDescriptorPrivateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	lock, err := Acquire(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	first := descriptorFixture(t, "run")
	second := descriptorFixture(t, "run")
	if first.InstanceID == second.InstanceID || first.Nonce == second.Nonce || len(first.InstanceID) != 32 || len(first.Nonce) != 64 || first.PID != os.Getpid() {
		t.Fatal("descriptor identity is not unique and correctly sized")
	}
	if err := Publish(path, first); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, first) {
		t.Fatal("descriptor changed on roundtrip")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("descriptor permissions %o", info.Mode().Perm())
		}
	}
	if err := Publish(path, second); err != nil {
		t.Fatal(err)
	}
	got, err = Read(path)
	if err != nil || got.InstanceID != second.InstanceID {
		t.Fatal("replacement failed")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".session-") {
			t.Fatal("temporary descriptor left behind")
		}
	}
}

func TestReadRejectsInvalidJSONAndSize(t *testing.T) {
	d := descriptorFixture(t, "history")
	valid, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"malformed":      []byte(`{"nonce":"PRIVATE",`),
		"unknown":        append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"unknown":"PRIVATE"}`)...),
		"case-duplicate": append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"VERSION":1}`)...),
		"duplicate":      append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"version":1}`)...),
		"trailing":       append(append([]byte(nil), valid...), []byte(` {}`)...),
		"oversized":      bytes.Repeat([]byte(" "), 64*1024+1),
		"null":           []byte("null"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(path); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("invalid/sensitive error: %v", err)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "session.json")
	padded := append(valid, bytes.Repeat([]byte(" "), 64*1024-len(valid))...)
	if err := os.WriteFile(path, padded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		t.Fatalf("exact size boundary: %v", err)
	}
}

func TestDescriptorRejectsInvalidFields(t *testing.T) {
	cases := map[string]func(*Descriptor){
		"version": func(d *Descriptor) { d.Version = 2 }, "pid": func(d *Descriptor) { d.PID = 0 }, "project": func(d *Descriptor) { d.ProjectID = "../invalid" }, "instance": func(d *Descriptor) { d.InstanceID = "bad" }, "nonce": func(d *Descriptor) { d.Nonce = "PRIVATE" }, "role": func(d *Descriptor) { d.Role = "daemon" },
		"ui-domain": func(d *Descriptor) { d.UIURL = "http://localhost:4319" }, "ui-ipv6": func(d *Descriptor) { d.UIURL = "http://[::1]:4319" }, "ui-no-port": func(d *Descriptor) { d.UIURL = "http://127.0.0.1" }, "ui-zero-port": func(d *Descriptor) { d.UIURL = "http://127.0.0.1:0" }, "ui-large-port": func(d *Descriptor) { d.UIURL = "http://127.0.0.1:65536" }, "ui-https": func(d *Descriptor) { d.UIURL = "https://127.0.0.1:4319" }, "ui-user": func(d *Descriptor) { d.UIURL = "http://PRIVATE@127.0.0.1:4319" }, "ui-query": func(d *Descriptor) { d.UIURL += "?token=PRIVATE" }, "ui-empty-query": func(d *Descriptor) { d.UIURL += "?" }, "ui-fragment": func(d *Descriptor) { d.UIURL += "#PRIVATE" }, "ui-path": func(d *Descriptor) { d.UIURL += "/private" }, "ui-encoded-path": func(d *Descriptor) { d.UIURL += "/%2f" },
		"receiver-empty": func(d *Descriptor) { d.ReceiverEndpoint = "" }, "receiver-path": func(d *Descriptor) { d.ReceiverEndpoint = "http://127.0.0.1:4320/private" }, "receiver-query": func(d *Descriptor) { d.ReceiverEndpoint += "?token=PRIVATE" }, "history-receiver": func(d *Descriptor) { d.Role = "history" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			d := descriptorFixture(t, "run")
			change(&d)
			err := Publish(filepath.Join(t.TempDir(), "session.json"), d)
			if err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("invalid/sensitive error: %v", err)
			}
		})
	}
	if _, err := NewDescriptor("bad", "run"); err == nil {
		t.Fatal("accepted bad project")
	}
	if _, err := NewDescriptor(strings.Repeat("1", 32), "bad"); err == nil {
		t.Fatal("accepted bad role")
	}
}

func TestDescriptorReadPrivateRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode/symlink contract")
	}
	path := filepath.Join(t.TempDir(), "session.json")
	d := descriptorFixture(t, "history")
	if err := Publish(path, d); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("read public descriptor")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := path + ".link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(link); err == nil {
		t.Fatal("followed descriptor symlink")
	}
}

func TestRemoveOnlyMatchingInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	d := descriptorFixture(t, "history")
	other := descriptorFixture(t, "history")
	if err := Publish(path, d); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, other.InstanceID); err == nil {
		t.Fatal("did not report different instance")
	}
	if _, err := Read(path); err != nil {
		t.Fatal("removed another instance")
	}
	if err := Remove(path, d.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descriptor remains: %v", err)
	}
	if err := Remove(path, d.InstanceID); err != nil {
		t.Fatalf("remove not idempotent: %v", err)
	}
}

func TestProbeIdentityAndNoNonce(t *testing.T) {
	d := descriptorFixture(t, "history")
	var sawRequest atomic.Bool
	server := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest.Store(true)
		if r.Method != "GET" || r.URL.Path != "/healthz" || r.URL.RawQuery != "" {
			t.Error("unexpected probe route")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tracelet-Nonce") != "" {
			t.Error("probe sent private authorization")
		}
		for _, values := range r.Header {
			for _, v := range values {
				if strings.Contains(v, d.Nonce) {
					t.Error("probe leaked nonce")
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "project_id": d.ProjectID, "instance_id": d.InstanceID})
	}))
	defer server.Close()
	d.UIURL = server.URL
	if err := Probe(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if !sawRequest.Load() {
		t.Fatal("no health request")
	}
}

func TestProbeRejectsReusedPortAndBadProtocol(t *testing.T) {
	for _, name := range []string{"instance", "project", "version", "unknown-field", "malformed", "oversized", "content-type", "status"} {
		t.Run(name, func(t *testing.T) {
			d := descriptorFixture(t, "history")
			server := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				body := map[string]any{"version": 1, "project_id": d.ProjectID, "instance_id": d.InstanceID}
				switch name {
				case "instance":
					body["instance_id"] = strings.Repeat("0", 32)
				case "project":
					body["project_id"] = strings.Repeat("0", 32)
				case "version":
					body["version"] = 2
				case "unknown-field":
					body["nonce"] = "PRIVATE"
				case "malformed":
					fmt.Fprint(w, "PRIVATE {")
					return
				case "oversized":
					fmt.Fprint(w, strings.Repeat(" ", 4097))
					return
				case "content-type":
					w.Header().Set("Content-Type", "text/html")
				case "status":
					w.WriteHeader(503)
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			d.UIURL = server.URL
			if err := Probe(context.Background(), d); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("invalid/sensitive probe error: %v", err)
			}
		})
	}
}

func TestProbeDoesNotFollowRedirect(t *testing.T) {
	var followed atomic.Bool
	target := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
	defer target.Close()
	origin := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL+"/PRIVATE", 302) }))
	defer origin.Close()
	d := descriptorFixture(t, "history")
	d.UIURL = origin.URL
	if err := Probe(context.Background(), d); err == nil {
		t.Fatal("accepted redirect")
	}
	if followed.Load() {
		t.Fatal("followed redirect")
	}
}

func TestProbeHonorsContext(t *testing.T) {
	server := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	d := descriptorFixture(t, "history")
	d.UIURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := Probe(ctx, d); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded timeout, got %v", err)
	}
}

func localServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}}
	server.Start()
	return server
}

func TestPublishConcurrentReadersSeeWholeDescriptor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	first, second := descriptorFixture(t, "history"), descriptorFixture(t, "history")
	if err := Publish(path, first); err != nil {
		t.Fatal(err)
	}
	ready, done := make(chan struct{}), make(chan struct{})
	failures := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(ready)
		for {
			select {
			case <-done:
				return
			default:
			}
			d, err := Read(path)
			if err == nil && d.InstanceID != first.InstanceID && d.InstanceID != second.InstanceID {
				err = errors.New("mixed instance identity")
			}
			if err != nil {
				select {
				case failures <- err:
				default:
				}
				return
			}
		}
	}()
	<-ready
	for i := 0; i < 16; i++ {
		d := first
		if i%2 == 1 {
			d = second
		}
		if err := Publish(path, d); err != nil {
			close(done)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(done)
	wg.Wait()
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	invalid := second
	invalid.UIURL = "http://PRIVATE@127.0.0.1:1234"
	if err := Publish(path, invalid); err == nil {
		t.Fatal("invalid replacement succeeded")
	}
	got, err := Read(path)
	if err != nil || got.InstanceID != second.InstanceID {
		t.Fatal("failed publish damaged prior descriptor")
	}
}
