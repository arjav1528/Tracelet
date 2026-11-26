package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"tracelet.local/tracelet/internal/trace"
)

var ErrNotFound = errors.New("trace or span not found")
var ErrInvalidQuery = errors.New("invalid trace query")
var ErrRevisionChanged = errors.New("trace data changed; restart pagination")

// Display operation and service come from the earliest root by start and span ID;
// a rootless trace uses its earliest observed span by the same ordering.
const traceProjection = `SELECT t.trace_id,root.name,root.service_name,t.start_ns,t.end_ns,t.span_count,(t.error_count>0),t.revision
FROM traces t JOIN spans root ON root.trace_id=t.trace_id AND root.span_id=(
 SELECT candidate.span_id FROM spans candidate WHERE candidate.trace_id=t.trace_id
 ORDER BY (candidate.parent_span_id!=''),candidate.start_ns,candidate.span_id LIMIT 1)`

type pageCursor struct {
	Version   int    `json:"v"`
	Kind      string `json:"kind"`
	Limit     int    `json:"limit"`
	Sort      string `json:"sort,omitempty"`
	Q         string `json:"q,omitempty"`
	Revision  int64  `json:"revision"`
	TraceID   string `json:"trace_id,omitempty"`
	Primary   int64  `json:"primary"`
	Secondary int64  `json:"secondary,omitempty"`
	ID        string `json:"id"`
}

func (s *Store) ListTraces(ctx context.Context, query trace.TraceQuery) (trace.TracePage, error) {
	page := trace.TracePage{Traces: []trace.TraceSummary{}}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Sort == "" {
		query.Sort = "newest"
	}
	if query.Limit < 1 || query.Limit > 200 || len(query.Q) > 256 || !utf8.ValidString(query.Q) || strings.ContainsRune(query.Q, 0) {
		return page, ErrInvalidQuery
	}
	orders := map[string]string{"newest": "t.start_ns DESC,t.trace_id ASC", "slowest": "(t.end_ns-t.start_ns) DESC,t.start_ns DESC,t.trace_id ASC", "errors": "(t.error_count>0) DESC,t.start_ns DESC,t.trace_id ASC"}
	order, ok := orders[query.Sort]
	if !ok {
		return page, ErrInvalidQuery
	}
	var cursor pageCursor
	if query.Cursor != "" {
		var err error
		cursor, err = decodeCursor(query.Cursor)
		if err != nil || cursor.Kind != "traces" || cursor.Limit != query.Limit || cursor.Sort != query.Sort || cursor.Q != query.Q || cursor.TraceID != "" {
			return page, ErrInvalidQuery
		}
		if cursor.ID, err = normalizedID(cursor.ID, 16); err != nil {
			return page, err
		}
		if query.Sort == "errors" && cursor.Primary > 1 {
			return page, ErrInvalidQuery
		}
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, fmt.Errorf("read traces: %w", err)
	}
	defer tx.Rollback()
	var revision int64
	if err := tx.QueryRowContext(ctx, "SELECT revision FROM metadata WHERE id=1").Scan(&revision); err != nil {
		return page, fmt.Errorf("read dataset revision: %w", err)
	}
	if query.Cursor != "" && cursor.Revision != revision {
		return page, ErrRevisionChanged
	}
	page.Revision = strconv.FormatInt(revision, 10)
	conditions := []string{"1=1"}
	args := []any{}
	if query.Q != "" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM spans searched WHERE searched.trace_id=t.trace_id AND instr(lower(searched.name),lower(?))>0)")
		args = append(args, query.Q)
	}
	if query.Cursor != "" {
		switch query.Sort {
		case "newest":
			conditions = append(conditions, "(t.start_ns<? OR (t.start_ns=? AND t.trace_id>?))")
			args = append(args, cursor.Primary, cursor.Primary, cursor.ID)
		case "slowest":
			conditions = append(conditions, "((t.end_ns-t.start_ns)<? OR ((t.end_ns-t.start_ns)=? AND (t.start_ns<? OR (t.start_ns=? AND t.trace_id>?))))")
			args = append(args, cursor.Primary, cursor.Primary, cursor.Secondary, cursor.Secondary, cursor.ID)
		case "errors":
			conditions = append(conditions, "((t.error_count>0)<? OR ((t.error_count>0)=? AND (t.start_ns<? OR (t.start_ns=? AND t.trace_id>?))))")
			args = append(args, cursor.Primary, cursor.Primary, cursor.Secondary, cursor.Secondary, cursor.ID)
		}
	}
	args = append(args, query.Limit+1)
	rows, err := tx.QueryContext(ctx, traceProjection+" WHERE "+strings.Join(conditions, " AND ")+" ORDER BY "+order+" LIMIT ?", args...)
	if err != nil {
		return page, fmt.Errorf("list traces: %w", err)
	}
	for rows.Next() {
		summary, err := scanTrace(rows)
		if err != nil {
			rows.Close()
			return page, err
		}
		page.Traces = append(page.Traces, summary)
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return page, fmt.Errorf("read trace rows: %w", err)
	}
	if len(page.Traces) > query.Limit {
		page.Traces = page.Traces[:query.Limit]
		last := page.Traces[len(page.Traces)-1]
		start, _ := strconv.ParseInt(last.StartTimeUnixNano, 10, 64)
		duration, _ := strconv.ParseInt(last.DurationNano, 10, 64)
		cursor = pageCursor{Version: 1, Kind: "traces", Limit: query.Limit, Sort: query.Sort, Q: query.Q, Revision: revision, ID: last.TraceID, Primary: start}
		if query.Sort == "slowest" {
			cursor.Primary = duration
			cursor.Secondary = start
		}
		if query.Sort == "errors" {
			cursor.Primary = 0
			if last.HasError {
				cursor.Primary = 1
			}
			cursor.Secondary = start
		}
		page.NextCursor = encodeCursor(cursor)
	}
	if err := tx.Commit(); err != nil {
		return trace.TracePage{}, fmt.Errorf("finish trace snapshot: %w", err)
	}
	return page, nil
}

