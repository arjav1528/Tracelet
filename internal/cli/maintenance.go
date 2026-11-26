package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/arjav1528/Tracelet/internal/storage"
)

func startMaintenance(ctx context.Context, store *storage.Store, output io.Writer) (func() error, error) {
	initial, cancel := context.WithTimeout(ctx, 15*time.Second)
	usage, err := store.Prune(initial, time.Now())
	cancel()
	if err != nil {
		return nil, err
	}
	if err = reportMaintenance(output, usage, nil); err != nil {
		return nil, err
	}
	// Ownership, not child cancellation, bounds maintenance; stop before closing storage.
	running, stop := context.WithCancel(context.Background())
	ticker := time.NewTicker(time.Minute)
	done := make(chan error, 1)
	go func() { defer ticker.Stop(); done <- maintain(running, store, ticker.C, output) }()
	return func() error { stop(); return <-done }, nil
}

func maintain(ctx context.Context, store *storage.Store, ticks <-chan time.Time, output io.Writer) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case now, ok := <-ticks:
			if !ok {
				return nil
			}
			bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
			usage, err := store.Prune(bounded, now)
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			if err = reportMaintenance(output, usage, err); err != nil {
				return err
			}
		}
	}
}

func reportMaintenance(output io.Writer, usage storage.Usage, pruneErr error) error {
	if pruneErr != nil {
		_, err := fmt.Fprintln(output, "Tracelet storage maintenance failed; committed data is preserved. Check free disk space and run tracelet doctor.")
		return err
	}
	if usage.LastPrunedTraces > 0 {
		_, err := fmt.Fprintf(output, "Tracelet retention pruned %d trace(s); local storage uses %d bytes.\n", usage.LastPrunedTraces, usage.TotalBytes)
		return err
	}
	if usage.Pressure {
		_, err := fmt.Fprintln(output, "Tracelet storage budget is under pressure; new batches may be rejected until space is available.")
		return err
	}
	return nil
}
