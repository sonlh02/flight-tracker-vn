package source

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"flighttracker/services/ingest/internal/opensky"
)

// Replay plays back a JSON Lines recording written by package record,
// keeping the original spacing between snapshots (divided by Speed).
//
// Timestamps are shifted so the first snapshot of each pass happens "now".
// Downstream services therefore see fresh data and cannot tell a replay from
// a live feed, which keeps demos independent of the OpenSky API.
type Replay struct {
	Path   string
	Speed  float64 // 1 = real time, 10 = ten times faster
	Loop   bool
	Logger *slog.Logger
}

func (r *Replay) Run(ctx context.Context, out chan<- opensky.Snapshot) error {
	for pass := 1; ; pass++ {
		sent, err := r.playOnce(ctx, out)
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if sent == 0 {
			return fmt.Errorf("replay %s: no snapshots", r.Path)
		}
		r.Logger.Info("replay finished", "pass", pass, "snapshots", sent)
		if !r.Loop {
			return nil
		}
	}
}

func (r *Replay) playOnce(ctx context.Context, out chan<- opensky.Snapshot) (sent int, err error) {
	f, err := os.Open(r.Path)
	if err != nil {
		return 0, fmt.Errorf("replay: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 32<<20) // one snapshot can hold hundreds of aircraft

	var offset, prev int64
	for line := 1; sc.Scan(); line++ {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		var snap opensky.Snapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return sent, fmt.Errorf("replay %s:%d: %w", r.Path, line, err)
		}

		if sent == 0 {
			offset = time.Now().Unix() - snap.Time
		} else if gap := snap.Time - prev; gap > 0 {
			delay := time.Duration(float64(time.Duration(gap)*time.Second) / r.Speed)
			if !sleep(ctx, delay) {
				return sent, nil
			}
		}
		prev = snap.Time

		if !send(ctx, out, shift(snap, offset)) {
			return sent, nil
		}
		sent++
	}
	if err := sc.Err(); err != nil {
		return sent, fmt.Errorf("replay %s: %w", r.Path, err)
	}
	return sent, nil
}

// shift moves every timestamp in snap forward by offset seconds.
func shift(snap opensky.Snapshot, offset int64) opensky.Snapshot {
	snap.Time += offset
	for i := range snap.Flights {
		f := &snap.Flights[i]
		if f.TimePosition != 0 {
			f.TimePosition += offset
		}
		f.LastContact += offset
	}
	return snap
}
