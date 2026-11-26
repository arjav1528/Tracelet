// Package privacy removes configured sensitive attribute values before persistence.
package privacy

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

const Redacted = "[REDACTED]"
const maxDepth = 64
const maxNodes = 4_000_000
const maxReportPaths = 1024

// Patterns extend the conservative defaults; they never disable those defaults.
type Policy struct{ Patterns []string }

type Report struct {
	Redacted       int      `json:"redacted"`
	Paths          []string `json:"paths"`
	PathsTruncated bool     `json:"paths_truncated"`
}

func DefaultPolicy() Policy {
	return Policy{Patterns: []string{
		"*authorization*", "*cookie*", "*password*", "*passwd*", "*token*", "*secret*",
		"*api_key*", "*apikey*", "*email*", "*body*", "*payload*",
		"url.full", "url.query", "http.url", "db.statement", "db.query.text",
	}}
}

// Sanitize mutates owned telemetry. On any failure it clears the request; callers must reject it.
func Sanitize(request *collectortracepb.ExportTraceServiceRequest, policy Policy) (report Report, err error) {
	if request == nil {
		return report, errors.New("missing telemetry request")
	}
	defer func() {
		if err != nil {
			request.Reset()
			report = Report{}
		}
	}()
	patterns, err := compilePatterns(policy)
	if err != nil {
		return report, err
	}
	walker := sanitizer{patterns: patterns, remaining: maxNodes}
	if err = walker.take(len(request.ResourceSpans)); err != nil {
		return report, err
	}
	for ri, resource := range request.ResourceSpans {
		if resource == nil {
			return report, errors.New("missing resource spans")
		}
		base := fmt.Sprintf("resource_spans[%d]", ri)
		if err = walker.attributes(resource.GetResource().GetAttributes(), "", base+".resource.attributes", 1); err != nil {
			return report, err
		}
		if err = walker.take(len(resource.ScopeSpans)); err != nil {
			return report, err
		}
		for si, scope := range resource.ScopeSpans {
			if scope == nil {
				return report, errors.New("missing scope spans")
			}
			scopePath := fmt.Sprintf("%s.scope_spans[%d]", base, si)
			if err = walker.attributes(scope.GetScope().GetAttributes(), "", scopePath+".scope.attributes", 1); err != nil {
				return report, err
			}
			if err = walker.take(len(scope.Spans)); err != nil {
				return report, err
			}
			for spi, span := range scope.Spans {
				if span == nil {
					return report, errors.New("missing span")
				}
				spanPath := fmt.Sprintf("%s.spans[%d]", scopePath, spi)
				if err = walker.attributes(span.Attributes, "", spanPath+".attributes", 1); err != nil {
					return report, err
				}
				if err = walker.take(len(span.Events) + len(span.Links)); err != nil {
					return report, err
				}
				for ei, event := range span.Events {
					if event == nil {
						return report, errors.New("missing event")
					}
					if err = walker.attributes(event.Attributes, "", fmt.Sprintf("%s.events[%d].attributes", spanPath, ei), 1); err != nil {
						return report, err
					}
				}
				for li, link := range span.Links {
					if link == nil {
						return report, errors.New("missing link")
					}
					if err = walker.attributes(link.Attributes, "", fmt.Sprintf("%s.links[%d].attributes", spanPath, li), 1); err != nil {
						return report, err
					}
				}
			}
		}
	}
	return walker.report, nil
}

func compilePatterns(policy Policy) ([]*regexp.Regexp, error) {
	if len(policy.Patterns) > 128 {
		return nil, errors.New("redaction policy exceeds 128 patterns")
	}
	patterns := append(DefaultPolicy().Patterns, policy.Patterns...)
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	seen := make(map[string]bool, len(patterns))
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" || len(pattern) > 256 || !utf8.ValidString(pattern) {
			return nil, errors.New("invalid redaction pattern: require 1–256 UTF-8 bytes")
		}
		key := strings.ToLower(pattern)
		if seen[key] {
			continue
		}
		seen[key] = true
		expression := strings.NewReplacer(`\*`, ".*", `\?`, ".").Replace(regexp.QuoteMeta(pattern))
		matcher, err := regexp.Compile("(?is)^(?:" + expression + ")$")
		if err != nil {
			return nil, errors.New("invalid redaction pattern")
		}
		compiled = append(compiled, matcher)
	}
	return compiled, nil
}

type sanitizer struct {
	patterns  []*regexp.Regexp
	remaining int
	report    Report
}

func (s *sanitizer) take(count int) error {
	if count < 0 || count > s.remaining {
		return errors.New("telemetry exceeds sanitizer traversal budget")
	}
	s.remaining -= count
	return nil
}

func (s *sanitizer) attributes(attributes []*commonpb.KeyValue, parent, path string, depth int) error {
	if depth > maxDepth {
		return errors.New("attribute nesting exceeds sanitizer limit")
	}
	if err := s.take(len(attributes)); err != nil {
		return err
	}
	for i, attribute := range attributes {
		if attribute == nil || attribute.Key == "" || !utf8.ValidString(attribute.Key) {
			return errors.New("invalid attribute key")
		}
		keyPath := attribute.Key
		if parent != "" {
			keyPath = parent + "." + attribute.Key
		}
		markerPath := fmt.Sprintf("%s[%d]", path, i)
		matched := false
		for _, pattern := range s.patterns {
			if pattern.MatchString(attribute.Key) || pattern.MatchString(keyPath) {
				matched = true
				break
			}
		}
		if matched {
			attribute.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: Redacted}}
			s.report.Redacted++
			// shortcut: markers are capped but all values/counts are sanitized, stream markers if a future consumer needs every path.
			if len(s.report.Paths) < maxReportPaths {
				s.report.Paths = append(s.report.Paths, markerPath)
			} else {
				s.report.PathsTruncated = true
			}
		} else if err := s.value(attribute.Value, keyPath, markerPath+".value", depth); err != nil {
			return err
		}
	}
	return nil
}

func (s *sanitizer) value(value *commonpb.AnyValue, keyPath, path string, depth int) error {
	if depth > maxDepth {
		return errors.New("attribute nesting exceeds sanitizer limit")
	}
	if err := s.take(1); err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	switch v := value.Value.(type) {
	case *commonpb.AnyValue_ArrayValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
		if err := s.take(len(v.ArrayValue.GetValues())); err != nil {
			return err
		}
		for i, item := range v.ArrayValue.GetValues() {
			if err := s.value(item, keyPath, fmt.Sprintf("%s.array_value.values[%d]", path, i), depth+1); err != nil {
				return err
			}
		}
	case *commonpb.AnyValue_KvlistValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
		return s.attributes(v.KvlistValue.GetValues(), keyPath, path+".kvlist_value.values", depth+1)
	case *commonpb.AnyValue_StringValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
	case *commonpb.AnyValue_BytesValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
	case *commonpb.AnyValue_IntValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
	case *commonpb.AnyValue_DoubleValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
	case *commonpb.AnyValue_BoolValue:
		if v == nil {
			return errors.New("invalid attribute value")
		}
	}
	return nil
}
