package spike

import (
	"compress/gzip"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, setting := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(setting); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	return db
}

// shortcut: this receiver only handles the synthetic self-test, replace with the bounded production receiver in T07.
func startReceiver(t *testing.T, db *sql.DB) (*httptest.Server, <-chan *collectorpb.ExportTraceServiceRequest) {
	t.Helper()
	received := make(chan *collectorpb.ExportTraceServiceRequest, 1)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-protobuf" || r.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("unexpected exporter request: %s %s type=%s encoding=%s", r.Method, r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get("Content-Encoding"))
			http.Error(w, "unexpected exporter request", http.StatusBadRequest)
			return
		}
		compressed := http.MaxBytesReader(w, r.Body, 4<<20)
		reader, err := gzip.NewReader(compressed)
		if err != nil {
			t.Error(err)
			http.Error(w, "bad gzip", 400)
			return
		}
		defer reader.Close()
		payload, err := io.ReadAll(io.LimitReader(reader, (16<<20)+1))
		if err != nil || len(payload) > 16<<20 {
			t.Errorf("read: %v, %d bytes", err, len(payload))
			http.Error(w, "invalid body", 400)
			return
		}
		decoded := new(collectorpb.ExportTraceServiceRequest)
		if err = proto.Unmarshal(payload, decoded); err != nil {
			t.Error(err)
			http.Error(w, "bad protobuf", 400)
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			t.Error(err)
			http.Error(w, "transaction failure", 500)
			return
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO batches(payload) VALUES (?)`, payload); err != nil {
			t.Error(err)
			http.Error(w, "write failure", 500)
			return
		}
		if err = tx.Commit(); err != nil {
			t.Error(err)
			http.Error(w, "commit failure", 500)
			return
		}
		select {
		case received <- decoded:
		default:
			t.Error("unexpected extra export batch")
		}
		response, err := proto.Marshal(&collectorpb.ExportTraceServiceResponse{})
		if err != nil {
			t.Error(err)
			http.Error(w, "encode failure", 500)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		if _, err = w.Write(response); err != nil {
			t.Error(err)
		}
	})}}
	server.Start()
	t.Cleanup(server.Close)
	return server, received
}

func fixtureSpan(index int) *tracepb.Span {
	traceID, spanID := make([]byte, 16), make([]byte, 8)
	binary.BigEndian.PutUint64(traceID[8:], uint64(index/100+1))
	binary.BigEndian.PutUint64(spanID, uint64(index+1))
	var parent []byte
	if index%100 != 0 {
		parent = make([]byte, 8)
		binary.BigEndian.PutUint64(parent, uint64(index/100*100+1))
	}
	text := func(key, value string) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
	}
	start := uint64(1_700_000_000_123_456_789 + int64(index)*10_000)
	return &tracepb.Span{
		TraceId: traceID, SpanId: spanID, ParentSpanId: parent, Name: "synthetic-operation", Kind: tracepb.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: start, EndTimeUnixNano: start + 1_000_000,
		Attributes: []*commonpb.KeyValue{
			text("code.file.path", "/synthetic/orders.go"), text("code.function.name", "example.GetOrder"),
			{Key: "code.line.number", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}},
			{Key: "synthetic.sequence", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(index)}}},
			text("synthetic.category", "orders"), text("synthetic.region", "local"),
			{Key: "synthetic.cached", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: index%2 == 0}}},
			{Key: "synthetic.labels", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{{Value: &commonpb.AnyValue_StringValue{StringValue: "one"}}, {Value: &commonpb.AnyValue_StringValue{StringValue: "two"}}}}}}},
		},
		Events: []*tracepb.Span_Event{{TimeUnixNano: start + 1, Name: "synthetic-event", Attributes: []*commonpb.KeyValue{text("synthetic.note", "fixture")}}},
		Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
	}
}

func runStorageExperiment(t *testing.T, layout string) {
	t.Helper()
	const count, batchSize, querySamples = 100_000, 100, 100
	path := filepath.Join(t.TempDir(), layout+".sqlite")
	db := openDB(t, path)
	defer db.Close()
	statements := []string{
		`CREATE TABLE spans(trace_id TEXT NOT NULL,span_id TEXT NOT NULL,start_ns INTEGER NOT NULL,end_ns INTEGER NOT NULL,payload BLOB NOT NULL,PRIMARY KEY(trace_id,span_id))`,
		`CREATE INDEX spans_start ON spans(start_ns,trace_id)`,
		`CREATE TABLE attributes(trace_id TEXT NOT NULL,span_id TEXT NOT NULL,key TEXT NOT NULL,value BLOB NOT NULL,PRIMARY KEY(trace_id,span_id,key),FOREIGN KEY(trace_id,span_id) REFERENCES spans(trace_id,span_id))`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	batches := make([]time.Duration, 0, count/batchSize)
	var payloadBytes int64
	began := time.Now()
	for offset := 0; offset < count; offset += batchSize {
		batchStarted := time.Now()
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		insert, err := tx.Prepare(`INSERT INTO spans VALUES(?,?,?,?,?)`)
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		attrs, err := tx.Prepare(`INSERT INTO attributes VALUES(?,?,?,?)`)
		if err != nil {
			insert.Close()
			tx.Rollback()
			t.Fatal(err)
		}
		for index := offset; index < offset+batchSize; index++ {
			span := fixtureSpan(index)
			traceID, spanID := hex.EncodeToString(span.TraceId), hex.EncodeToString(span.SpanId)
			attributes := span.Attributes
			if layout == "normalized_attributes" {
				span.Attributes = nil
			}
			payload, err := proto.Marshal(span)
			if err != nil {
				t.Fatal(err)
			}
			payloadBytes += int64(len(payload))
			if _, err = insert.Exec(traceID, spanID, span.StartTimeUnixNano, span.EndTimeUnixNano, payload); err != nil {
				t.Fatal(err)
			}
			if layout == "normalized_attributes" {
				for _, a := range attributes {
					value, err := proto.Marshal(a.Value)
					if err != nil {
						t.Fatal(err)
					}
					payloadBytes += int64(len(value) + len(a.Key))
					if _, err = attrs.Exec(traceID, spanID, a.Key, value); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		if err = insert.Close(); err != nil {
			t.Fatal(err)
		}
		if err = attrs.Close(); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, time.Since(batchStarted))
	}
	ingestElapsed := time.Since(began)
	runtime.ReadMemStats(&after)
	var rows, attributeRows int
	if err := db.QueryRow(`SELECT count(*) FROM spans`).Scan(&rows); err != nil || rows != count {
		t.Fatalf("rows %d: %v", rows, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM attributes`).Scan(&attributeRows); err != nil {
		t.Fatal(err)
	}
	wantAttrs := 0
	if layout == "normalized_attributes" {
		wantAttrs = count * 8
	}
	if attributeRows != wantAttrs {
		t.Fatalf("attribute rows %d, want %d", attributeRows, wantAttrs)
	}
	// Both layouts keep non-attribute OTLP fields intact; the normalized candidate reconstructs only attributes.
	var stored []byte
	first := fixtureSpan(0)
	traceID, spanID := hex.EncodeToString(first.TraceId), hex.EncodeToString(first.SpanId)
	if err := db.QueryRow(`SELECT payload FROM spans WHERE trace_id=? AND span_id=?`, traceID, spanID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	recovered := new(tracepb.Span)
	if err := proto.Unmarshal(stored, recovered); err != nil {
		t.Fatal(err)
	}
	if layout == "normalized_attributes" {
		result, err := db.Query(`SELECT key,value FROM attributes WHERE trace_id=? AND span_id=? ORDER BY key`, traceID, spanID)
		if err != nil {
			t.Fatal(err)
		}
		for result.Next() {
			var key string
			var value []byte
			if err = result.Scan(&key, &value); err != nil {
				t.Fatal(err)
			}
			a := &commonpb.AnyValue{}
			if err = proto.Unmarshal(value, a); err != nil {
				t.Fatal(err)
			}
			recovered.Attributes = append(recovered.Attributes, &commonpb.KeyValue{Key: key, Value: a})
		}
		if err = result.Err(); err != nil {
			t.Fatal(err)
		}
		result.Close()
		slices.SortFunc(first.Attributes, func(a, b *commonpb.KeyValue) int {
			if a.Key < b.Key {
				return -1
			}
			if a.Key > b.Key {
				return 1
			}
			return 0
		})
	}
	if !proto.Equal(first, recovered) {
		t.Fatal("candidate storage lost span fields")
	}
	latencies := make([]time.Duration, 0, querySamples)
	for n := 0; n < querySamples+1; n++ {
		began := time.Now()
		result, err := db.Query(`SELECT trace_id,min(start_ns),max(end_ns),count(*) FROM spans GROUP BY trace_id ORDER BY min(start_ns) DESC,trace_id LIMIT 50`)
		if err != nil {
			t.Fatal(err)
		}
		seen := 0
		for result.Next() {
			var id string
			var start, end, total int64
			if err = result.Scan(&id, &start, &end, &total); err != nil {
				t.Fatal(err)
			}
			if total != 100 || end < start {
				t.Fatal("bad trace summary")
			}
			seen++
		}
		if err = result.Err(); err != nil {
			t.Fatal(err)
		}
		result.Close()
		if seen != 50 {
			t.Fatalf("got %d trace summaries", seen)
		}
		if n > 0 {
			latencies = append(latencies, time.Since(began))
		}
	}
	fileSize := func(name string) int64 {
		info, err := os.Stat(name)
		if os.IsNotExist(err) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return info.Size()
	}
	sizes := map[string]int64{"db": fileSize(path), "wal": fileSize(path + "-wal"), "shm": fileSize(path + "-shm")}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity %q: %v", integrity, err)
	}
	percentile := func(values []time.Duration) float64 {
		slices.Sort(values)
		return float64(values[(len(values)*95+99)/100-1]) / float64(time.Millisecond)
	}
	result := map[string]any{
		"layout": layout, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(),
		"spans": count, "spans_per_trace": 100, "attributes_per_span": 8, "batch_size": batchSize, "query_samples": querySamples, "query_warmups": 1,
		"ingest_seconds": ingestElapsed.Seconds(), "spans_per_second": float64(count) / ingestElapsed.Seconds(), "batch_commit_p95_ms": percentile(batches), "trace_list_p95_ms": percentile(latencies),
		"payload_bytes": payloadBytes, "disk_bytes_before_close": sizes, "go_heap_alloc_bytes_after_ingest": after.HeapAlloc, "go_heap_sys_bytes_after_ingest": after.HeapSys, "go_total_alloc_delta_bytes": after.TotalAlloc - before.TotalAlloc,
		"journal": "wal", "synchronous": "full", "connections": 1,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("STORAGE_RESULT " + string(encoded))
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openDB(t, path)
	defer reopened.Close()
	if err = reopened.QueryRow(`SELECT count(*) FROM spans`).Scan(&rows); err != nil || rows != count {
		t.Fatalf("reopen rows %d: %v", rows, err)
	}
	t.Log(fmt.Sprintf("CLOSED_DB_BYTES %d", fileSize(path)))
}
