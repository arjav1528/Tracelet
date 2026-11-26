package privacy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arjav1528/Tracelet/testdata/fixtures"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func textAttribute(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func TestRedactionAllContainers(t *testing.T) {
	request := fixtures.Request("sensitive")
	span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Attributes = append(span.Attributes,
		textAttribute("Authorization", fixtures.Secret), textAttribute("proxy.authorization", fixtures.Secret), textAttribute("http.request.body", fixtures.Secret), textAttribute("url.full", fixtures.Secret), textAttribute("db.query.text", fixtures.Secret),
		&commonpb.KeyValue{Key: "safe.int64", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 9007199254740993}}},
		&commonpb.KeyValue{Key: "safe.bytes", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: []byte{0, 1, 255}}}},
	)
	before := proto.Clone(span).(*tracepb.Span)
	report, err := Sanitize(request, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(fixtures.Secret)) {
		t.Fatal("sensitive sentinel survived sanitization")
	}
	if report.Redacted < 10 || len(report.Paths) != report.Redacted {
		t.Fatalf("missing redaction markers: %+v", report)
	}
	for _, path := range report.Paths {
		if strings.Contains(path, fixtures.Secret) || strings.Contains(strings.ToLower(path), "authorization") {
			t.Fatal("marker leaked attribute data")
		}
	}
	if !bytes.Equal(span.TraceId, before.TraceId) || !bytes.Equal(span.SpanId, before.SpanId) || span.StartTimeUnixNano != before.StartTimeUnixNano || span.EndTimeUnixNano != before.EndTimeUnixNano {
		t.Fatal("changed identity or timing")
	}
	attrs := span.Attributes
	if !proto.Equal(attrs[len(attrs)-1], before.Attributes[len(attrs)-1]) || !proto.Equal(attrs[len(attrs)-2], before.Attributes[len(attrs)-2]) {
		t.Fatal("changed unrelated typed attributes")
	}
	second, err := Sanitize(request, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	again, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, again) || second.Redacted != report.Redacted {
		t.Fatal("redaction was not idempotent")
	}
}

func TestGlobAndNestedPaths(t *testing.T) {
	for _, tc := range []struct {
		pattern, key string
		redact       bool
	}{
		{"account.id?", "ACCOUNT.ID世", true}, {"account.id?", "account.id", false}, {"account.id?", "account.idxy", false},
		{"account.*", "account.a/b\nc", true}, {"account.id", "prefix.account.id", false},
		{"[private]", "[private]", true}, {"[private]", "p", false}, {"a.b+", "a.b+", true}, {"a.b+", "axb", false},
	} {
		t.Run(tc.pattern+tc.key, func(t *testing.T) {
			request := fixtures.Request("simple")
			span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
			span.Attributes = []*commonpb.KeyValue{textAttribute(tc.key, "fixture-value")}
			_, err := Sanitize(request, Policy{Patterns: []string{tc.pattern}})
			if err != nil {
				t.Fatal(err)
			}
			got := span.Attributes[0].Value.GetStringValue() == Redacted
			if got != tc.redact {
				t.Fatalf("redacted=%v want %v", got, tc.redact)
			}
		})
	}
	request := fixtures.Request("simple")
	span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Attributes = []*commonpb.KeyValue{{Key: "account", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{textAttribute("id", "fixture-private")}}}}}}}}}}
	report, err := Sanitize(request, Policy{Patterns: []string{"ACCOUNT.ID"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Redacted != 1 {
		t.Fatalf("dotted nested path did not match: %+v", report)
	}
	value := span.Attributes[0].Value.GetArrayValue().Values[0].GetKvlistValue().Values[0].Value.GetStringValue()
	if value != Redacted {
		t.Fatal("nested array attribute not redacted")
	}
}

func TestCustomPolicyCannotDisableSafeDefaults(t *testing.T) {
	request := fixtures.Request("sensitive")
	if _, err := Sanitize(request, Policy{Patterns: []string{"custom.private"}}); err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(fixtures.Secret)) {
		t.Fatal("custom patterns disabled safe defaults")
	}
}

func TestSanitizeFailsClosed(t *testing.T) {
	for name, policy := range map[string]Policy{
		"empty pattern": {Patterns: []string{""}}, "whitespace": {Patterns: []string{" "}}, "too long": {Patterns: []string{strings.Repeat("x", 257)}}, "invalid unicode": {Patterns: []string{string([]byte{255})}}, "too many": {Patterns: make([]string, 129)},
	} {
		t.Run(name, func(t *testing.T) {
			request := fixtures.Request("sensitive")
			report, err := Sanitize(request, policy)
			if err == nil {
				t.Fatal("invalid policy accepted")
			}
			if len(request.ResourceSpans) != 0 || report.Redacted != 0 || len(report.Paths) != 0 {
				t.Fatal("failure retained an unsanitized request or partial report")
			}
			if strings.Contains(err.Error(), fixtures.Secret) {
				t.Fatal("error leaked data")
			}
		})
	}
	request := fixtures.Request("simple")
	cycle := &commonpb.AnyValue{}
	cycle.Value = &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{cycle}}}
	request.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = []*commonpb.KeyValue{{Key: "safe", Value: cycle}}
	if _, err := Sanitize(request, Policy{}); err == nil {
		t.Fatal("unbounded cycle accepted")
	}
	if len(request.ResourceSpans) != 0 {
		t.Fatal("cycle failure did not clear request")
	}
	if _, err := Sanitize(nil, Policy{}); err == nil {
		t.Fatal("nil request accepted")
	}
}

