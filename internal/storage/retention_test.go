package storage

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"tracelet.local/tracelet/testdata/fixtures"
)

func retainFixture(t *testing.T, s *Store, id byte, lastSeen time.Time, copies int) *collectortracepb.ExportTraceServiceRequest {
	t.Helper()
	request := fixtures.Request("simple")
	scope := request.ResourceSpans[0].ScopeSpans[0]
	scope.Spans = scope.Spans[:1]
	base := scope.Spans[0]
	base.TraceId[15] = id
	if copies > 1 {
		base.Attributes = append(base.Attributes, &commonpb.KeyValue{Key: "synthetic.data", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: strings.Repeat("x", 4096)}}})
		for i := 1; i < copies; i++ {
			span := fixtures.Request("simple").ResourceSpans[0].ScopeSpans[0].Spans[0]
			span.TraceId[15] = id
			span.SpanId[6], span.SpanId[7] = byte((i+1)>>8), byte(i+1)
			span.Attributes = base.Attributes
			scope.Spans = append(scope.Spans, span)
		}
	}
	request.ResourceSpans[0].Resource.Attributes[0].Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "synthetic-service-" + hex.EncodeToString([]byte{id})}}
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE traces SET last_seen_ns=? WHERE trace_id=?", lastSeen.UnixNano(), hex.EncodeToString(base.TraceId)); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestPruneAgeBoundaryQuietWindowAndOrphanMetadata(t *testing.T) {
	s := openStore(t, testConfig(t))
	now := time.Unix(1_800_000_000, 123)
	cutoff := now.Add(-7 * 24 * time.Hour)
	retainFixture(t, s, 1, cutoff.Add(-time.Nanosecond), 1)
	retainFixture(t, s, 2, cutoff, 1)
	retainFixture(t, s, 3, cutoff.Add(time.Nanosecond), 1)
	retainFixture(t, s, 4, now.Add(-29*time.Second), 1)
	revision := scalar(t, s.db, "SELECT revision FROM metadata")
	survivorRevision := scalar(t, s.db, "SELECT revision FROM traces ORDER BY last_seen_ns DESC LIMIT 1")
	usage, err := s.Prune(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.LastPrunedTraces != 2 || usage.PrunedTraces != 2 || usage.TraceCount != 2 || usage.SpanCount != 2 || usage.LastPrunedAt != now.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("unexpected prune: %+v", usage)
	}
	if scalar(t, s.db, "SELECT revision FROM metadata") != revision+1 {
		t.Fatal("deletion did not advance dataset revision once")
	}
	if scalar(t, s.db, "SELECT count(*) FROM resources") != 2 || scalar(t, s.db, "SELECT count(*) FROM scopes") != 1 {
		t.Fatal("orphan metadata remained or shared scope was removed")
	}
	if scalar(t, s.db, "SELECT revision FROM traces ORDER BY last_seen_ns DESC LIMIT 1") != survivorRevision {
		t.Fatal("retention changed a surviving trace revision")
	}
	usage, err = s.Prune(context.Background(), now)
	if err != nil || usage.LastPrunedTraces != 0 || usage.PrunedTraces != 2 || scalar(t, s.db, "SELECT revision FROM metadata") != revision+1 {
		t.Fatalf("no-op prune changed revision: %+v %v", usage, err)
	}
}

