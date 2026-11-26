package server

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedOfflineAssets(t *testing.T) {
	s, _ := startTest(t)
	response, err := http.Get(s.URL() + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("bundle scripts blocked by CSP")
	}
	paths := regexp.MustCompile(`(?:src|href)="(/assets/[^" ]+)"`).FindAllStringSubmatch(string(body), -1)
	if len(paths) < 2 {
		t.Fatal("production script and stylesheet not embedded")
	}
	for _, match := range paths {
		asset, err := http.Get(s.URL() + match[1])
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(asset.Body)
		asset.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if asset.StatusCode != 200 || len(data) == 0 {
			t.Fatalf("asset %s unavailable", match[1])
		}
		if strings.HasSuffix(match[1], ".js") && !strings.Contains(asset.Header.Get("Content-Type"), "javascript") {
			t.Fatal("wrong JS content type")
		}
		if strings.HasSuffix(match[1], ".css") && !strings.Contains(asset.Header.Get("Content-Type"), "text/css") {
			t.Fatal("wrong CSS content type")
		}
	}
	for _, path := range []string{"/assets/", "/assets/missing.js", "/package.json", "/src/main.tsx"} {
		response, err := http.Get(s.URL() + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatalf("unexpected exposed path %s: %d", path, response.StatusCode)
		}
	}
}