func TestFreeTextIsNotPretendedToBeSanitized(t *testing.T) {
	request := fixtures.Request("simple")
	span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Name = fixtures.Secret
	span.Attributes = []*commonpb.KeyValue{textAttribute("ordinary.message", fixtures.Secret)}
	if _, err := Sanitize(request, Policy{}); err != nil {
		t.Fatal(err)
	}
	if span.Name != fixtures.Secret || span.Attributes[0].Value.GetStringValue() != fixtures.Secret {
		t.Fatal("unexpected free-text transformation")
	}
}

func TestMalformedContainersFailClosed(t *testing.T) {
	request := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{nil}}
	if _, err := Sanitize(request, Policy{}); err == nil || len(request.ResourceSpans) != 0 {
		t.Fatal("malformed envelope was accepted")
	}
}

func TestSanitizeBoundsMarkerReportWithoutSkippingValues(t *testing.T) {
	request := fixtures.Request("simple")
	span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Attributes = nil
	for i := 0; i < maxReportPaths+1; i++ {
		span.Attributes = append(span.Attributes, textAttribute("token", fixtures.Secret))
	}
	report, err := Sanitize(request, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Redacted != maxReportPaths+1 || len(report.Paths) != maxReportPaths || !report.PathsTruncated {
		t.Fatalf("unbounded or inaccurate report: count=%d paths=%d truncated=%v", report.Redacted, len(report.Paths), report.PathsTruncated)
	}
	for _, attribute := range span.Attributes {
		if attribute.Value.GetStringValue() != Redacted {
			t.Fatal("report cap skipped sanitization")
		}
	}
}

func TestSanitizeBoundsUnvalidatedRequestTraversal(t *testing.T) {
	request := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: make([]*tracepb.ResourceSpans, maxNodes+1)}
	if _, err := Sanitize(request, Policy{}); err == nil || !strings.Contains(err.Error(), "traversal budget") {
		t.Fatalf("expected bounded rejection, got %v", err)
	}
	if len(request.ResourceSpans) != 0 {
		t.Fatal("oversized request not cleared")
	}
}

func TestSanitizeRejectsMalformedNestedValues(t *testing.T) {
	for name, value := range map[string]*commonpb.AnyValue{
		"array":  {Value: (*commonpb.AnyValue_ArrayValue)(nil)},
		"kvlist": {Value: (*commonpb.AnyValue_KvlistValue)(nil)},
		"string": {Value: (*commonpb.AnyValue_StringValue)(nil)},
		"bytes":  {Value: (*commonpb.AnyValue_BytesValue)(nil)},
		"int":    {Value: (*commonpb.AnyValue_IntValue)(nil)},
		"double": {Value: (*commonpb.AnyValue_DoubleValue)(nil)},
		"bool":   {Value: (*commonpb.AnyValue_BoolValue)(nil)},
	} {
		t.Run(name, func(t *testing.T) {
			request := fixtures.Request("simple")
			request.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = []*commonpb.KeyValue{{Key: "safe", Value: value}}
			if _, err := Sanitize(request, Policy{}); err == nil || len(request.ResourceSpans) != 0 {
				t.Fatal("malformed nested value accepted")
			}
		})
	}
}