func TestPruneBudgetDeletesOldestAndReclaimsPhysicalSpace(t *testing.T) {
	s := openStore(t, testConfig(t))
	now := time.Unix(1_800_000_000, 0)
	oldest := retainFixture(t, s, 1, now.Add(-time.Hour), 130)
	newer := retainFixture(t, s, 2, now.Add(-time.Minute), 130)
	active := retainFixture(t, s, 3, now.Add(-29*time.Second), 1)
	s.cfg.Storage.MaxSizeMB = 1
	before, err := s.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	usage, err := s.Prune(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.LastPrunedTraces != 1 || usage.TraceCount != 2 || usage.TotalBytes >= before.TotalBytes || usage.TotalBytes > usage.BudgetBytes {
		t.Fatalf("did not reclaim only oldest trace: before=%+v after=%+v", before, usage)
	}
	for _, test := range []struct {
		request *collectortracepb.ExportTraceServiceRequest
		want    int
	}{{oldest, 0}, {newer, 1}, {active, 1}} {
		var count int
		id := hex.EncodeToString(test.request.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceId)
		if err := s.db.QueryRow("SELECT count(*) FROM traces WHERE trace_id=?", id).Scan(&count); err != nil || count != test.want {
			t.Fatalf("wrong surviving trace %s: %d, %v", id, count, err)
		}
	}
	for suffix, expected := range map[string]int64{"": usage.DatabaseBytes, "-wal": usage.WALBytes, "-shm": usage.SHMBytes, "-journal": usage.JournalBytes} {
		info, err := os.Stat(s.cfg.Storage.Path + suffix)
		if errors.Is(err, os.ErrNotExist) && expected == 0 {
			continue
		}
		if err != nil || info.Size() != expected {
			t.Fatalf("wrong physical accounting for %q: %d, %v", suffix, expected, err)
		}
	}
	if usage.TotalBytes != usage.DatabaseBytes+usage.WALBytes+usage.SHMBytes+usage.JournalBytes {
		t.Fatal("sidecar accounting does not add up")
	}
}

func TestStoragePressureRejectsNewDataButAllowsDuplicateReplay(t *testing.T) {
	s := openStore(t, testConfig(t))
	now := time.Now()
	existing := retainFixture(t, s, 1, now, 300)
	s.cfg.Storage.MaxSizeMB = 1
	usage, err := s.Prune(context.Background(), now)
	if err != nil || !usage.Pressure || usage.TraceCount != 1 || usage.LastPrunedTraces != 0 {
		t.Fatalf("active trace was not protected: %+v %v", usage, err)
	}
	if result, err := s.Ingest(context.Background(), existing); err != nil || result.Duplicates != 300 || result.Accepted != 0 {
		t.Fatalf("exact retry rejected under pressure: %+v %v", result, err)
	}
	newRequest := fixtures.Request("simple")
	for _, span := range newRequest.ResourceSpans[0].ScopeSpans[0].Spans {
		span.TraceId[15] = 99
	}
	before := scalar(t, s.db, "SELECT revision FROM metadata")
	if result, err := s.Ingest(context.Background(), newRequest); !errors.Is(err, ErrStorageFull) || result.Accepted != 0 {
		t.Fatalf("expected storage backpressure, got %+v %v", result, err)
	}
	if scalar(t, s.db, "SELECT revision FROM metadata") != before || scalar(t, s.db, "SELECT count(*) FROM spans") != 300 {
		t.Fatal("rejected batch changed committed data")
	}
	usage, err = s.Prune(context.Background(), now.Add(30*time.Second))
	if err != nil || usage.Pressure || usage.TraceCount != 0 {
		t.Fatalf("quiet boundary did not reclaim trace: %+v %v", usage, err)
	}
	if result, err := s.Ingest(context.Background(), newRequest); err != nil || result.Accepted != 3 {
		t.Fatalf("did not resume after pruning: %+v %v", result, err)
	}
}

func TestPruneIsIncrementalAndCanceledWorkDoesNotMutate(t *testing.T) {
	s := openStore(t, testConfig(t))
	now := time.Unix(1_800_000_000, 0)
	for i := 1; i <= 66; i++ {
		retainFixture(t, s, byte(i), now.Add(-8*24*time.Hour), 1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Prune(ctx, now); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled prune: %v", err)
	}
	if scalar(t, s.db, "SELECT count(*) FROM traces") != 66 {
		t.Fatal("canceled prune deleted data")
	}
	usage, err := s.Prune(context.Background(), now)
	if err != nil || usage.LastPrunedTraces != 64 || usage.TraceCount != 2 {
		t.Fatalf("prune did not bound deletion work: %+v %v", usage, err)
	}
	usage, err = s.Prune(context.Background(), now)
	if err != nil || usage.LastPrunedTraces != 2 || usage.TraceCount != 0 {
		t.Fatalf("prune did not resume: %+v %v", usage, err)
	}
}

func TestPruneFailureRollsBackWholeDeletion(t *testing.T) {
	s := openStore(t, testConfig(t))
	now := time.Unix(1_800_000_000, 0)
	retainFixture(t, s, 1, now.Add(-8*24*time.Hour), 1)
	retainFixture(t, s, 2, now.Add(-8*24*time.Hour), 1)
	revision := scalar(t, s.db, "SELECT revision FROM metadata")
	if _, err := s.db.Exec("CREATE TRIGGER fail_retention BEFORE DELETE ON resources BEGIN SELECT RAISE(ABORT,'synthetic deletion failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prune(context.Background(), now); err == nil {
		t.Fatal("retention failure was hidden")
	}
	if scalar(t, s.db, "SELECT count(*) FROM traces") != 2 || scalar(t, s.db, "SELECT count(*) FROM spans") != 2 || scalar(t, s.db, "SELECT count(*) FROM resources") != 2 || scalar(t, s.db, "SELECT revision FROM metadata") != revision {
		t.Fatal("failed retention partially deleted data")
	}
	usage, err := s.Usage(context.Background())
	if err != nil || usage.PrunedTraces != 0 || usage.LastPrunedAt != "" {
		t.Fatalf("failed deletion recorded as successful: %+v %v", usage, err)
	}
}

func TestConcurrentIngestUsageAndRetentionSerialize(t *testing.T) {
	s := openStore(t, testConfig(t))
	request := retainFixture(t, s, 1, time.Now(), 1)
	var group sync.WaitGroup
	errorsCh := make(chan error, 30)
	for i := 0; i < 10; i++ {
		group.Add(3)
		go func() {
			defer group.Done()
			_, err := s.Ingest(context.Background(), request)
			errorsCh <- err
		}()
		go func() {
			defer group.Done()
			_, err := s.Usage(context.Background())
			errorsCh <- err
		}()
		go func() {
			defer group.Done()
			_, err := s.Prune(context.Background(), time.Now())
			errorsCh <- err
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if scalar(t, s.db, "SELECT count(*) FROM spans") != 1 {
		t.Fatal("concurrent maintenance changed active data")
	}
}
