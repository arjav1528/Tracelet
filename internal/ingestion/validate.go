package ingestion

import (
	"errors"
	"math"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type Limits struct {
	Attributes int
	KeyBytes   int
	ValueBytes int
	Depth      int
	Events     int
	Links      int
}

func DefaultLimits() Limits {
	return Limits{Attributes: 128, KeyBytes: 256, ValueBytes: 4096, Depth: 8, Events: 128, Links: 128}
}

func (l Limits) validate() error {
	if l.Attributes <= 0 || l.KeyBytes <= 0 || l.ValueBytes <= 0 || l.Depth <= 0 || l.Events <= 0 || l.Links <= 0 {
		return errors.New("validation limits must be positive")
	}
	return nil
}

// ValidateResourceSpans checks shared metadata separately from individual spans.
func ValidateResourceSpans(resource *tracepb.ResourceSpans, limits Limits) error {
	if err := limits.validate(); err != nil {
		return err
	}
	if resource == nil {
		return errors.New("missing resource spans")
	}
	if len(resource.SchemaUrl) > limits.ValueBytes {
		return errors.New("resource schema URL exceeds limit")
	}
	if err := validateAttributes(resource.GetResource().GetAttributes(), limits, 1); err != nil {
		return err
	}
	for _, scope := range resource.ScopeSpans {
		if scope == nil {
			return errors.New("missing scope spans")
		}
		if len(scope.SchemaUrl) > limits.ValueBytes || len(scope.GetScope().GetName()) > limits.ValueBytes || len(scope.GetScope().GetVersion()) > limits.ValueBytes {
			return errors.New("scope metadata exceeds limit")
		}
		if err := validateAttributes(scope.GetScope().GetAttributes(), limits, 1); err != nil {
			return err
		}
	}
	return nil
}

func ValidateSpan(span *tracepb.Span, limits Limits) error {
	if err := limits.validate(); err != nil {
		return err
	}
	if span == nil {
		return errors.New("missing span")
	}
	if !validID(span.TraceId, 16) {
		return errors.New("invalid trace ID")
	}
	if !validID(span.SpanId, 8) {
		return errors.New("invalid span ID")
	}
	if len(span.ParentSpanId) != 0 && !validID(span.ParentSpanId, 8) {
		return errors.New("invalid parent span ID")
	}
	if span.StartTimeUnixNano > math.MaxInt64 || span.EndTimeUnixNano > math.MaxInt64 || span.EndTimeUnixNano == 0 || span.EndTimeUnixNano < span.StartTimeUnixNano {
		return errors.New("invalid span interval")
	}
	if len(span.Name) > limits.ValueBytes || len(span.TraceState) > limits.ValueBytes || len(span.GetStatus().GetMessage()) > limits.ValueBytes {
		return errors.New("span text exceeds limit")
	}
	if err := validateAttributes(span.Attributes, limits, 1); err != nil {
		return err
	}
	if len(span.Events) > limits.Events {
		return errors.New("span event count exceeds limit")
	}
	if len(span.Links) > limits.Links {
		return errors.New("span link count exceeds limit")
	}
	for _, event := range span.Events {
		if event == nil {
			return errors.New("missing span event")
		}
		if event.TimeUnixNano > math.MaxInt64 {
			return errors.New("invalid event timestamp")
		}
		if len(event.Name) > limits.ValueBytes {
			return errors.New("event name exceeds limit")
		}
		if err := validateAttributes(event.Attributes, limits, 1); err != nil {
			return err
		}
	}
	for _, link := range span.Links {
		if link == nil || !validID(link.TraceId, 16) || !validID(link.SpanId, 8) {
			return errors.New("invalid span link identity")
		}
		if len(link.TraceState) > limits.ValueBytes {
			return errors.New("link trace state exceeds limit")
		}
		if err := validateAttributes(link.Attributes, limits, 1); err != nil {
			return err
		}
	}
	return nil
}

func validID(id []byte, size int) bool {
	if len(id) != size {
		return false
	}
	for _, b := range id {
		if b != 0 {
			return true
		}
	}
	return false
}

func validateAttributes(attributes []*commonpb.KeyValue, limits Limits, depth int) error {
	if len(attributes) > limits.Attributes {
		return errors.New("attribute count exceeds limit")
	}
	for _, attribute := range attributes {
		if attribute == nil || attribute.Key == "" {
			return errors.New("missing attribute key")
		}
		if len(attribute.Key) > limits.KeyBytes {
			return errors.New("attribute key exceeds limit")
		}
		if err := validateValue(attribute.Value, limits, depth); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(value *commonpb.AnyValue, limits Limits, depth int) error {
	if depth > limits.Depth {
		return errors.New("attribute nesting exceeds limit")
	}
	if value == nil {
		return nil
	}
	switch v := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		if len(v.StringValue) > limits.ValueBytes {
			return errors.New("attribute string exceeds limit")
		}
	case *commonpb.AnyValue_BytesValue:
		if len(v.BytesValue) > limits.ValueBytes {
			return errors.New("attribute bytes exceed limit")
		}
	case *commonpb.AnyValue_ArrayValue:
		if len(v.ArrayValue.GetValues()) > limits.Attributes {
			return errors.New("attribute array count exceeds limit")
		}
		for _, item := range v.ArrayValue.GetValues() {
			if err := validateValue(item, limits, depth+1); err != nil {
				return err
			}
		}
	case *commonpb.AnyValue_KvlistValue:
		return validateAttributes(v.KvlistValue.GetValues(), limits, depth+1)
	}
	return nil
}
