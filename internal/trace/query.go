package trace

// TraceQuery uses bounded keyset pages, defaulting to 50 traces (maximum 200).
// Q is a literal substring of any retained span name; ASCII case is ignored.
// Sort is newest (default), slowest, or errors, with trace ID breaking ties.
// A cursor binds the query, limit, sort tuple, and dataset revision. Restart
// pagination after the dataset revision changes.
type TraceQuery struct {
	Limit  int
	Cursor string
	Q      string
	Sort   string
}

type TracePage struct {
	Traces     []TraceSummary `json:"traces"`
	Revision   string         `json:"revision"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type SpanPage struct {
	TraceID    string        `json:"trace_id"`
	Spans      []SpanSummary `json:"spans"`
	Revision   string        `json:"revision"`
	SpanCount  int           `json:"span_count"`
	NextCursor string        `json:"next_cursor,omitempty"`
}
