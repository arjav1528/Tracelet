package collector

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/arjav1528/Tracelet/internal/privacy"
	"github.com/arjav1528/Tracelet/internal/session"
	"github.com/arjav1528/Tracelet/internal/storage"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestPinnedSDKPersistsRedactedTraceBeforeShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := testConfig(t)
	lock, err := session.Acquire(cfg.Storage.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	receiver := startReceiver(t, cfg, store)
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(receiver.Endpoint()), otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	if err != nil {
		t.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", "synthetic-receiver-integration"))),
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Hour)),
	)
	defer provider.Shutdown(context.Background())
	start := time.Unix(1_700_000_000, 123456789)
	_, span := provider.Tracer("tracelet-receiver-test").Start(ctx, "synthetic-operation",
		trace.WithTimestamp(start), trace.WithAttributes(attribute.String("api.token", "synthetic-secret-sentinel"), attribute.String("code.function.name", "syntheticHandler")),
	)
	identity := span.SpanContext()
	span.End(trace.WithTimestamp(start.Add(1234567 * time.Nanosecond)))
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if stats := receiver.Snapshot(); stats.AcceptedSpans != 1 || stats.RejectedSpans != 0 {
		t.Fatalf("SDK export was not committed: %+v", stats)
	}
	if err := receiver.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var traceID, spanID string
	var payload []byte
	if err := db.QueryRowContext(ctx, "SELECT trace_id,span_id,payload FROM spans").Scan(&traceID, &spanID, &payload); err != nil {
		t.Fatal(err)
	}
	if traceID != identity.TraceID().String() || spanID != identity.SpanID().String() {
		t.Fatal("SDK identity changed during persistence")
	}
	if bytes.Contains(payload, []byte("synthetic-secret-sentinel")) {
		t.Fatal("unredacted SDK attribute persisted")
	}
	stored := &tracepb.Span{}
	if err := proto.Unmarshal(payload, stored); err != nil {
		t.Fatal(err)
	}
	if stored.StartTimeUnixNano != uint64(start.UnixNano()) || stored.EndTimeUnixNano-stored.StartTimeUnixNano != 1234567 {
		t.Fatal("SDK nanosecond timestamps changed")
	}
	attributes := map[string]string{}
	for _, value := range stored.Attributes {
		attributes[value.Key] = value.GetValue().GetStringValue()
	}
	if attributes["api.token"] != privacy.Redacted || attributes["code.function.name"] != "syntheticHandler" {
		t.Fatalf("unexpected attributes: %+v", attributes)
	}
}
