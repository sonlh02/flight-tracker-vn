package opensky

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Flight is one aircraft state vector from OpenSky, trimmed to the fields we use.
// Optional values that OpenSky may report as null are pointers.
type Flight struct {
	ICAO24        string   `json:"icao24"`
	Callsign      string   `json:"callsign,omitempty"`
	OriginCountry string   `json:"origin_country"`
	TimePosition  int64    `json:"time_position,omitempty"` // unix seconds, 0 if unknown
	LastContact   int64    `json:"last_contact"`            // unix seconds
	Longitude     float64  `json:"longitude"`
	Latitude      float64  `json:"latitude"`
	BaroAltitude  *float64 `json:"baro_altitude,omitempty"` // metres
	GeoAltitude   *float64 `json:"geo_altitude,omitempty"`  // metres
	OnGround      bool     `json:"on_ground"`
	Velocity      *float64 `json:"velocity,omitempty"`      // m/s over ground
	TrueTrack     *float64 `json:"true_track,omitempty"`    // degrees clockwise from north
	VerticalRate  *float64 `json:"vertical_rate,omitempty"` // m/s, positive = climbing
	Squawk        string   `json:"squawk,omitempty"`
}

// Snapshot is the state of every tracked aircraft at one point in time.
type Snapshot struct {
	Time    int64    `json:"time"` // unix seconds
	Flights []Flight `json:"flights"`
}

// Column indexes of a state vector in the /states/all response.
// See https://openskynetwork.github.io/opensky-api/rest.html#all-state-vectors
const (
	colICAO24 = iota
	colCallsign
	colOriginCountry
	colTimePosition
	colLastContact
	colLongitude
	colLatitude
	colBaroAltitude
	colOnGround
	colVelocity
	colTrueTrack
	colVerticalRate
	colSensors
	colGeoAltitude
	colSquawk
	colSPI
	colPositionSource
	minColumns
)

type statesResponse struct {
	Time   int64               `json:"time"`
	States [][]json.RawMessage `json:"states"` // null when no aircraft are in range
}

// parseStates decodes a /states/all response body. Aircraft without a known
// position are dropped since they cannot be shown on a map.
func parseStates(body []byte) (Snapshot, error) {
	var resp statesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Snapshot{}, fmt.Errorf("opensky: decode states: %w", err)
	}

	snap := Snapshot{Time: resp.Time, Flights: make([]Flight, 0, len(resp.States))}
	for i, row := range resp.States {
		f, ok, err := parseStateVector(row)
		if err != nil {
			return Snapshot{}, fmt.Errorf("opensky: state vector %d: %w", i, err)
		}
		if ok {
			snap.Flights = append(snap.Flights, f)
		}
	}
	return snap, nil
}

// parseStateVector converts one heterogeneous JSON array into a Flight.
// ok is false when the aircraft has no position.
func parseStateVector(row []json.RawMessage) (f Flight, ok bool, err error) {
	if len(row) < minColumns {
		return Flight{}, false, fmt.Errorf("expected at least %d columns, got %d", minColumns, len(row))
	}

	d := &rowDecoder{row: row}
	icao24 := decode[string](d, colICAO24)
	callsign := decode[string](d, colCallsign)
	country := decode[string](d, colOriginCountry)
	timePosition := decode[int64](d, colTimePosition)
	lastContact := decode[int64](d, colLastContact)
	lon := decode[float64](d, colLongitude)
	lat := decode[float64](d, colLatitude)
	onGround := decode[bool](d, colOnGround)
	squawk := decode[string](d, colSquawk)
	f = Flight{
		BaroAltitude: decode[float64](d, colBaroAltitude),
		GeoAltitude:  decode[float64](d, colGeoAltitude),
		Velocity:     decode[float64](d, colVelocity),
		TrueTrack:    decode[float64](d, colTrueTrack),
		VerticalRate: decode[float64](d, colVerticalRate),
	}
	if d.err != nil {
		return Flight{}, false, d.err
	}
	if icao24 == nil || *icao24 == "" {
		return Flight{}, false, fmt.Errorf("missing icao24")
	}
	if lon == nil || lat == nil {
		return Flight{}, false, nil
	}

	f.ICAO24 = *icao24
	f.Callsign = strings.TrimSpace(deref(callsign)) // OpenSky pads callsigns to 8 chars
	f.OriginCountry = deref(country)
	f.TimePosition = deref(timePosition)
	f.LastContact = deref(lastContact)
	f.Longitude = *lon
	f.Latitude = *lat
	f.OnGround = deref(onGround)
	f.Squawk = deref(squawk)
	return f, true, nil
}

// rowDecoder remembers the first decoding error so a row can be decoded
// field by field without checking an error after every column.
type rowDecoder struct {
	row []json.RawMessage
	err error
}

// decode returns column i as *T, or nil if the value is JSON null.
func decode[T any](d *rowDecoder, i int) *T {
	if d.err != nil {
		return nil
	}
	var v *T
	if err := json.Unmarshal(d.row[i], &v); err != nil {
		d.err = fmt.Errorf("column %d: %w", i, err)
		return nil
	}
	return v
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
