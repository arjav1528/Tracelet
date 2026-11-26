package ingestion

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"tracelet.local/tracelet/testdata/fixtures"
)

type sinkFunc func(context.Context, *collectortracepb.ExportTraceServiceRequest) (Result, error)

func (f sinkFunc) Ingest(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (Result, error) {
	return f(ctx, request)
}

func TestPipelineWaitsForSinkAndBoundsPendingBatches(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	sink := sinkFunc(func(ctx context.Context, _ *collectortracepb.ExportTraceServiceRequest) (Result, error) {
		close(started)
		select {
		case <-release:
			return Result{Accepted: 3}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	})
	pipeline, err := NewPipeline(sink, 1, 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		result, err := pipeline.Submit(context.Background(), fixtures.Request("simple"))
		if err == nil && result.Accepted != 3 {
			err = errors.New("accepted count changed")
		}
		done <- err
	}()
	<-started
	select {
	case <-done:
		t.Fatal("returned before sink committed")
	default:
	}
	if _, err := pipeline.Submit(context.Background(), fixtures.Request("simple")); !errors.Is(err, ErrOverloaded) {
		t.Fatalf("expected overload, got %v", err)
	}
	stats := pipeline.Snapshot()
	if stats.PendingBatches != 1 || stats.PendingBytes <= 0 || stats.PendingBytes > 32<<20 {
		t.Fatalf("invalid pending stats: %+v", stats)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if stats := pipeline.Snapshot(); stats.PendingBatches != 0 || stats.PendingBytes != 0 {
		t.Fatalf("reservation leaked: %+v", stats)
	}
}

func TestPipelineByteBudgetAndFailureRelease(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	failure := errors.New("synthetic writer failure")
	sink := sinkFunc(func(context.Context, *collectortracepb.ExportTraceServiceRequest) (Result, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return Result{}, failure
	})
	pipeline, err := NewPipeline(sink, 8, 24<<10)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := pipeline.Submit(context.Background(), fixtures.Request("simple")); done <- err }()
	<-started
	if _, err := pipeline.Submit(context.Background(), fixtures.Request("simple")); !errors.Is(err, ErrOverloaded) {
		t.Fatalf("expected byte overload, got %v", err)
	}
	close(release)
	if err := <-done; !errors.Is(err, failure) {
		t.Fatalf("lost sink failure: %v", err)
	}
	if _, err := pipeline.Submit(context.Background(), fixtures.Request("simple")); !errors.Is(err, failure) {
		t.Fatalf("reservation not released: %v", err)
	}
	if stats := pipeline.Snapshot(); stats.PendingBytes != 0 || stats.PendingBatches != 0 {
		t.Fatalf("leaked reservation: %+v", stats)
	}
	tiny, err := NewPipeline(sink, 8, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tiny.Submit(context.Background(), fixtures.Request("simple")); !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("expected permanent size limit, got %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected sink calls: %d", calls.Load())
	}
}

func TestPipelineCancellation(t *testing.T) {
	var calls atomic.Int32
	sink := sinkFunc(func(ctx context.Context, _ *collectortracepb.ExportTraceServiceRequest) (Result, error) {
		calls.Add(1)
		<-ctx.Done()
		return Result{}, ctx.Err()
	})
	pipeline, err := NewPipeline(sink, 8, 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pipeline.Submit(ctx, fixtures.Request("simple")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("sent canceled request to sink")
	}
	if _, err := pipeline.Submit(context.Background(), nil); err == nil {
		t.Fatal("accepted nil request")
	}
	for _, args := range []struct {
		batches int
		bytes   int64
	}{{0, 1}, {1, 0}, {-1, 1}} {
		if _, err := NewPipeline(sink, args.batches, args.bytes); err == nil {
			t.Fatal("accepted invalid pipeline limits")
		}
	}
	if _, err := NewPipeline(nil, 1, 1); err == nil {
		t.Fatal("accepted nil sink")
	}
}
