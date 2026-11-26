package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

var ErrStorageFull = errors.New("storage budget cannot admit this batch; retry after retention or increase the storage budget")

const retentionQuietWindow = 30 * time.Second
const maxPrunedTraces = 64
const maxReclaimedPages = 4096

type Usage struct {
	DatabaseBytes    int64  `json:"database_bytes"`
	WALBytes         int64  `json:"wal_bytes"`
	SHMBytes         int64  `json:"shm_bytes"`
	JournalBytes     int64  `json:"journal_bytes"`
	TotalBytes       int64  `json:"total_bytes"`
	BudgetBytes      int64  `json:"budget_bytes"`
	FreePageBytes    int64  `json:"free_page_bytes"`
	TraceCount       int64  `json:"trace_count"`
	SpanCount        int64  `json:"span_count"`
	Pressure         bool   `json:"pressure"`
	CheckpointBusy   bool   `json:"checkpoint_busy"`
	LastPrunedAt     string `json:"last_pruned_at,omitempty"`
	LastPrunedTraces int64  `json:"last_pruned_traces"`
	PrunedTraces     int64  `json:"pruned_traces"`
}

// All state and maintenance are protected by Store.writer, also used by Ingest.
type retentionState struct {
	pressure       bool
	checkpointBusy bool
	lastPrunedAt   string
	lastPruned     int64
	pruned         int64
}

func (s *Store) Usage(ctx context.Context) (Usage, error) {
	if err := s.acquireWriter(ctx); err != nil {
		return Usage{}, err
	}
	defer s.releaseWriter()
	return s.usageLocked(ctx)
}

// Prune performs bounded maintenance; the owner schedules subsequent passes.
// Age is measured from the last newly committed span, never application clocks.
func (s *Store) Prune(ctx context.Context, now time.Time) (Usage, error) {
	if err := s.acquireWriter(ctx); err != nil {
		return Usage{}, err
	}
	defer s.releaseWriter()
	return s.pruneLocked(ctx, now, 0)
}

func (s *Store) acquireWriter(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.writer <- struct{}{}:
		if err := ctx.Err(); err != nil {
			s.releaseWriter()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) releaseWriter() { <-s.writer }

func (s *Store) physicalUsage() (Usage, error) {
	u := Usage{BudgetBytes: s.cfg.Storage.MaxSizeMB << 20, Pressure: s.retention.pressure,
		CheckpointBusy: s.retention.checkpointBusy, LastPrunedAt: s.retention.lastPrunedAt,
		LastPrunedTraces: s.retention.lastPruned, PrunedTraces: s.retention.pruned}
	for _, file := range []struct {
		suffix string
		bytes  *int64
	}{{"", &u.DatabaseBytes}, {"-wal", &u.WALBytes}, {"-shm", &u.SHMBytes}, {"-journal", &u.JournalBytes}} {
		info, err := os.Lstat(s.cfg.Storage.Path + file.suffix)
		if errors.Is(err, os.ErrNotExist) && file.suffix != "" {
			continue
		}
		if err != nil {
			return Usage{}, fmt.Errorf("measure database files: %w", err)
		}
		if !info.Mode().IsRegular() {
			return Usage{}, errors.New("database sidecars must be regular local files")
		}
		*file.bytes = info.Size()
		u.TotalBytes += info.Size()
	}
	u.Pressure = u.Pressure || u.TotalBytes >= u.BudgetBytes
	return u, nil
}

func (s *Store) usageLocked(ctx context.Context) (Usage, error) {
	u, err := s.physicalUsage()
	if err != nil {
		return Usage{}, err
	}
	var freePages, pageSize int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freePages); err != nil {
		return Usage{}, fmt.Errorf("measure reusable database pages: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return Usage{}, fmt.Errorf("measure database page size: %w", err)
	}
	u.FreePageBytes = freePages * pageSize
	if err := s.db.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(span_count),0) FROM traces").Scan(&u.TraceCount, &u.SpanCount); err != nil {
		return Usage{}, fmt.Errorf("measure retained traces: %w", err)
	}
	return u, nil
}

func fitsBudget(total, budget, reserve int64) bool {
	return reserve >= 0 && reserve <= budget && total <= budget-reserve
}

