package source

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/opensky"
)

// StatesFetcher is the part of opensky.Client that Live needs.
type StatesFetcher interface {
	States(ctx context.Context, box opensky.BBox) (opensky.Snapshot, error)
}

// Live polls OpenSky on a fixed interval.
type Live struct {
	Client   StatesFetcher
	BBox     opensky.BBox
	Interval time.Duration
	Logger   *slog.Logger
}

func (l *Live) Run(ctx context.Context, out chan<- opensky.Snapshot) error {
	ticker := time.NewTicker(l.Interval)
	defer ticker.Stop()

	var pausedUntil time.Time
	for {
		if !time.Now().Before(pausedUntil) {
			snap, err := l.Client.States(ctx, l.BBox)
			var rateLimited *opensky.RateLimitError
			switch {
			case err == nil:
				if !send(ctx, out, snap) {
					return nil
				}
			case ctx.Err() != nil:
				return nil
			case errors.As(err, &rateLimited):
				// Skip ticks instead of hammering the API; errors on a single
				// poll are transient, so the loop keeps going either way.
				pausedUntil = time.Now().Add(rateLimited.RetryAfter)
				l.Logger.Warn("rate limited by OpenSky, pausing", "retry_after", rateLimited.RetryAfter)
			default:
				l.Logger.Error("poll failed", "err", err)
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
