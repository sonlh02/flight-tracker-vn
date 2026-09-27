// Package source produces aircraft snapshots, either live from OpenSky or
// replayed from a recording.
package source

import (
	"context"
	"time"

	"flighttracker/services/ingest/internal/opensky"
)

// Source sends snapshots to out until ctx is cancelled or the source is
// exhausted. It returns nil on cancellation.
type Source interface {
	Run(ctx context.Context, out chan<- opensky.Snapshot) error
}

// send delivers snap unless ctx is cancelled first.
func send(ctx context.Context, out chan<- opensky.Snapshot, snap opensky.Snapshot) bool {
	select {
	case out <- snap:
		return true
	case <-ctx.Done():
		return false
	}
}

// sleep waits for d or until ctx is cancelled. It reports whether the full
// duration elapsed.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
