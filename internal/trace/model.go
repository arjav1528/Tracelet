package trace

import (
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const SchemaVersion = 1

type SpanSummary struct {
	TraceID           string `json:"trace_id"`
	SpanID            string `json:"span_id"`
	ParentSpanID      string `json:"parent_span_id,omitempty"`
	Name              string `json:"name"`
	ServiceName       string `json:"service_name"`
	StartTimeUnixNano string `json:"start_time_unix_nano"`
	EndTimeUnixNano   string `json:"end_time_unix_nano"`
	DurationNano      string `json:"duration_nano"`
	Kind              int32  `json:"kind"`
	Status            int32  `json:"status"`
}

type TraceSummary struct {
	TraceID           string `json:"trace_id"`
	Operation         string `json:"operation"`
	ServiceName       string `json:"service_name"`
	StartTimeUnixNano string `json:"start_time_unix_nano"`
	EndTimeUnixNano   string `json:"end_time_unix_nano"`
	DurationNano      string `json:"duration_nano"`
	SpanCount         int    `json:"span_count"`
	HasError          bool   `json:"has_error"`
	Revision          string `json:"revision"`
}

type TreeWarning struct {
	Code    string `json:"code"`
	SpanID  string `json:"span_id,omitempty"`
	Message string `json:"message"`
}

type TraceDetail struct {
	NextCursor string        `json:"next_cursor,omitempty"`
	Summary    TraceSummary  `json:"summary"`
	Spans      []SpanSummary `json:"spans"`
	Warnings   []TreeWarning `json:"warnings"`
}

// ProtoJSON preserves 64-bit integers and byte values at the browser boundary.
type Telemetry struct {
	Request *collectortracepb.ExportTraceServiceRequest
}

func (t Telemetry) MarshalJSON() ([]byte, error) {
	return protojson.Marshal(t.Request)
}

func (t *Telemetry) UnmarshalJSON(data []byte) error {
	request := &collectortracepb.ExportTraceServiceRequest{}
	if err := protojson.Unmarshal(data, request); err != nil {
		return err
	}
	t.Request = request
	return nil
}
