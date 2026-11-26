// Package fixtures contains deterministic synthetic telemetry, never captured user data.
package fixtures

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const StartTimeUnixNano uint64 = 1795679780123456789
const Secret = "SYNTHETIC_SECRET_DO_NOT_PUBLISH"

func id(number uint64, size int) []byte {
	result := make([]byte, size)
	binary.BigEndian.PutUint64(result[len(result)-8:], number)
	return result
}

func text(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func integer(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}}}
}

func span(number, parent uint64) *tracepb.Span {
	result := &tracepb.Span{
		TraceId: id(1, 16), SpanId: id(number, 8), Name: "synthetic operation", Kind: tracepb.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: StartTimeUnixNano + number*100, EndTimeUnixNano: StartTimeUnixNano + number*100 + 50,
		TraceState: "synthetic=fixture", Flags: 257,
		Attributes: []*commonpb.KeyValue{text("code.file.path", "/synthetic/workspace/handler.go"), text("code.function.name", "synthetic.Handle"), integer("code.line.number", 42)},
	}
	if parent != 0 {
		result.ParentSpanId = id(parent, 8)
	} else {
		result.Name = "GET /synthetic"
		result.Kind = tracepb.Span_SPAN_KIND_SERVER
		result.EndTimeUnixNano = StartTimeUnixNano + 1000000
	}
	return result
}

func envelope(spans ...*tracepb.Span) *collectortracepb.ExportTraceServiceRequest {
	return &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		SchemaUrl: "https://example.invalid/resource/1.0",
		Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{text("service.name", "synthetic-api")}, DroppedAttributesCount: 1},
		ScopeSpans: []*tracepb.ScopeSpans{{
			SchemaUrl: "https://example.invalid/scope/1.0",
			Scope:     &commonpb.InstrumentationScope{Name: "tracelet.synthetic", Version: "1.0", Attributes: []*commonpb.KeyValue{text("scope.fixture", "synthetic")}, DroppedAttributesCount: 2},
			Spans:     spans,
		}},
	}}}
}

// Request returns a fresh fixture. Unknown names panic so a misspelled test cannot pass silently.
func Request(name string) *collectortracepb.ExportTraceServiceRequest {
	request := envelope(span(1, 0), span(2, 1), span(3, 1))
	resource := request.ResourceSpans[0]
	scope := resource.ScopeSpans[0]
	switch name {
	case "simple":
	case "nested":
		scope.Spans = []*tracepb.Span{span(1, 0), span(2, 1), span(3, 2), span(4, 3)}
	case "error":
		failed := scope.Spans[1]
		failed.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "synthetic dependency failure"}
		failed.Events = []*tracepb.Span_Event{{Name: "exception", TimeUnixNano: failed.StartTimeUnixNano + 1, Attributes: []*commonpb.KeyValue{text("exception.type", "SyntheticError"), text("exception.message", "synthetic failure"), text("exception.stacktrace", "synthetic.Handle at /synthetic/workspace/handler.go:42")}, DroppedAttributesCount: 3}}
	case "partial":
		scope.Spans = []*tracepb.Span{span(2, 1)}
	case "late-parent":
		scope.Spans = []*tracepb.Span{span(1, 0)}
	case "multi-service":
		other := proto.Clone(resource).(*tracepb.ResourceSpans)
		other.Resource.Attributes = []*commonpb.KeyValue{text("service.name", "synthetic-downstream")}
		other.ScopeSpans[0].Spans = []*tracepb.Span{scope.Spans[2]}
		scope.Spans = scope.Spans[:2]
		request.ResourceSpans = append(request.ResourceSpans, other)
	case "cycle":
		scope.Spans = []*tracepb.Span{span(1, 2), span(2, 1)}
	case "large":
		scope.Spans = []*tracepb.Span{span(1, 0)}
		for i := uint64(2); i <= 2001; i++ {
			scope.Spans = append(scope.Spans, span(i, 1))
		}
	case "sensitive":
		resource.Resource.Attributes = append(resource.Resource.Attributes, text("authorization", Secret))
		scope.Scope.Attributes = append(scope.Scope.Attributes, text("token", Secret))
		target := scope.Spans[1]
		target.Attributes = append(target.Attributes, text("http.request.header.authorization", Secret), text("email", "synthetic@example.invalid"), text("db.query.text", Secret), text("url.full", "https://example.invalid/?token="+Secret), text("http.request.body", Secret))
		target.Attributes = append(target.Attributes, &commonpb.KeyValue{Key: "nested", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{text("password", Secret)}}}}}}}}})
		target.Events = []*tracepb.Span_Event{{Name: "synthetic event", TimeUnixNano: target.StartTimeUnixNano, Attributes: []*commonpb.KeyValue{text("cookie", Secret)}}}
		target.Links = []*tracepb.Span_Link{{TraceId: id(2, 16), SpanId: id(1, 8), TraceState: "synthetic=link", Flags: 257, Attributes: []*commonpb.KeyValue{text("api_key", Secret)}, DroppedAttributesCount: 4}}
	case "paths":
		paths := []string{"/synthetic/workspace/handler.go", "/app/handler.go", "handler.go", "../outside.go", `C:\workspace\handler.go`, `\\server\share\handler.go`}
		scope.Spans = nil
		for i, path := range paths {
			item := span(uint64(i+1), 0)
			item.Attributes = []*commonpb.KeyValue{text("code.file.path", path), text("code.function.name", "synthetic.Handle"), integer("code.line.number", 42)}
			scope.Spans = append(scope.Spans, item)
		}
	case "duplicate", "conflicting-duplicate":
		copy := proto.Clone(scope.Spans[1]).(*tracepb.Span)
		if name == "conflicting-duplicate" {
			copy.Name = "synthetic conflicting operation"
		}
		scope.Spans = append(scope.Spans, copy)
	default:
		panic("unknown synthetic trace fixture: " + name)
	}
	return request
}

type ProtocolFixture struct {
	ContentType     string
	ContentEncoding string
	Body            []byte
}

func Protocol(name string) ProtocolFixture {
	body, err := proto.Marshal(Request("simple"))
	if err != nil {
		panic(err)
	}
	fixture := ProtocolFixture{ContentType: "application/x-protobuf", Body: body}
	switch name {
	case "valid":
	case "gzip":
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(body); err != nil {
			panic(err)
		}
		if err := writer.Close(); err != nil {
			panic(err)
		}
		fixture.Body = compressed.Bytes()
		fixture.ContentEncoding = "gzip"
	case "malformed":
		fixture.Body = []byte{0xff}
	case "oversized":
		fixture.Body = make([]byte, 16*1024*1024+1)
	case "unsupported-content-type":
		fixture.ContentType = "text/plain"
	default:
		panic("unknown synthetic protocol fixture: " + name)
	}
	return fixture
}
