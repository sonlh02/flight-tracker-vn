// Command ingest collects aircraft positions over Vietnam from the OpenSky
// Network (or replays a recording) and forwards them downstream.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/opensky"
	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/record"
	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/source"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := loadConfig(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	src := newSource(cfg, logger)

	var rec *record.Writer
	if cfg.RecordPath != "" {
		if rec, err = record.Open(cfg.RecordPath); err != nil {
			return err
		}
		defer rec.Close()
		logger.Info("recording snapshots", "file", cfg.RecordPath)
	}

	snaps := make(chan opensky.Snapshot)
	errc := make(chan error, 1)
	go func() {
		errc <- src.Run(ctx, snaps)
		close(snaps)
	}()

	for snap := range snaps {
		logSnapshot(logger, snap)
		if rec != nil {
			if err := rec.Write(snap); err != nil {
				return err
			}
		}
	}
	if err := <-errc; err != nil {
		return err
	}
	logger.Info("shut down cleanly")
	return nil
}

func newSource(cfg config, logger *slog.Logger) source.Source {
	if cfg.Mode == "replay" {
		logger.Info("starting replay", "file", cfg.ReplayPath, "speed", cfg.ReplaySpeed, "loop", cfg.ReplayLoop)
		return &source.Replay{Path: cfg.ReplayPath, Speed: cfg.ReplaySpeed, Loop: cfg.ReplayLoop, Logger: logger}
	}

	client := opensky.NewClient(opensky.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret})
	logger.Info("starting live polling",
		"bbox", cfg.BBox, "interval", cfg.Interval, "authenticated", client.Authenticated())
	return &source.Live{Client: client, BBox: cfg.BBox, Interval: cfg.Interval, Logger: logger}
}

func logSnapshot(logger *slog.Logger, snap opensky.Snapshot) {
	airborne := 0
	sample := make([]string, 0, 5)
	for _, f := range snap.Flights {
		if !f.OnGround {
			airborne++
		}
		if len(sample) < cap(sample) && f.Callsign != "" {
			sample = append(sample, f.Callsign)
		}
		logger.Debug("flight",
			"icao24", f.ICAO24, "callsign", f.Callsign,
			"lat", f.Latitude, "lon", f.Longitude,
			"alt_m", optional(f.BaroAltitude), "speed_ms", optional(f.Velocity),
			"track_deg", optional(f.TrueTrack), "on_ground", f.OnGround)
	}
	logger.Info("snapshot",
		"time", time.Unix(snap.Time, 0).UTC().Format(time.RFC3339),
		"flights", len(snap.Flights), "airborne", airborne,
		"sample", strings.Join(sample, ","))
}

// optional unwraps a nullable value so it logs as a number or <nil>, not a pointer.
func optional(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
