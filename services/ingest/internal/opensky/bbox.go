package opensky

import (
	"fmt"
	"strconv"
	"strings"
)

// BBox is a geographic bounding box in WGS84 decimal degrees.
type BBox struct {
	LaMin, LoMin, LaMax, LoMax float64
}

// VietnamBBox covers Vietnamese airspace (Hanoi and Ho Chi Minh FIRs) including
// the busy overflight routes above the East Sea.
//
// OpenSky charges credits by area: this box is ~240 square degrees, so each
// request costs 3 credits. Shrink it to save credits.
var VietnamBBox = BBox{LaMin: 8.0, LoMin: 102.0, LaMax: 23.5, LoMax: 117.5}

// ParseBBox parses "lamin,lomin,lamax,lomax".
func ParseBBox(s string) (BBox, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return BBox{}, fmt.Errorf("bbox %q: want lamin,lomin,lamax,lomax", s)
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return BBox{}, fmt.Errorf("bbox %q: %w", s, err)
		}
		v[i] = f
	}
	b := BBox{LaMin: v[0], LoMin: v[1], LaMax: v[2], LoMax: v[3]}
	return b, b.validate()
}

func (b BBox) validate() error {
	switch {
	case b.LaMin < -90 || b.LaMax > 90:
		return fmt.Errorf("bbox %s: latitude must be within [-90, 90]", b)
	case b.LoMin < -180 || b.LoMax > 180:
		return fmt.Errorf("bbox %s: longitude must be within [-180, 180]", b)
	case b.LaMin >= b.LaMax || b.LoMin >= b.LoMax:
		return fmt.Errorf("bbox %s: min must be less than max", b)
	}
	return nil
}

func (b BBox) String() string {
	return fmt.Sprintf("%g,%g,%g,%g", b.LaMin, b.LoMin, b.LaMax, b.LoMax)
}

// MarshalText and UnmarshalText let a BBox be used with flag.TextVar.
func (b BBox) MarshalText() ([]byte, error) { return []byte(b.String()), nil }

func (b *BBox) UnmarshalText(text []byte) error {
	parsed, err := ParseBBox(string(text))
	if err != nil {
		return err
	}
	*b = parsed
	return nil
}
