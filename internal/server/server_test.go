package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"tracelet.local/tracelet/internal/config"
	"tracelet.local/tracelet/internal/session"
)

func fixture(t *testing.T) (config.Config, session.Descriptor) {
	t.Helper()
	var cfg config.Config
	cfg.Project.ID = strings.Repeat("1", 32)
	cfg.UI.Bind = "127.0.0.1"
	cfg.Limits.ReadHeaderTimeoutSeconds = 2
	cfg.Limits.ReadTimeoutSeconds = 2
	cfg.Limits.WriteTimeoutSeconds = 2
	cfg.Limits.IdleTimeoutSeconds = 2
	d, err := session.NewDescriptor(cfg.Project.ID, "history")
	if err != nil {
		t.Fatal(err)
	}
	return cfg, d
}

func startTest(t *testing.T) (*Server, session.Descriptor) {
	t.Helper()
	cfg, d := fixture(t)
	s, err := Start(cfg, d, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	d.UIURL = s.URL()
	return s, d
}

func TestHealthPublicIdentityAndHeaders(t *testing.T) {
	s, d := startTest(t)
	if err := session.Probe(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(s.URL() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["version"] != float64(1) || got["project_id"] != d.ProjectID || got["instance_id"] != d.InstanceID {
		t.Fatalf("unexpected public identity: %v", got)
	}
	if strings.Contains(string(body), d.Nonce) || strings.Contains(string(body), "nonce") {
		t.Fatal("health exposed private authorization")
	}
	assertHeaders(t, response)
	if response.Header.Get("Content-Type") != "application/json" {
		t.Fatal("wrong health content type")
	}
}

func TestPageContainsNoPrivateState(t *testing.T) {
	s, d := startTest(t)
	response, err := http.Get(s.URL() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !strings.Contains(string(body), "Tracelet") {
		t.Fatal("missing local history page")
	}
	for _, private := range []string{d.Nonce, d.ProjectID, d.InstanceID, "http://", "https://"} {
		if strings.Contains(string(body), private) {
			t.Fatal("page exposed session state or an external URL")
		}
	}
	assertHeaders(t, response)
	if response.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("wrong page content type")
	}
}

func assertHeaders(t *testing.T, response *http.Response) {
	t.Helper()
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unsafe response headers: %v", response.Header)
	}
}

func TestHostOriginAndFetchMetadata(t *testing.T) {
	s, _ := startTest(t)
	u, err := url.Parse(s.URL())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, host     string
		origins, sites []string
		want           int
	}{
		{name: "originless", want: 200},
		{name: "same-origin", origins: []string{s.URL()}, sites: []string{"same-origin"}, want: 200},
		{name: "browser-originless", sites: []string{"same-origin"}, want: 200},
		{name: "navigation", sites: []string{"none"}, want: 200},
		{name: "rebound-host", host: "evil.example:" + u.Port(), want: 403},
		{name: "wrong-port", host: "127.0.0.1:1", want: 403},
		{name: "localhost", host: "localhost:" + u.Port(), want: 403},
		{name: "hostile-origin", origins: []string{"https://evil.example"}, want: 403},
		{name: "null-origin", origins: []string{"null"}, want: 403},
		{name: "empty-origin", origins: []string{""}, want: 403},
		{name: "duplicate-origin", origins: []string{s.URL(), "https://evil.example"}, want: 403},
		{name: "cross-site", sites: []string{"cross-site"}, want: 403},
		{name: "same-site-other-origin", sites: []string{"same-site"}, want: 403},
		{name: "duplicate-site", sites: []string{"same-origin", "cross-site"}, want: 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, s.URL()+"/healthz", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.host != "" {
				request.Host = tc.host
			}
			for _, origin := range tc.origins {
				request.Header.Add("Origin", origin)
			}
			for _, site := range tc.sites {
				request.Header.Add("Sec-Fetch-Site", site)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", response.StatusCode, tc.want)
			}
			assertHeaders(t, response)
		})
	}
}

func TestUnsupportedRoutesAndMethods(t *testing.T) {
	s, _ := startTest(t)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/healthz", 405}, {"HEAD", "/", 405}, {"OPTIONS", "/", 405}, {"GET", "/api/traces", 404}, {"GET", "/v1/traces", 404},
	} {
		request, err := http.NewRequest(tc.method, s.URL()+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("%s %s: status %d", tc.method, tc.path, response.StatusCode)
		}
		if tc.want == 405 && response.Header.Get("Allow") != "GET" {
			t.Fatal("missing method guidance")
		}
	}
}

func TestListenerHeldAndClose(t *testing.T) {
	s, _ := startTest(t)
	u, err := url.Parse(s.URL())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := net.Listen("tcp4", u.Host); err == nil {
		t.Fatal("server did not retain its listener")
	}
	cfg, d := fixture(t)
	cfg.UI.Port, err = strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := Start(cfg, d, nil, nil); err == nil {
		duplicate.Close(context.Background())
		t.Fatal("second server took occupied port")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", u.Host)
	if err != nil {
		t.Fatalf("close retained port: %v", err)
	}
	listener.Close()
}

func TestStartRejectsUnsafeConfiguration(t *testing.T) {
	for _, name := range []string{"bind", "port", "timeout", "version", "project", "instance"} {
		t.Run(name, func(t *testing.T) {
			cfg, d := fixture(t)
			switch name {
			case "bind":
				cfg.UI.Bind = "0.0.0.0"
			case "port":
				cfg.UI.Port = -1
			case "timeout":
				cfg.Limits.ReadTimeoutSeconds = 0
			case "version":
				d.Version = 2
			case "project":
				d.ProjectID = strings.Repeat("2", 32)
			case "instance":
				d.InstanceID = "invalid"
			}
			if s, err := Start(cfg, d, nil, nil); err == nil {
				s.Close(context.Background())
				t.Fatal("accepted unsafe server configuration")
			}
		})
	}
}
