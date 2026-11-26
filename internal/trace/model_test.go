package trace

import (
	"bytes"
	"encoding/json"
	"testing"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestTelemetryRoundTripPrecision(t *testing.T) {
	const timestamp = uint64(1795679780123456789)
	request := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		SchemaUrl: "https://example.invalid/resource/1",
		Resource:  &resourcepb.Resource{DroppedAttributesCount: 2, Attributes: []*commonpb.KeyValue{{Key: "resource", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}}}},
		ScopeSpans: []*tracepb.ScopeSpans{{SchemaUrl: "https://example.invalid/scope/1", Scope: &commonpb.InstrumentationScope{Name: "synthetic", Version: "1", DroppedAttributesCount: 3}, Spans: []*tracepb.Span{{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name: "synthetic operation", StartTimeUnixNano: timestamp, EndTimeUnixNano: timestamp + 123,
			TraceState: "synthetic=value", Flags: 257, DroppedAttributesCount: 4, DroppedEventsCount: 5, DroppedLinksCount: 6,
			Attributes: []*commonpb.KeyValue{
				{Key: "large.integer", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 9007199254740993}}},
				{Key: "bytes", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: []byte{0, 1, 128, 255}}}},
				{Key: "array", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 1.25}}, {Value: &commonpb.AnyValue_StringValue{StringValue: "synthetic"}}}}}}},
			},
			Events: []*tracepb.Span_Event{{TimeUnixNano: timestamp + 10, Name: "event", DroppedAttributesCount: 7}},
			Links:  []*tracepb.Span_Link{{TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17}, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 9}, TraceState: "linked=state", Flags: 257, DroppedAttributesCount: 8}},
		}}}},
	}}}
	encoded, err := json.Marshal(Telemetry{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"1795679780123456789"`, `"9007199254740993"`, `"AAGA/w=="`} {
		if !bytes.Contains(encoded, []byte(value)) {
			t.Fatalf("missing lossless JSON value %s in %s", value, encoded)
		}
	}
	var decoded Telemetry
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(request, decoded.Request) {
		t.Fatalf("telemetry changed after JSON roundtrip: got %v", decoded.Request)
	}
	wire, err := proto.Marshal(decoded.Request)
	if err != nil {
		t.Fatal(err)
	}
	restored := &collectortracepb.ExportTraceServiceRequest{}
	if err := proto.Unmarshal(wire, restored); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(request, restored) {
		t.Fatal("telemetry changed after protobuf roundtrip")
	}
}

func TestSummaryTimesAreJSONStrings(t *testing.T) {
	summary := SpanSummary{TraceID: "00000000000000000000000000000001", SpanID: "0000000000000001", StartTimeUnixNano: "1795679780123456789", EndTimeUnixNano: "1795679780123456799", DurationNano: "10"}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"start_time_unix_nano", "end_time_unix_nano", "duration_nano"} {
		if _, ok := fields[key].(string); !ok {
			t.Fatalf("%s is not a decimal string: %v", key, fields[key])
		}
	}
}
