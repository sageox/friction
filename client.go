package frictionax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// clientConfig configures the friction client.
type clientConfig struct {
	Endpoint         string
	Version          string
	AuthFunc         func() string
	RequestDecorator func(*http.Request)
	HTTPClient       *http.Client
	Timeout          time.Duration
}

// client is the friction API client.
type client struct {
	config     clientConfig
	sampleRate float64
	retryAfter time.Time
	mu         sync.RWMutex
}

// submitResponse contains the response from the friction API.
type submitResponse struct {
	StatusCode int
	SampleRate float64
	RetryAfter time.Duration
	Catalog    *CatalogData
}

// submitOptions contains optional parameters for Submit.
type submitOptions struct {
	CatalogVersion string
}

// maxEventsPerRequest is the maximum number of events per submission.
const maxEventsPerRequest = 100

// newClient creates a new friction client with the given configuration.
func newClient(cfg clientConfig) *client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	return &client{
		config:     cfg,
		sampleRate: 1.0,
	}
}

// Submit sends friction events to the API.
func (c *client) Submit(ctx context.Context, events []FrictionEvent, opts *submitOptions) (*submitResponse, error) {
	if len(events) == 0 {
		events = []FrictionEvent{}
	}

	if len(events) > maxEventsPerRequest {
		events = events[:maxEventsPerRequest]
	}

	for i := range events {
		events[i].Truncate()
	}

	reqBody := SubmitRequest{
		Version: c.config.Version,
		Events:  events,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.config.Endpoint + "/api/v1/cli/friction"

	reqCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.config.RequestDecorator != nil {
		c.config.RequestDecorator(req)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Version", c.config.Version)

	if opts != nil && opts.CatalogVersion != "" {
		req.Header.Set("X-Catalog-Version", opts.CatalogVersion)
	}

	if c.config.AuthFunc != nil {
		if token := c.config.AuthFunc(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	result := &submitResponse{
		StatusCode: resp.StatusCode,
		SampleRate: -1,
	}

	c.updateFromHeaders(resp.Header, result)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		respBody, err := io.ReadAll(resp.Body)
		if err == nil && len(respBody) > 0 {
			var frictionResp FrictionResponse
			if json.Unmarshal(respBody, &frictionResp) == nil && frictionResp.Catalog != nil {
				result.Catalog = frictionResp.Catalog
			}
		}
	}

	return result, nil
}

// ShouldSend returns true if events should be sent based on current rate limiting.
func (c *client) ShouldSend() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if time.Now().Before(c.retryAfter) {
		return false
	}

	if c.sampleRate <= 0 {
		return false
	}
	if c.sampleRate >= 1.0 {
		return true
	}

	return rand.Float64() < c.sampleRate
}

// SampleRate returns the current sample rate (0.0-1.0).
func (c *client) SampleRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sampleRate
}

// RetryAfter returns the time until which requests should be delayed.
func (c *client) RetryAfter() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.retryAfter
}

func (c *client) updateFromHeaders(h http.Header, result *submitResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if rateStr := h.Get("X-Friction-Sample-Rate"); rateStr != "" {
		if rate, err := strconv.ParseFloat(rateStr, 64); err == nil {
			if rate >= 0 && rate <= 1.0 {
				c.sampleRate = rate
				result.SampleRate = rate
			}
		}
	}

	if retryStr := h.Get("Retry-After"); retryStr != "" {
		if seconds, err := strconv.Atoi(retryStr); err == nil {
			c.retryAfter = time.Now().Add(time.Duration(seconds) * time.Second)
			result.RetryAfter = time.Duration(seconds) * time.Second
		} else if t, err := http.ParseTime(retryStr); err == nil {
			c.retryAfter = t
			result.RetryAfter = time.Until(t)
		}
	}
}

// Reset clears rate limiting state.
func (c *client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sampleRate = 1.0
	c.retryAfter = time.Time{}
}

