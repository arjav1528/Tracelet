package trace_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
	"tracelet.local/tracelet/internal/ingestion"
	"tracelet.local/tracelet/testdata/fixtures"
)

func TestSyntheticTraceFixtures(t *testing.T) {
	counts := map[string]int{"simple": 3, "nested": 4, "error": 3, "partial": 1, "late-parent": 1, "multi-service": 3, "cycle": 2, "large": 2001, "sensitive": 3, "paths": 6, "duplicate": 4, "conflicting-duplicate": 4}
	for name, want := range counts {
		t.Run(name, func(t *testing.T) {
			request := fixtures.Request(name)
			if !proto.Equal(request, fixtures.Request(name)) {
				t.Fatal("fixture is nondeterministic")
			}
			got := 0
			for _, resource := range request.ResourceSpans {
				if err := ingestion.ValidateResourceSpans(resource, ingestion.DefaultLimits()); err != nil {
					t.Fatal(err)
				}
				for _, scope := range resource.ScopeSpans {
					for _, span := range scope.Spans {
						got++
						if err := ingestion.ValidateSpan(span, ingestion.DefaultLimits()); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			if got != want {
				t.Fatalf("got %d spans, want %d", got, want)
			}
		})
	}
}

func TestProtocolFixtures(t *testing.T) {
	for _, name := range []string{"valid", "gzip"} {
		t.Run(name, func(t *testing.T) {
			payload := fixtures.Protocol(name)
			body := payload.Body
			if name == "gzip" {
				reader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				body, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					t.Fatal(err)
				}
				if payload.ContentEncoding != "gzip" {
					t.Fatal("gzip header missing")
				}
			}
			var request collectortracepb.ExportTraceServiceRequest
			if err := proto.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(&request, fixtures.Request("simple")) {
				t.Fatal("protocol fixture differs from simple trace")
			}
		})
	}
	malformed := fixtures.Protocol("malformed")
	if err := proto.Unmarshal(malformed.Body, &collectortracepb.ExportTraceServiceRequest{}); err == nil {
		t.Fatal("malformed fixture decoded")
	}
	if len(fixtures.Protocol("oversized").Body) <= 16*1024*1024 {
		t.Fatal("oversized fixture within decoded limit")
	}
	if fixtures.Protocol("unsupported-content-type").ContentType == "application/x-protobuf" {
		t.Fatal("unsupported fixture uses supported type")
	}
}
