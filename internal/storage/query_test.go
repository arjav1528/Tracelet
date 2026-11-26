package storage

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"tracelet.local/tracelet/internal/trace"
	"tracelet.local/tracelet/testdata/fixtures"
)

func queryFixture(id uint64, name string, start, duration uint64, failed bool) *collectortracepb.ExportTraceServiceRequest {
	request := fixtures.Request("late-parent")
	span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	binary.BigEndian.PutUint64(span.TraceId[8:], id)
	span.Name = name
	span.StartTimeUnixNano = start
	span.EndTimeUnixNano = start + duration
	if failed {
		span.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR}
	}
	return request
}

func ingestQueryFixture(t *testing.T, s *Store, id uint64, name string, start, duration uint64, failed bool) string {
	t.Helper()
	request := queryFixture(id, name, start, duration, failed)
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(request.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
}

func TestTraceListSortSearchAndTiePagination(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx := context.Background()
	ids := []string{}
	for i := uint64(1); i <= 4; i++ {
		ids = append(ids, ingestQueryFixture(t, s, i, "GET /same", 100, 10, false))
	}
	for _, sort := range []string{"newest", "slowest", "errors"} {
		cursor := ""
		for i, id := range ids {
			page, err := s.ListTraces(ctx, trace.TraceQuery{Sort: sort, Limit: 1, Cursor: cursor})
			if err != nil || len(page.Traces) != 1 || page.Traces[0].TraceID != id {
				t.Fatalf("%s tied page %d=%+v err=%v", sort, i, page, err)
			}
			cursor = page.NextCursor
			if (cursor == "") != (i == len(ids)-1) {
				t.Fatalf("%s tied page %d cursor=%q", sort, i, cursor)
			}
		}
	}
	first, err := s.ListTraces(ctx, trace.TraceQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Traces) != 2 || first.NextCursor == "" || first.Traces[0].TraceID != ids[0] || first.Traces[1].TraceID != ids[1] {
		t.Fatalf("first=%+v", first)
	}
	if _, err := s.Ingest(ctx, queryFixture(1, "GET /same", 100, 10, false)); err != nil {
		t.Fatal(err)
	}
	second, err := s.ListTraces(ctx, trace.TraceQuery{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Traces) != 2 || second.NextCursor != "" || second.Traces[0].TraceID != ids[2] || second.Traces[1].TraceID != ids[3] || first.Revision != second.Revision {
		t.Fatalf("second=%+v", second)
	}
	if _, err := s.ListTraces(ctx, trace.TraceQuery{Limit: 2, Cursor: first.NextCursor, Q: "different"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("changed query cursor: %v", err)
	}
	for _, query := range []trace.TraceQuery{{Limit: 1, Cursor: first.NextCursor}, {Limit: 2, Sort: "slowest", Cursor: first.NextCursor}} {
		if _, err := s.ListTraces(ctx, query); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("changed cursor parameters: %v", err)
		}
	}
	ingestQueryFixture(t, s, 5, "new trace", 200, 10, false)
	if _, err := s.ListTraces(ctx, trace.TraceQuery{Limit: 2, Cursor: first.NextCursor}); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("stale cursor: %v", err)
	}
}

func TestTraceListSortsAndLiteralSearch(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx := context.Background()
	a := ingestQueryFixture(t, s, 1, "GET /one", 10, 100, false)
	b := ingestQueryFixture(t, s, 2, "worker 100%_FAIL", 20, 10, true)
	c := ingestQueryFixture(t, s, 3, "GET /three", 30, 50, true)
	for sort, want := range map[string][]string{"newest": {c, b, a}, "slowest": {a, c, b}, "errors": {c, b, a}} {
		t.Run(sort, func(t *testing.T) {
			page, err := s.ListTraces(ctx, trace.TraceQuery{Sort: sort})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Traces) != 3 {
				t.Fatal("missing traces")
			}
			for i, id := range want {
				if page.Traces[i].TraceID != id {
					t.Fatalf("order=%+v", page.Traces)
				}
			}
			cursor := ""
			for i, id := range want {
				page, err := s.ListTraces(ctx, trace.TraceQuery{Sort: sort, Limit: 1, Cursor: cursor})
				if err != nil || len(page.Traces) != 1 || page.Traces[0].TraceID != id {
					t.Fatalf("page %d=%+v err=%v", i, page, err)
				}
				cursor = page.NextCursor
				if (cursor == "") != (i == len(want)-1) {
					t.Fatalf("page %d cursor=%q", i, cursor)
				}
			}
		})
	}
	for query, want := range map[string]int{"get /": 2, "%_": 1, "' OR 1=1 --": 0} {
		page, err := s.ListTraces(ctx, trace.TraceQuery{Q: query})
		if err != nil || len(page.Traces) != want {
			t.Fatalf("search %q: %+v %v", query, page, err)
		}
	}
}

