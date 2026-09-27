package opensky

import (
	"os"
	"strings"
	"testing"
)

func TestParseStates(t *testing.T) {
	const row = `["888152","HVN213  ","Viet Nam",1727399998,1727399999,105.8412,21.2187,10668.0,false,236.5,178.3,0.0,null,10972.8,"2034",false,0]`

	tests := []struct {
		name        string
		body        string
		wantFlights int
		wantErr     string
	}{
		{name: "one flight", body: `{"time":1,"states":[` + row + `]}`, wantFlights: 1},
		{name: "null states means empty sky", body: `{"time":1,"states":null}`, wantFlights: 0},
		{name: "extended row with category", body: `{"time":1,"states":[` + strings.TrimSuffix(row, "]") + `,3]]}`, wantFlights: 1},
		{
			name:        "no position is skipped",
			body:        `{"time":1,"states":[["7c1234","","Australia",null,1,null,null,null,false,null,null,null,null,null,null,false,0]]}`,
			wantFlights: 0,
		},
		{name: "short row", body: `{"time":1,"states":[["888152","HVN213"]]}`, wantErr: "at least 17 columns"},
		{
			name:    "wrong type",
			body:    `{"time":1,"states":[["888152","HVN213","Viet Nam",1,1,"east",21.2,null,false,null,null,null,null,null,null,false,0]]}`,
			wantErr: "column 5",
		},
		{
			name:    "missing icao24",
			body:    `{"time":1,"states":[[null,"HVN213","Viet Nam",1,1,105.8,21.2,null,false,null,null,null,null,null,null,false,0]]}`,
			wantErr: "missing icao24",
		},
		{name: "not json", body: `<html>`, wantErr: "decode states"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, err := parseStates([]byte(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(snap.Flights) != tt.wantFlights {
				t.Fatalf("got %d flights, want %d", len(snap.Flights), tt.wantFlights)
			}
		})
	}
}

func TestParseStatesFields(t *testing.T) {
	body, err := os.ReadFile("testdata/states.json")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := parseStates(body)
	if err != nil {
		t.Fatal(err)
	}

	if snap.Time != 1727400000 {
		t.Errorf("Time = %d", snap.Time)
	}
	if len(snap.Flights) != 2 {
		t.Fatalf("got %d flights, want 2 (the one without position is dropped)", len(snap.Flights))
	}

	f := snap.Flights[0]
	if f.ICAO24 != "888152" || f.Callsign != "HVN213" || f.OriginCountry != "Viet Nam" {
		t.Errorf("identity = %q %q %q", f.ICAO24, f.Callsign, f.OriginCountry)
	}
	if f.Latitude != 21.2187 || f.Longitude != 105.8412 {
		t.Errorf("position = %v,%v", f.Latitude, f.Longitude)
	}
	if f.BaroAltitude == nil || *f.BaroAltitude != 10668 {
		t.Errorf("BaroAltitude = %v", f.BaroAltitude)
	}
	if f.Squawk != "2034" || f.OnGround {
		t.Errorf("Squawk = %q, OnGround = %v", f.Squawk, f.OnGround)
	}

	g := snap.Flights[1]
	if !g.OnGround || g.BaroAltitude != nil || g.Squawk != "" {
		t.Errorf("nullable fields not preserved: %+v", g)
	}
}

func TestParseBBox(t *testing.T) {
	tests := []struct {
		in      string
		want    BBox
		wantErr bool
	}{
		{in: "8,102,23.5,117.5", want: VietnamBBox},
		{in: " 10 , 105 , 12 , 108 ", want: BBox{10, 105, 12, 108}},
		{in: "8,102,23.5", wantErr: true},
		{in: "8,102,x,117", wantErr: true},
		{in: "23.5,102,8,117.5", wantErr: true}, // min > max
		{in: "-95,0,10,10", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseBBox(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseBBox(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseBBox(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
