package opensky

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientStatesAuthenticated(t *testing.T) {
	body, err := os.ReadFile("testdata/states.json")
	if err != nil {
		t.Fatal(err)
	}

	var tokenCalls atomic.Int32
	var (
		mu                sync.Mutex
		gotAuth, gotQuery string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		if r.FormValue("grant_type") != "client_credentials" || r.FormValue("client_id") != "id" {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"access_token":"tok-1","expires_in":1800}`))
	})
	mux.HandleFunc("GET /api/states/all", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth, gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		mu.Unlock()
		w.Write(body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL + "/api", TokenURL: srv.URL + "/token", ClientID: "id", ClientSecret: "secret"})
	for range 2 {
		snap, err := c.States(context.Background(), VietnamBBox)
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.Flights) != 2 {
			t.Fatalf("got %d flights, want 2", len(snap.Flights))
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer tok-1" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if want := "lamax=23.5&lamin=8&lomax=117.5&lomin=102"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
	if n := tokenCalls.Load(); n != 1 {
		t.Errorf("token fetched %d times, want 1 (cached)", n)
	}
}

func TestClientStatesRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("anonymous client sent Authorization header")
		}
		w.Header().Set("X-Rate-Limit-Retry-After-Seconds", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := NewClient(Config{BaseURL: srv.URL}).States(context.Background(), VietnamBBox)
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want *RateLimitError", err)
	}
	if rl.RetryAfter != 42*time.Second {
		t.Errorf("RetryAfter = %s, want 42s", rl.RetryAfter)
	}
}

func TestClientStatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "maintenance", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := NewClient(Config{BaseURL: srv.URL}).States(context.Background(), VietnamBBox)
	if err == nil {
		t.Fatal("expected error")
	}
}
