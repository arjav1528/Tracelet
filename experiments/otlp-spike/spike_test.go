package spike

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func TestGoExporterGzipPersistAndReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "spike.sqlite")
	db := openDB(t, path)
	if _, err := db.Exec(`CREATE TABLE batches (id INTEGER PRIMARY KEY, payload BLOB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	server, received := startReceiver(t, db)
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(server.URL+"/v1/traces"), otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	if err != nil {
		t.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes("https://example.invalid/schema/resource/1", attribute.String("service.name", "synthetic-spike"))),
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Hour)),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	tracer := provider.Tracer("tracelet.spike", trace.WithInstrumentationVersion("1"), trace.WithSchemaURL("https://example.invalid/schema/scope/1"))
	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	remoteParent, err := trace.SpanIDFromHex("1111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	ctx = trace.ContextWithRemoteSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: remoteParent, TraceFlags: trace.FlagsSampled, Remote: true}))
	start := time.Unix(1_700_000_000, 123_456_789)
	ctx, root := tracer.Start(ctx, "synthetic-request", trace.WithTimestamp(start), trace.WithAttributes(attribute.String("code.file.path", "/synthetic/orders.go"), attribute.String("code.function.name", "example.GetOrder"), attribute.Int("code.line.number", 42), attribute.Int64("synthetic.int64", 9_007_199_254_740_993)))
	_, child := tracer.Start(ctx, "synthetic-child", trace.WithTimestamp(start.Add(time.Millisecond)), trace.WithAttributes(attribute.StringSlice("synthetic.tags", []string{"one", "two"})), trace.WithLinks(trace.Link{SpanContext: root.SpanContext(), Attributes: []attribute.KeyValue{attribute.String("synthetic.link", "fixture")}}))
	child.AddEvent("synthetic-event", trace.WithTimestamp(start.Add(2*time.Millisecond)), trace.WithAttributes(attribute.Bool("synthetic.event", true)))
	child.End(trace.WithTimestamp(start.Add(3 * time.Millisecond)))
	root.End(trace.WithTimestamp(start.Add(4 * time.Millisecond)))
	var before int
	if err := db.QueryRow(`SELECT count(*) FROM batches`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("batch exported before explicit shutdown: %d", before)
	}
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	var accepted *collectorpb.ExportTraceServiceRequest
	select {
	case accepted = <-received:
	case <-ctx.Done():
		t.Fatal("shutdown did not flush telemetry")
	}
	server.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	childProcess := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReopenInFreshProcess$")
	childProcess.Env = append(os.Environ(), "TRACELET_SPIKE_REOPEN_DB="+path)
	if output, err := childProcess.CombinedOutput(); err != nil {
		t.Fatalf("fresh process reopen: %v\n%s", err, output)
	}
	db = openDB(t, path)
	defer db.Close()
	var payload []byte
	if err := db.QueryRow(`SELECT payload FROM batches`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	restored := new(collectorpb.ExportTraceServiceRequest)
	if err := proto.Unmarshal(payload, restored); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(accepted, restored) {
		t.Fatal("persisted telemetry lost fields")
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity: %q, %v", integrity, err)
	}
	rs := restored.GetResourceSpans()
	if len(rs) != 1 || rs[0].GetSchemaUrl() != "https://example.invalid/schema/resource/1" || stringAttr(rs[0].GetResource().GetAttributes(), "service.name") != "synthetic-spike" {
		t.Fatal("resource metadata not preserved")
	}
	scopes := rs[0].GetScopeSpans()
	if len(scopes) != 1 || scopes[0].GetScope().GetName() != "tracelet.spike" || scopes[0].GetScope().GetVersion() != "1" || scopes[0].GetSchemaUrl() != "https://example.invalid/schema/scope/1" {
		t.Fatal("scope metadata not preserved")
	}
	spans := scopes[0].GetSpans()
	if len(spans) != 2 {
		t.Fatalf("got %d spans", len(spans))
	}
	byName := map[string]int{}
	for i, s := range spans {
		byName[s.GetName()] = i
		if hex.EncodeToString(s.GetTraceId()) != traceID.String() {
			t.Fatal("trace identity changed")
		}
	}
	rootSpan := spans[byName["synthetic-request"]]
	childSpan := spans[byName["synthetic-child"]]
	if !bytes.Equal(childSpan.GetParentSpanId(), rootSpan.GetSpanId()) || !bytes.Equal(rootSpan.GetParentSpanId(), remoteParent[:]) {
		t.Fatal("parent relationship changed")
	}
	if rootSpan.GetStartTimeUnixNano() != uint64(start.UnixNano()) || rootSpan.GetEndTimeUnixNano() != uint64(start.Add(4*time.Millisecond).UnixNano()) {
		t.Fatal("nanosecond time changed")
	}
	if stringAttr(rootSpan.GetAttributes(), "code.file.path") != "/synthetic/orders.go" || stringAttr(rootSpan.GetAttributes(), "code.function.name") != "example.GetOrder" {
		t.Fatal("source strings missing")
	}
	if intAttr(rootSpan.GetAttributes(), "code.line.number") != 42 || intAttr(rootSpan.GetAttributes(), "synthetic.int64") != 9_007_199_254_740_993 {
		t.Fatal("integer attribute precision lost")
	}
	if len(childSpan.GetEvents()) != 1 || !childSpan.GetEvents()[0].GetAttributes()[0].GetValue().GetBoolValue() || len(childSpan.GetLinks()) != 1 {
		t.Fatal("events or links lost")
	}
	if tags := childSpan.GetAttributes()[0].GetValue().GetArrayValue().GetValues(); len(tags) != 2 || tags[1].GetStringValue() != "two" {
		t.Fatal("array attribute lost")
	}
}

func TestStorageExperiment(t *testing.T) {
	if os.Getenv("TRACELET_SPIKE_BENCH") != "1" {
		t.Skip("set TRACELET_SPIKE_BENCH=1 for the 100,000-span storage comparison")
	}
	for _, layout := range []string{"protobuf", "normalized_attributes"} {
		t.Run(layout, func(t *testing.T) { runStorageExperiment(t, layout) })
	}
}

func stringAttr(attrs []*commonpb.KeyValue, key string) string {
	for _, a := range attrs {
		if a.GetKey() == key {
			return a.GetValue().GetStringValue()
		}
	}
	return ""
}
func intAttr(attrs []*commonpb.KeyValue, key string) int64 {
	for _, a := range attrs {
		if a.GetKey() == key {
			return a.GetValue().GetIntValue()
		}
	}
	return 0
}

func TestReopenInFreshProcess(t *testing.T) {
	path := os.Getenv("TRACELET_SPIKE_REOPEN_DB")
	if path == "" {
		t.Skip("invoked only by the exporter persistence test")
	}
	db := openDB(t, path)
	defer db.Close()
	var payload []byte
	if err := db.QueryRow(`SELECT payload FROM batches`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	request := new(collectorpb.ExportTraceServiceRequest)
	if err := proto.Unmarshal(payload, request); err != nil {
		t.Fatal(err)
	}
	spans := request.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()
	if len(spans) != 2 || hex.EncodeToString(spans[0].GetTraceId()) != "0123456789abcdef0123456789abcdef" {
		t.Fatal("fresh process cannot read the accepted trace")
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("fresh process integrity: %q, %v", integrity, err)
	}
}