func (s *Store) Trace(ctx context.Context, id string) (trace.TraceDetail, error) {
	detail := trace.TraceDetail{Spans: []trace.SpanSummary{}, Warnings: []trace.TreeWarning{}}
	id, err := normalizedID(id, 16)
	if err != nil {
		return detail, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return detail, fmt.Errorf("read trace: %w", err)
	}
	defer tx.Rollback()
	detail.Summary, err = scanTrace(tx.QueryRowContext(ctx, traceProjection+" WHERE t.trace_id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return detail, ErrNotFound
	}
	if err != nil {
		return detail, err
	}
	revision, _ := strconv.ParseInt(detail.Summary.Revision, 10, 64)
	page, err := spanPage(ctx, tx, id, pageCursor{}, 500, revision, detail.Summary.SpanCount)
	if err != nil {
		return detail, err
	}
	detail.Spans = page.Spans
	detail.NextCursor = page.NextCursor
	if page.NextCursor == "" && len(page.Spans) == page.SpanCount {
		detail.Warnings = trace.BuildTree(page.Spans).Warnings
	}
	if err := tx.Commit(); err != nil {
		return trace.TraceDetail{}, fmt.Errorf("finish trace snapshot: %w", err)
	}
	return detail, nil
}

func (s *Store) TraceSpans(ctx context.Context, id, cursorText string, limit int) (trace.SpanPage, error) {
	page := trace.SpanPage{Spans: []trace.SpanSummary{}}
	id, err := normalizedID(id, 16)
	if err != nil {
		return page, err
	}
	if limit == 0 {
		limit = 500
	}
	if limit < 1 || limit > 2000 {
		return page, ErrInvalidQuery
	}
	var cursor pageCursor
	if cursorText != "" {
		cursor, err = decodeCursor(cursorText)
		if err != nil || cursor.Kind != "spans" || cursor.TraceID != id || cursor.Limit != limit || cursor.Sort != "" || cursor.Q != "" || cursor.Secondary != 0 {
			return page, ErrInvalidQuery
		}
		if cursor.ID, err = normalizedID(cursor.ID, 8); err != nil {
			return page, err
		}
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, fmt.Errorf("read trace page: %w", err)
	}
	defer tx.Rollback()
	var revision int64
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT revision,span_count FROM traces WHERE trace_id=?", id).Scan(&revision, &count); errors.Is(err, sql.ErrNoRows) {
		return page, ErrNotFound
	} else if err != nil {
		return page, fmt.Errorf("read trace revision: %w", err)
	}
	if cursorText != "" && cursor.Revision != revision {
		return page, ErrRevisionChanged
	}
	page, err = spanPage(ctx, tx, id, cursor, limit, revision, count)
	if err != nil {
		return page, err
	}
	if err := tx.Commit(); err != nil {
		return trace.SpanPage{}, fmt.Errorf("finish span snapshot: %w", err)
	}
	return page, nil
}

