package ingestion

import (
	"bytes"
	"math"
	"strings"
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func validSpan() *tracepb.Span {
	return &tracepb.Span{TraceId: bytes.Repeat([]byte{1}, 16), SpanId: bytes.Repeat([]byte{2}, 8), StartTimeUnixNano: 1795679780123456789, EndTimeUnixNano: 1795679780123456790}
}

func TestValidateSpanIdentityAndTime(t *testing.T) {
	cases := []struct {
		name   string
		change func(*tracepb.Span)
		valid  bool
	}{
		{"valid", func(*tracepb.Span) {}, true},
		{"short trace", func(s *tracepb.Span) { s.TraceId = []byte{1} }, false},
		{"zero trace", func(s *tracepb.Span) { s.TraceId = make([]byte, 16) }, false},
		{"short span", func(s *tracepb.Span) { s.SpanId = []byte{1} }, false},
		{"zero span", func(s *tracepb.Span) { s.SpanId = make([]byte, 8) }, false},
		{"short parent", func(s *tracepb.Span) { s.ParentSpanId = []byte{1} }, false},
		{"zero parent", func(s *tracepb.Span) { s.ParentSpanId = make([]byte, 8) }, false},
		{"valid parent", func(s *tracepb.Span) { s.ParentSpanId = bytes.Repeat([]byte{3}, 8) }, true},
		{"missing end", func(s *tracepb.Span) { s.EndTimeUnixNano = 0 }, false},
		{"reverse interval", func(s *tracepb.Span) { s.EndTimeUnixNano = s.StartTimeUnixNano - 1 }, false},
		{"zero duration", func(s *tracepb.Span) { s.EndTimeUnixNano = s.StartTimeUnixNano }, true},
		{"unrepresentable start", func(s *tracepb.Span) { s.StartTimeUnixNano = math.MaxUint64 }, false},
		{"unrepresentable end", func(s *tracepb.Span) { s.EndTimeUnixNano = math.MaxUint64 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSpan()
			tc.change(s)
			err := ValidateSpan(s, DefaultLimits())
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
	if err := ValidateSpan(nil, DefaultLimits()); err == nil {
		t.Fatal("nil span accepted")
	}
}

func stringAttribute(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func TestValidateAttributeContainersAndLimits(t *testing.T) {
	limits := DefaultLimits()
	secret := strings.Repeat("SYNTHETIC_SECRET", 400)
	bad := stringAttribute("sensitive", secret)
	cases := []struct {
		name   string
		mutate func(*tracepb.Span)
	}{
		{"span", func(s *tracepb.Span) { s.Attributes = []*commonpb.KeyValue{bad} }},
		{"event", func(s *tracepb.Span) { s.Events = []*tracepb.Span_Event{{Attributes: []*commonpb.KeyValue{bad}}} }},
		{"link", func(s *tracepb.Span) {
			s.Links = []*tracepb.Span_Link{{TraceId: s.TraceId, SpanId: s.SpanId, Attributes: []*commonpb.KeyValue{bad}}}
		}},
		{"key", func(s *tracepb.Span) {
			s.Attributes = []*commonpb.KeyValue{stringAttribute(strings.Repeat("k", limits.KeyBytes+1), "ok")}
		}},
		{"attributes", func(s *tracepb.Span) {
			for i := 0; i <= limits.Attributes; i++ {
				s.Attributes = append(s.Attributes, stringAttribute("key", "value"))
			}
		}},
		{"events", func(s *tracepb.Span) {
			for i := 0; i <= limits.Events; i++ {
				s.Events = append(s.Events, &tracepb.Span_Event{})
			}
		}},
		{"links", func(s *tracepb.Span) {
			for i := 0; i <= limits.Links; i++ {
				s.Links = append(s.Links, &tracepb.Span_Link{TraceId: s.TraceId, SpanId: s.SpanId})
			}
		}},
		{"nested", func(s *tracepb.Span) {
			v := bad.Value
			for i := 0; i <= limits.Depth; i++ {
				v = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{v}}}}
			}
			s.Attributes = []*commonpb.KeyValue{{Key: "nested", Value: v}}
		}},
		{"bytes", func(s *tracepb.Span) {
			s.Attributes = []*commonpb.KeyValue{{Key: "bytes", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: make([]byte, limits.ValueBytes+1)}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSpan()
			tc.mutate(s)
			err := ValidateSpan(s, limits)
			if err == nil {
				t.Fatal("out-of-policy input accepted")
			}
			if strings.Contains(err.Error(), "SYNTHETIC_SECRET") {
				t.Fatal("error exposes attribute value")
			}
		})
	}
	for _, name := range []string{"resource", "scope"} {
		t.Run(name, func(t *testing.T) {
			rs := &tracepb.ResourceSpans{Resource: &resourcepb.Resource{}, ScopeSpans: []*tracepb.ScopeSpans{{Scope: &commonpb.InstrumentationScope{}, Spans: []*tracepb.Span{validSpan()}}}}
			if name == "resource" {
				rs.Resource.Attributes = []*commonpb.KeyValue{bad}
			} else {
				rs.ScopeSpans[0].Scope.Attributes = []*commonpb.KeyValue{bad}
			}
			if err := ValidateResourceSpans(rs, limits); err == nil {
				t.Fatal("invalid shared attributes accepted")
			}
		})
	}
}

func TestValidationDoesNotMutateTelemetry(t *testing.T) {
	span := validSpan()
	span.Attributes = []*commonpb.KeyValue{stringAttribute("safe", "unchanged")}
	before := proto.Clone(span)
	if err := ValidateSpan(span, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, span) {
		t.Fatal("validation changed telemetry")
	}
	// Shared-container validation must not reject an unrelated bad individual span.
	resource := &tracepb.ResourceSpans{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{}}}}}
	if err := ValidateResourceSpans(resource, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
}
