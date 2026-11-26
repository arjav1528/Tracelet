package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInstrumentedRequest(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer downstream.Close()
	handler := orderHandler(provider.Tracer("test"), downstream.URL)
	for _, path := range []string{"/orders/123", "/orders/error"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		want := 200
		if path == "/orders/error" {
			want = 500
		}
		if response.Code != want {
			t.Fatalf("%s: got %d want %d", path, response.Code, want)
		}
	}
	spans := exporter.GetSpans()
	names := map[string]bool{}
	hasSource, hasError := false, false
	for _, span := range spans {
		names[span.Name] = true
		for _, a := range span.Attributes {
			if a.Key == "code.file.path" && a.Value.AsString() != "" {
				hasSource = true
			}
		}
		if span.Status.Code == codes.Error {
			hasError = true
		}
	}
	for _, name := range []string{"GET /orders/{id}", "authenticate", "GetOrder", "payment API"} {
		if !names[name] {
			t.Errorf("missing span %s", name)
		}
	}
	if !hasSource || !hasError {
		t.Fatal("missing source metadata or error status")
	}
}
