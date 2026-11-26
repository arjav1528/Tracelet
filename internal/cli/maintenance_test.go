package cli

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/arjav1528/Tracelet/internal/storage"
	"github.com/arjav1528/Tracelet/testdata/fixtures"
)

func TestMaintenancePrunesAtStartupAndStops(t *testing.T) {
	_, cfg := sessionConfig(t)
	ctx := context.Background()
	lock, err := acquireProject(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.Ingest(ctx, fixtures.Request("simple")); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE traces SET last_seen_ns=?", time.Now().Add(-8*24*time.Hour).UnixNano()); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	stop, err := startMaintenance(ctx, store, &output)
	if err != nil {
		t.Fatal(err)
	}
	if err = stop(); err != nil {
		t.Fatal(err)
	}
	usage, err := store.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TraceCount != 0 || !strings.Contains(output.String(), "pruned 1 trace") {
		t.Fatalf("usage=%+v output=%s", usage, output.String())
	}
}

func TestPeriodicMaintenanceUsesTickTime(t *testing.T) {
	_, cfg := sessionConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lock, err := acquireProject(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.Ingest(ctx, fixtures.Request("simple")); err != nil {
		t.Fatal(err)
	}
	ticks := make(chan time.Time)
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- maintain(ctx, store, ticks, &output) }()
	ticks <- time.Now().Add(8 * 24 * time.Hour)
	// Sending another tick waits until the first prune/report completed.
	ticks <- time.Now().Add(8 * 24 * time.Hour)
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	usage, err := store.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.TraceCount != 0 || usage.PrunedTraces != 1 {
		t.Fatalf("usage=%+v", usage)
	}
}