func (s *Store) pruneLocked(ctx context.Context, now time.Time, reserve int64) (Usage, error) {
	if now.Before(time.Unix(0, 0)) || now.After(time.Unix(0, math.MaxInt64)) {
		return Usage{}, errors.New("retention time is outside supported nanosecond range")
	}
	s.retention.lastPruned = 0
	remainingPages := maxReclaimedPages
	if err := s.reclaim(ctx, &remainingPages); err != nil {
		return Usage{}, err
	}
	quietCutoff := now.Add(-retentionQuietWindow).UnixNano()
	ageCutoff := now.Add(-time.Duration(s.cfg.Storage.RetentionDays) * 24 * time.Hour).UnixNano()
	ids, err := s.oldest(ctx, min(quietCutoff, ageCutoff), maxPrunedTraces)
	if err != nil {
		return Usage{}, err
	}
	if len(ids) > 0 {
		if err := s.deleteTraces(ctx, ids, now); err != nil {
			return Usage{}, err
		}
		if err := s.reclaim(ctx, &remainingPages); err != nil {
			return Usage{}, err
		}
	}
	for {
		u, err := s.usageLocked(ctx)
		if err != nil {
			return Usage{}, err
		}
		if fitsBudget(u.TotalBytes, u.BudgetBytes, reserve) {
			s.retention.pressure = false
			u.Pressure = false
			return u, nil
		}
		s.retention.pressure = true
		u.Pressure = true
		// Do not delete newer history while already-free pages or a held WAL
		// need another bounded maintenance pass to become physical free space.
		if s.retention.lastPruned >= maxPrunedTraces || u.CheckpointBusy || (remainingPages == 0 && u.FreePageBytes > 0) {
			return u, nil
		}
		ids, err := s.oldest(ctx, quietCutoff, 1)
		if err != nil || len(ids) == 0 {
			return u, err
		}
		if err := s.deleteTraces(ctx, ids, now); err != nil {
			return Usage{}, err
		}
		if err := s.reclaim(ctx, &remainingPages); err != nil {
			return Usage{}, err
		}
	}
}

func (s *Store) oldest(ctx context.Context, cutoff int64, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT trace_id FROM traces WHERE last_seen_ns<=? ORDER BY last_seen_ns,trace_id LIMIT ?", cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("select retention candidates: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read retention candidate: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) deleteTraces(ctx context.Context, ids []string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start retention transaction: %w", err)
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "DELETE FROM traces WHERE trace_id=?", id); err != nil {
			return fmt.Errorf("delete retained trace: %w", err)
		}
	}
	for _, query := range []string{
		"DELETE FROM resources WHERE resource_id NOT IN (SELECT resource_id FROM spans)",
		"DELETE FROM scopes WHERE scope_id NOT IN (SELECT scope_id FROM spans)",
		"UPDATE metadata SET revision=revision+1 WHERE id=1",
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("finish retention metadata: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retention: %w", err)
	}
	s.retention.lastPrunedAt = now.UTC().Format(time.RFC3339Nano)
	s.retention.lastPruned += int64(len(ids))
	s.retention.pruned += int64(len(ids))
	return nil
}

func (s *Store) reclaim(ctx context.Context, remainingPages *int) error {
	var freePages int
	if err := s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freePages); err != nil {
		return fmt.Errorf("inspect reusable database pages: %w", err)
	}
	pages := min(freePages, *remainingPages)
	if pages > 0 {
		// Drain every row: incremental_vacuum can return one row per step.
		rows, err := s.db.QueryContext(ctx, "PRAGMA incremental_vacuum("+strconv.Itoa(pages)+")")
		if err != nil {
			return fmt.Errorf("reclaim database pages: %w", err)
		}
		for rows.Next() {
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return fmt.Errorf("reclaim database pages: %w", err)
		}
		*remainingPages -= pages
	}
	var busy, logFrames, checkpointed int
	if err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("checkpoint retained database: %w", err)
	}
	s.retention.checkpointBusy = busy != 0
	return nil
}

// The serialized writer reserves a conservative growth estimate, including
// indexes and WAL page overhead. This is not an exact filesystem quota: at most
// one bounded transaction plus SQLite bookkeeping can cross the configured budget.
func (s *Store) admit(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest, spans int) error {
	reserve := 4*int64(proto.Size(request)) + int64(spans)*(16<<10) + 128<<10
	for _, resource := range request.ResourceSpans {
		serviceBytes := 0
		for _, attr := range resource.GetResource().GetAttributes() {
			if attr.Key == "service.name" {
				serviceBytes = len(attr.GetValue().GetStringValue())
				break
			}
		}
		for _, scope := range resource.ScopeSpans {
			reserve += 4 * int64(serviceBytes) * int64(len(scope.Spans))
		}
	}
	u, err := s.physicalUsage()
	if err != nil {
		return err
	}
	if fitsBudget(u.TotalBytes, u.BudgetBytes, reserve) {
		s.retention.pressure = false
		return nil
	}
	if reserve <= u.BudgetBytes {
		u, err = s.pruneLocked(ctx, time.Now(), reserve)
		if err != nil {
			return err
		}
		if fitsBudget(u.TotalBytes, u.BudgetBytes, reserve) {
			return nil
		}
	}
	s.retention.pressure = true
	return ErrStorageFull
}
