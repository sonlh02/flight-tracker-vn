package source

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/opensky"
	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/record"
)

func TestRecordThenReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec", "flights.jsonl")
	recorded := []opensky.Snapshot{
		{Time: 1000, Flights: []opensky.Flight{{ICAO24: "888152", Callsign: "HVN213", LastContact: 999, TimePosition: 998}}},
		{Time: 1010, Flights: []opensky.Flight{{ICAO24: "888152", Callsign: "HVN213", LastContact: 1009}}},
	}

	w, err := record.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range recorded {
		if err := w.Write(s); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()

	r := &Replay{Path: path, Speed: 1000, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	out := make(chan opensky.Snapshot, len(recorded))
	start := time.Now().Unix()
	if err := r.Run(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var got []opensky.Snapshot
	for s := range out {
		got = append(got, s)
	}
	if len(got) != 2 {
		t.Fatalf("replayed %d snapshots, want 2", len(got))
	}

	// Timestamps are rebased to now while keeping their relative spacing.
	if got[0].Time < start || got[0].Time > time.Now().Unix() {
		t.Errorf("first snapshot time %d not rebased to now (%d)", got[0].Time, start)
	}
	offset := got[0].Time - 1000
	if got[1].Time-got[0].Time != 10 {
		t.Errorf("spacing = %d, want 10", got[1].Time-got[0].Time)
	}
	f := got[0].Flights[0]
	if f.LastContact != 999+offset || f.TimePosition != 998+offset {
		t.Errorf("flight timestamps not shifted: %+v", f)
	}
	if got[1].Flights[0].TimePosition != 0 {
		t.Errorf("unknown TimePosition should stay 0, got %d", got[1].Flights[0].TimePosition)
	}
}

func TestReplayStopsOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flights.jsonl")
	w, err := record.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(opensky.Snapshot{Time: 1000})
	w.Write(opensky.Snapshot{Time: 4600}) // an hour later at 1x speed
	w.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r := &Replay{Path: path, Speed: 1, Loop: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	out := make(chan opensky.Snapshot, 10)

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, out) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context was cancelled")
	}
}