func TestTraceSummaryUsesRootsAndObservedBounds(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx := context.Background()
	request := fixtures.Request("partial")
	child := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	child.Name = "earlier child"
	child.StartTimeUnixNano = 10
	child.EndTimeUnixNano = 100
	if _, err := s.Ingest(ctx, request); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(child.TraceId)
	detail, err := s.Trace(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Summary.Operation != "earlier child" || detail.Summary.SpanCount != 1 || len(detail.Warnings) != 2 {
		t.Fatalf("partial=%+v", detail)
	}
	warnings := map[string]bool{}
	for _, warning := range detail.Warnings {
		warnings[warning.Code] = true
	}
	if !warnings["missing_parent"] || !warnings["missing_root"] {
		t.Fatalf("partial warnings=%+v", detail.Warnings)
	}
	request = fixtures.Request("late-parent")
	root := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	root.Name = "root operation"
	root.StartTimeUnixNano = 20
	root.EndTimeUnixNano = 200
	second := proto.Clone(root).(*tracepb.Span)
	second.SpanId[7] = 3
	second.Name = "other root"
	request.ResourceSpans[0].ScopeSpans[0].Spans = append(request.ResourceSpans[0].ScopeSpans[0].Spans, second)
	if _, err := s.Ingest(ctx, request); err != nil {
		t.Fatal(err)
	}
	detail, err = s.Trace(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Summary.Operation != "root operation" || detail.Summary.DurationNano != "190" || detail.Summary.StartTimeUnixNano != "10" || detail.Summary.SpanCount != 3 {
		t.Fatalf("summary=%+v", detail.Summary)
	}
	for _, warning := range detail.Warnings {
		if warning.Code == "missing_parent" || warning.Code == "missing_root" {
			t.Fatal("late parent remained missing")
		}
	}
}

func TestTimelineQueriesDoNotLoadDetailPayloads(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx := context.Background()
	id := ingestQueryFixture(t, s, 1, "bounded detail", 1795679780123456789, 7, false)
	// A damaged detail field cannot prevent the scalar timeline from loading.
	// The selected-span route must still reject that damaged payload.
	if _, err := s.db.ExecContext(ctx, "UPDATE spans SET payload=x'ff'"); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListTraces(ctx, trace.TraceQuery{})
	if err != nil || len(page.Traces) != 1 || page.Traces[0].StartTimeUnixNano != "1795679780123456789" || page.Traces[0].DurationNano != "7" {
		t.Fatalf("summary=%+v err=%v", page, err)
	}
	detail, err := s.Trace(ctx, id)
	if err != nil || len(detail.Spans) != 1 || detail.Spans[0].StartTimeUnixNano != "1795679780123456789" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := s.Span(ctx, id, detail.Spans[0].SpanID); err == nil {
		t.Fatal("damaged selected-span payload was accepted")
	}
}

func TestSpanPagesDoNotInventUnloadedParents(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx := context.Background()
	request := fixtures.Request("large")
	root := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
	root.StartTimeUnixNano += 1_000_000
	root.EndTimeUnixNano = root.StartTimeUnixNano + 100
	id := hex.EncodeToString(root.TraceId)
	if _, err := s.Ingest(ctx, request); err != nil {
		t.Fatal(err)
	}
	first, err := s.Trace(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Spans) != 500 || first.NextCursor == "" || len(first.Warnings) != 0 || first.Summary.SpanCount != 2001 {
		t.Fatalf("initial page=%+v", first.Summary)
	}
	spans := append([]trace.SpanSummary(nil), first.Spans...)
	cursor := first.NextCursor
	ingestQueryFixture(t, s, 2, "unrelated", 1, 1, false)
	for cursor != "" {
		page, err := s.TraceSpans(ctx, id, cursor, 500)
		if err != nil {
			t.Fatal(err)
		}
		if page.Revision != first.Summary.Revision || page.SpanCount != 2001 {
			t.Fatal("unstable trace page")
		}
		spans = append(spans, page.Spans...)
		cursor = page.NextCursor
	}
	if len(spans) != 2001 {
		t.Fatalf("loaded %d spans", len(spans))
	}
	seen := map[string]bool{}
	for _, span := range spans {
		if seen[span.SpanID] {
			t.Fatal("repeated span across pages")
		}
		seen[span.SpanID] = true
	}
	tree := trace.BuildTree(spans)
	for _, warning := range tree.Warnings {
		if warning.Code == "missing_parent" {
			t.Fatal("loaded parent classified as missing")
		}
	}
	request = fixtures.Request("partial")
	request.ResourceSpans[0].ScopeSpans[0].Spans[0].SpanId[6] = 10
	if _, err := s.Ingest(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TraceSpans(ctx, id, first.NextCursor, 500); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("stale span page: %v", err)
	}
}

func TestSelectedSpanPreservesExactMetadata(t *testing.T) {
	s := openStore(t, testConfig(t))
	request := fixtures.Request("error")
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	selected := request.ResourceSpans[0].ScopeSpans[0].Spans[1]
	expected := proto.Clone(request).(*collectortracepb.ExportTraceServiceRequest)
	expected.ResourceSpans[0].ScopeSpans[0].Spans = []*tracepb.Span{proto.Clone(selected).(*tracepb.Span)}
	got, err := s.Span(context.Background(), hex.EncodeToString(selected.TraceId), hex.EncodeToString(selected.SpanId))
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.Request, expected) {
		t.Fatal("selected span changed resource/scope/event/typed metadata")
	}
}

func TestQueryValidationAndNotFound(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx := context.Background()
	for _, query := range []trace.TraceQuery{{Limit: -1}, {Limit: 201}, {Sort: "drop table traces"}, {Q: strings.Repeat("x", 257)}, {Cursor: "%%%"}, {Cursor: "e30"}} {
		if _, err := s.ListTraces(ctx, query); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("query=%+v err=%v", query, err)
		}
	}
	id := strings.Repeat("0", 31) + "1"
	if _, err := s.Trace(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing trace: %v", err)
	}
	if _, err := s.Trace(ctx, "bad"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid ID: %v", err)
	}
	if _, err := s.Span(ctx, id, "0000000000000001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing span: %v", err)
	}
	if _, err := s.TraceSpans(ctx, id, "", 2001); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unbounded span limit: %v", err)
	}
}
