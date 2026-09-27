package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/opensky"
)

type config struct {
	Mode        string
	Interval    time.Duration
	BBox        opensky.BBox
	RecordPath  string
	ReplayPath  string
	ReplaySpeed float64
	ReplayLoop  bool
	LogLevel    slog.Level

	// Read from the environment only, so secrets never end up in shell history.
	ClientID     string
	ClientSecret string
}

// loadConfig reads settings from, in increasing priority: built-in defaults,
// INGEST_* environment variables (e.g. INGEST_REPLAY_FILE for -replay-file),
// and command-line flags.
func loadConfig(args []string) (config, error) {
	c := config{}
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.StringVar(&c.Mode, "mode", "live", "data source: live (poll OpenSky) or replay (play back a recording)")
	fs.DurationVar(&c.Interval, "interval", 15*time.Second, "live: polling interval")
	fs.TextVar(&c.BBox, "bbox", opensky.VietnamBBox, "live: bounding box as lamin,lomin,lamax,lomax")
	fs.StringVar(&c.RecordPath, "record", "", "append every snapshot to this JSON Lines file")
	fs.StringVar(&c.ReplayPath, "replay-file", "", "replay: recording to play back")
	fs.Float64Var(&c.ReplaySpeed, "replay-speed", 1, "replay: playback speed multiplier")
	fs.BoolVar(&c.ReplayLoop, "replay-loop", false, "replay: start over when the recording ends")
	fs.TextVar(&c.LogLevel, "log-level", slog.LevelInfo, "debug, info, warn or error")

	var envErr error
	fs.VisitAll(func(f *flag.Flag) {
		name := "INGEST_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v := os.Getenv(name); v != "" && envErr == nil {
			if err := f.Value.Set(v); err != nil {
				envErr = fmt.Errorf("%s: %w", name, err)
			}
		}
	})
	if envErr != nil {
		return c, envErr
	}
	if err := fs.Parse(args); err != nil {
		return c, err
	}

	c.ClientID = os.Getenv("OPENSKY_CLIENT_ID")
	c.ClientSecret = os.Getenv("OPENSKY_CLIENT_SECRET")
	return c, c.validate()
}

func (c config) validate() error {
	switch c.Mode {
	case "live":
		if c.Interval < time.Second {
			return errors.New("-interval must be at least 1s")
		}
		if c.ClientID != "" && c.ClientSecret == "" {
			return errors.New("OPENSKY_CLIENT_ID is set but OPENSKY_CLIENT_SECRET is empty")
		}
	case "replay":
		if c.ReplayPath == "" {
			return errors.New("replay mode needs -replay-file")
		}
		if c.ReplaySpeed <= 0 {
			return errors.New("-replay-speed must be positive")
		}
	default:
		return fmt.Errorf("unknown -mode %q (want live or replay)", c.Mode)
	}
	return nil
}

// loadDotEnv sets KEY=VALUE pairs from path without overriding variables
// that are already set. A missing file is not an error.
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, i+1)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
	return nil
}
