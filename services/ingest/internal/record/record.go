// Package record saves snapshots to a JSON Lines file for later replay.
package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sonlh02/flight-tracker-vn/services/ingest/internal/opensky"
)

// Writer appends one snapshot per line to a file.
type Writer struct {
	f   *os.File
	enc *json.Encoder
}

// Open creates path (and its directory) or appends to it if it exists.
func Open(path string) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("record: %w", err)
	}
	return &Writer{f: f, enc: json.NewEncoder(f)}, nil
}

// Write is unbuffered on purpose: each snapshot reaches the file in a single
// write, so stopping the recorder with Ctrl+C never leaves a partial line.
func (w *Writer) Write(snap opensky.Snapshot) error {
	if err := w.enc.Encode(snap); err != nil {
		return fmt.Errorf("record: %w", err)
	}
	return nil
}

func (w *Writer) Close() error { return w.f.Close() }