func spanPage(ctx context.Context, tx *sql.Tx, id string, cursor pageCursor, limit int, revision int64, count int) (trace.SpanPage, error) {
	page := trace.SpanPage{TraceID: id, Spans: []trace.SpanSummary{}, Revision: strconv.FormatInt(revision, 10), SpanCount: count}
	query := "SELECT trace_id,span_id,parent_span_id,name,service_name,start_ns,end_ns,kind,status FROM spans WHERE trace_id=?"
	args := []any{id}
	if cursor.ID != "" {
		query += " AND (start_ns>? OR (start_ns=? AND span_id>?))"
		args = append(args, cursor.Primary, cursor.Primary, cursor.ID)
	}
	query += " ORDER BY start_ns,span_id LIMIT ?"
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return page, fmt.Errorf("list trace spans: %w", err)
	}
	for rows.Next() {
		var span trace.SpanSummary
		var start, end int64
		if err := rows.Scan(&span.TraceID, &span.SpanID, &span.ParentSpanID, &span.Name, &span.ServiceName, &start, &end, &span.Kind, &span.Status); err != nil {
			rows.Close()
			return page, fmt.Errorf("read span summary: %w", err)
		}
		span.StartTimeUnixNano = strconv.FormatInt(start, 10)
		span.EndTimeUnixNano = strconv.FormatInt(end, 10)
		span.DurationNano = strconv.FormatInt(end-start, 10)
		page.Spans = append(page.Spans, span)
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return page, fmt.Errorf("read span page: %w", err)
	}
	if len(page.Spans) > limit {
		page.Spans = page.Spans[:limit]
		last := page.Spans[len(page.Spans)-1]
		start, _ := strconv.ParseInt(last.StartTimeUnixNano, 10, 64)
		page.NextCursor = encodeCursor(pageCursor{Version: 1, Kind: "spans", Limit: limit, Revision: revision, TraceID: id, Primary: start, ID: last.SpanID})
	}
	return page, nil
}

func (s *Store) Span(ctx context.Context, traceID, spanID string) (trace.Telemetry, error) {
	var result trace.Telemetry
	traceID, err := normalizedID(traceID, 16)
	if err != nil {
		return result, err
	}
	spanID, err = normalizedID(spanID, 8)
	if err != nil {
		return result, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("read selected span: %w", err)
	}
	defer tx.Rollback()
	var spanPayload, resourcePayload, scopePayload []byte
	err = tx.QueryRowContext(ctx, `SELECT s.payload,r.payload,c.payload FROM spans s JOIN resources r ON r.resource_id=s.resource_id JOIN scopes c ON c.scope_id=s.scope_id WHERE s.trace_id=? AND s.span_id=?`, traceID, spanID).Scan(&spanPayload, &resourcePayload, &scopePayload)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, fmt.Errorf("read span details: %w", err)
	}
	span := new(tracepb.Span)
	resource := new(tracepb.ResourceSpans)
	scope := new(tracepb.ScopeSpans)
	decode := proto.UnmarshalOptions{RecursionLimit: 256}
	for _, part := range []struct {
		payload []byte
		message proto.Message
	}{{spanPayload, span}, {resourcePayload, resource}, {scopePayload, scope}} {
		if err := decode.Unmarshal(part.payload, part.message); err != nil {
			return result, errors.New("stored span metadata could not be decoded")
		}
	}
	scope.Spans = []*tracepb.Span{span}
	resource.ScopeSpans = []*tracepb.ScopeSpans{scope}
	result.Request = &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{resource}}
	if err := tx.Commit(); err != nil {
		return trace.Telemetry{}, fmt.Errorf("finish span detail snapshot: %w", err)
	}
	return result, nil
}

func scanTrace(row interface{ Scan(...any) error }) (trace.TraceSummary, error) {
	var summary trace.TraceSummary
	var start, end, revision int64
	if err := row.Scan(&summary.TraceID, &summary.Operation, &summary.ServiceName, &start, &end, &summary.SpanCount, &summary.HasError, &revision); err != nil {
		return summary, err
	}
	summary.StartTimeUnixNano = strconv.FormatInt(start, 10)
	summary.EndTimeUnixNano = strconv.FormatInt(end, 10)
	summary.DurationNano = strconv.FormatInt(end-start, 10)
	summary.Revision = strconv.FormatInt(revision, 10)
	return summary, nil
}

func normalizedID(value string, size int) (string, error) {
	if len(value) != size*2 {
		return "", ErrInvalidQuery
	}
	id, err := hex.DecodeString(value)
	if err != nil {
		return "", ErrInvalidQuery
	}
	nonzero := false
	for _, b := range id {
		nonzero = nonzero || b != 0
	}
	if !nonzero {
		return "", ErrInvalidQuery
	}
	return strings.ToLower(value), nil
}

func decodeCursor(value string) (pageCursor, error) {
	var cursor pageCursor
	if len(value) > 4096 {
		return cursor, ErrInvalidQuery
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return cursor, ErrInvalidQuery
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return cursor, ErrInvalidQuery
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return cursor, ErrInvalidQuery
	}
	if cursor.Version != 1 || cursor.Revision < 0 || cursor.Primary < 0 || cursor.Secondary < 0 {
		return cursor, ErrInvalidQuery
	}
	return cursor, nil
}

func encodeCursor(cursor pageCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}
