package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"tracelet.local/tracelet/internal/ingestion"
	"tracelet.local/tracelet/internal/privacy"
)

type IngestResult = ingestion.Result

func (s *Store) Ingest(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (IngestResult, error) {
	select {
	case s.writer <- struct{}{}:
		defer func() { <-s.writer }()
	case <-ctx.Done():
		return IngestResult{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return IngestResult{}, err
	}
	if request == nil {
		return IngestResult{}, errors.New("missing trace batch")
	}
	limits := ingestion.Limits{Attributes: s.cfg.Limits.MaxAttributes, KeyBytes: s.cfg.Limits.MaxKeyBytes, ValueBytes: s.cfg.Limits.MaxValueBytes, Depth: s.cfg.Limits.MaxDepth, Events: s.cfg.Limits.MaxEvents, Links: s.cfg.Limits.MaxLinks}
	count := 0
	for _, resource := range request.ResourceSpans {
		if err := ingestion.ValidateResourceSpans(resource, limits); err != nil {
			return IngestResult{}, fmt.Errorf("invalid shared trace metadata: %w", err)
		}
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				count++
				if count > s.cfg.Limits.MaxSpansPerBatch {
					return IngestResult{}, errors.New("trace batch span count exceeds limit")
				}
				if err := ingestion.ValidateSpan(span, limits); err != nil {
					return IngestResult{}, fmt.Errorf("invalid span: %w", err)
				}
			}
		}
	}
	if int64(proto.Size(request)) > s.cfg.Limits.MaxDecodedBytes {
		return IngestResult{}, errors.New("trace batch decoded size exceeds limit")
	}
	if count == 0 {
		return IngestResult{}, nil
	}
	sanitized := proto.Clone(request).(*collectortracepb.ExportTraceServiceRequest)
	if _, err := privacy.Sanitize(sanitized, privacy.Policy{Patterns: s.cfg.Privacy.RedactAttributeKeys}); err != nil {
		return IngestResult{}, fmt.Errorf("sanitize trace batch: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestResult{}, fmt.Errorf("start trace transaction: %w", err)
	}
	defer tx.Rollback()
	result := IngestResult{}
	touched := map[string]struct{}{}
	receivedAt := time.Now().UnixNano()
	for _, resource := range sanitized.ResourceSpans {
		metadata := &tracepb.ResourceSpans{Resource: resource.Resource, SchemaUrl: resource.SchemaUrl}
		canonicalAttributes(metadata.GetResource().GetAttributes())
		resourcePayload, err := proto.MarshalOptions{Deterministic: true}.Marshal(metadata)
		if err != nil {
			return IngestResult{}, fmt.Errorf("encode resource metadata: %w", err)
		}
		resourceID := contentID(resourcePayload)
		service := ""
		for _, attribute := range resource.GetResource().GetAttributes() {
			if attribute.Key == "service.name" {
				service = attribute.GetValue().GetStringValue()
				break
			}
		}
		for _, scope := range resource.ScopeSpans {
			scopeMetadata := &tracepb.ScopeSpans{Scope: scope.Scope, SchemaUrl: scope.SchemaUrl}
			canonicalAttributes(scopeMetadata.GetScope().GetAttributes())
			scopePayload, err := proto.MarshalOptions{Deterministic: true}.Marshal(scopeMetadata)
			if err != nil {
				return IngestResult{}, fmt.Errorf("encode scope metadata: %w", err)
			}
			scopeID := contentID(scopePayload)
			for _, span := range scope.Spans {
				payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(span)
				if err != nil {
					return IngestResult{}, fmt.Errorf("encode span: %w", err)
				}
				traceID, spanID := hex.EncodeToString(span.TraceId), hex.EncodeToString(span.SpanId)
				var existing []byte
				var storedResource, storedScope string
				err = tx.QueryRowContext(ctx, "SELECT payload,resource_id,scope_id FROM spans WHERE trace_id=? AND span_id=?", traceID, spanID).Scan(&existing, &storedResource, &storedScope)
				if err == nil {
					result.Duplicates++
					if !bytes.Equal(existing, payload) || storedResource != resourceID || storedScope != scopeID {
						result.Conflicts++
					}
					continue
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return IngestResult{}, fmt.Errorf("check span identity: %w", err)
				}
				if _, err := tx.ExecContext(ctx, "INSERT INTO resources(resource_id,payload) VALUES(?,?) ON CONFLICT DO NOTHING", resourceID, resourcePayload); err != nil {
					return IngestResult{}, fmt.Errorf("persist resource: %w", err)
				}
				if _, err := tx.ExecContext(ctx, "INSERT INTO scopes(scope_id,payload) VALUES(?,?) ON CONFLICT DO NOTHING", scopeID, scopePayload); err != nil {
					return IngestResult{}, fmt.Errorf("persist scope: %w", err)
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO spans(trace_id,span_id,parent_span_id,name,service_name,start_ns,end_ns,kind,status,resource_id,scope_id,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, traceID, spanID, hex.EncodeToString(span.ParentSpanId), span.Name, service, int64(span.StartTimeUnixNano), int64(span.EndTimeUnixNano), int32(span.Kind), int32(span.GetStatus().GetCode()), resourceID, scopeID, payload)
				if err != nil {
					return IngestResult{}, fmt.Errorf("persist span: %w", err)
				}
				isError := 0
				if span.GetStatus().GetCode() == tracepb.Status_STATUS_CODE_ERROR {
					isError = 1
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO traces(trace_id,start_ns,end_ns,span_count,error_count,first_seen_ns,last_seen_ns,revision) VALUES(?,?,?,1,?,?,?,0)
ON CONFLICT(trace_id) DO UPDATE SET start_ns=min(start_ns,excluded.start_ns),end_ns=max(end_ns,excluded.end_ns),span_count=span_count+1,error_count=error_count+excluded.error_count,last_seen_ns=excluded.last_seen_ns`, traceID, int64(span.StartTimeUnixNano), int64(span.EndTimeUnixNano), isError, receivedAt, receivedAt)
				if err != nil {
					return IngestResult{}, fmt.Errorf("update trace summary: %w", err)
				}
				result.Accepted++
				touched[traceID] = struct{}{}
			}
		}
	}
	if result.Accepted > 0 {
		var revision int64
		if err := tx.QueryRowContext(ctx, "UPDATE metadata SET revision=revision+1 WHERE id=1 RETURNING revision").Scan(&revision); err != nil {
			return IngestResult{}, fmt.Errorf("advance dataset revision: %w", err)
		}
		for traceID := range touched {
			if _, err := tx.ExecContext(ctx, "UPDATE traces SET revision=? WHERE trace_id=?", revision, traceID); err != nil {
				return IngestResult{}, fmt.Errorf("advance trace revision: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return IngestResult{}, fmt.Errorf("commit trace batch: %w", err)
	}
	return result, nil
}

func contentID(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func canonicalAttributes(attributes []*commonpb.KeyValue) {
	sort.SliceStable(attributes, func(i, j int) bool { return attributes[i].Key < attributes[j].Key })
	for _, attribute := range attributes {
		canonicalValue(attribute.Value)
	}
}

func canonicalValue(value *commonpb.AnyValue) {
	if value == nil {
		return
	}
	switch v := value.Value.(type) {
	case *commonpb.AnyValue_KvlistValue:
		canonicalAttributes(v.KvlistValue.GetValues())
	case *commonpb.AnyValue_ArrayValue:
		for _, item := range v.ArrayValue.GetValues() {
			canonicalValue(item)
		}
	}
}
