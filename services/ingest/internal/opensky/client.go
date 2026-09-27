// Package opensky is a minimal client for the OpenSky Network REST API.
package opensky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL  = "https://opensky-network.org/api"
	DefaultTokenURL = "https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token"

	maxResponseBytes  = 16 << 20
	defaultRetryAfter = time.Minute
)

// Config configures a Client. Zero values select sensible defaults.
type Config struct {
	BaseURL      string
	TokenURL     string
	ClientID     string // empty means anonymous access
	ClientSecret string
	HTTPClient   *http.Client
}

// Client fetches aircraft state vectors from OpenSky.
type Client struct {
	baseURL string
	http    *http.Client
	tokens  *tokenSource // nil for anonymous access
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = DefaultTokenURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}

	c := &Client{baseURL: strings.TrimRight(cfg.BaseURL, "/"), http: cfg.HTTPClient}
	if cfg.ClientID != "" {
		c.tokens = &tokenSource{
			url:          cfg.TokenURL,
			clientID:     cfg.ClientID,
			clientSecret: cfg.ClientSecret,
			http:         cfg.HTTPClient,
		}
	}
	return c
}

// Authenticated reports whether the client sends OAuth2 credentials.
func (c *Client) Authenticated() bool { return c.tokens != nil }

// RateLimitError is returned when OpenSky answers 429 Too Many Requests.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("opensky: rate limited, retry after %s", e.RetryAfter)
}

// States returns all aircraft with a known position inside box.
func (c *Client) States(ctx context.Context, box BBox) (Snapshot, error) {
	q := url.Values{}
	q.Set("lamin", formatCoord(box.LaMin))
	q.Set("lomin", formatCoord(box.LoMin))
	q.Set("lamax", formatCoord(box.LaMax))
	q.Set("lomax", formatCoord(box.LoMax))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/states/all?"+q.Encode(), nil)
	if err != nil {
		return Snapshot{}, err
	}
	if c.tokens != nil {
		token, err := c.tokens.Token(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("opensky: get states: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if err != nil {
			return Snapshot{}, fmt.Errorf("opensky: read states: %w", err)
		}
		return parseStates(body)
	case http.StatusTooManyRequests:
		return Snapshot{}, &RateLimitError{RetryAfter: retryAfter(resp.Header)}
	case http.StatusUnauthorized, http.StatusForbidden:
		// The token may have been revoked early; fetch a fresh one next time.
		if c.tokens != nil {
			c.tokens.invalidate()
		}
	}
	return Snapshot{}, fmt.Errorf("opensky: get states: %s: %s", resp.Status, bodySnippet(resp.Body))
}

func formatCoord(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(h.Get("X-Rate-Limit-Retry-After-Seconds"))
	if err != nil || secs <= 0 {
		return defaultRetryAfter
	}
	return time.Duration(secs) * time.Second
}

func bodySnippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 512))
	return strings.TrimSpace(string(b))
}

// tokenSource fetches and caches an OAuth2 access token using the
// client credentials grant.
type tokenSource struct {
	url, clientID, clientSecret string
	http                        *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// expiryMargin refreshes tokens slightly early so a request never carries
// a token that expires in flight.
const expiryMargin = 30 * time.Second

func (ts *tokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.token != "" && time.Now().Before(ts.expiry) {
		return ts.token, nil
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {ts.clientID},
		"client_secret": {ts.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.url, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ts.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("opensky: fetch token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("opensky: fetch token: %s: %s", resp.Status, bodySnippet(resp.Body))
	}

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // seconds
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("opensky: decode token: %w", err)
	}
	if body.AccessToken == "" {
		return "", errors.New("opensky: token response has no access_token")
	}

	ts.token = body.AccessToken
	ts.expiry = time.Now().Add(time.Duration(body.ExpiresIn)*time.Second - expiryMargin)
	return ts.token, nil
}

func (ts *tokenSource) invalidate() {
	ts.mu.Lock()
	ts.token = ""
	ts.mu.Unlock()
}
