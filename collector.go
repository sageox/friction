package frictionax

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/sageox/frictionax/internal/ringbuffer"
	"github.com/sageox/frictionax/internal/throttle"
)

// collectorConfig configures the frictionCollector.
type collectorConfig struct {
	Endpoint         string
	Version          string
	AuthFunc         func() string
	RequestDecorator func(*http.Request)
	Logger           *slog.Logger
	BufferSize       int
	FlushInterval    time.Duration
	BatchThreshold   int
	FlushCooldown    time.Duration
	CachePath        string
	IsEnabled        func() bool
}

// frictionCollector manages friction event buffering and transmission.
type frictionCollector struct {
	mu           sync.Mutex
	buffer       *ringbuffer.RingBuffer[FrictionEvent]
	client       *client
	catalogCache *catalogCache
	throttle     *throttle.Throttle

	enabled        bool
	shutdown       chan struct{}
	stopped        sync.Once
	wg             sync.WaitGroup
	logger         *slog.Logger
	flushInterval  time.Duration
	batchThreshold int
}

// newFrictionCollector creates a new friction event collector.
func newFrictionCollector(cfg collectorConfig) *frictionCollector {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 15 * time.Minute
	}
	if cfg.BatchThreshold <= 0 {
		cfg.BatchThreshold = 20
	}
	if cfg.FlushCooldown <= 0 {
		cfg.FlushCooldown = 15 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	enabled := true
	if cfg.IsEnabled != nil {
		enabled = cfg.IsEnabled()
	}

	var cache *catalogCache
	if cfg.CachePath != "" {
		cache = newCatalogCache(cfg.CachePath)
	}

	fc := &frictionCollector{
		buffer: ringbuffer.New(cfg.BufferSize, func(e FrictionEvent) string {
			return string(e.Kind) + ":" + e.Input
		}),
		catalogCache:   cache,
		throttle:       throttle.New(cfg.FlushCooldown),
		enabled:        enabled,
		shutdown:       make(chan struct{}),
		logger:         cfg.Logger,
		flushInterval:  cfg.FlushInterval,
		batchThreshold: cfg.BatchThreshold,
	}

	fc.client = newClient(clientConfig{
		Endpoint:         cfg.Endpoint,
		Version:          cfg.Version,
		RequestDecorator: cfg.RequestDecorator,
		AuthFunc: func() string {
			if cfg.AuthFunc != nil {
				return cfg.AuthFunc()
			}
			return ""
		},
	})

	return fc
}

// Start begins background processing of friction events.
func (f *frictionCollector) Start() {
	if !f.enabled {
		return
	}

	if f.catalogCache != nil {
		if err := f.catalogCache.Load(); err != nil {
			f.logger.Debug("failed to load catalog cache", "error", err)
		} else if v := f.catalogCache.Version(); v != "" {
			f.logger.Debug("loaded catalog cache", "version", v)
		}
	}

	f.wg.Add(1)
	go f.backgroundSender()
}

// Stop gracefully shuts down the friction collector.
func (f *frictionCollector) Stop() {
	if !f.enabled {
		return
	}

	f.stopped.Do(func() {
		close(f.shutdown)
	})
	f.wg.Wait()
}

// Record adds a friction event to the buffer.
func (f *frictionCollector) Record(event FrictionEvent) {
	if !f.enabled {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	event.Truncate()

	f.buffer.Add(event)

	if f.buffer.Count() >= f.batchThreshold && f.throttle.TryFlush() {
		select {
		case <-f.shutdown:
		default:
			go f.flush()
		}
	}
}

func (f *frictionCollector) backgroundSender() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.flush()
		case <-f.shutdown:
			f.flush()
			return
		}
	}
}

func (f *frictionCollector) flush() {
	if !f.client.ShouldSend() {
		f.logger.Debug("friction flush skipped due to rate limiting",
			"sample_rate", f.client.SampleRate(),
			"retry_after", f.client.RetryAfter())
		return
	}

	events := f.buffer.Drain()
	if len(events) == 0 {
		return
	}

	f.throttle.RecordFlush()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var opts *submitOptions
	if f.catalogCache != nil {
		opts = &submitOptions{
			CatalogVersion: f.catalogCache.Version(),
		}
	}

	resp, err := f.client.Submit(ctx, events, opts)
	if err != nil {
		f.logger.Debug("friction submit failed, re-queuing events", "error", err, "events", len(events))
		f.buffer.Refill(events)
		return
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		f.logger.Debug("friction events sent", "events", len(events), "status", resp.StatusCode)

		if resp.Catalog != nil && f.catalogCache != nil {
			if _, err := f.catalogCache.Update(resp.Catalog); err != nil {
				f.logger.Debug("failed to update catalog cache", "error", err)
			}
		}
	} else {
		f.logger.Debug("friction submit returned non-success, re-queuing events", "events", len(events), "status", resp.StatusCode)
		f.buffer.Refill(events)
	}
}

// IsEnabled returns whether friction collection is enabled.
func (f *frictionCollector) IsEnabled() bool {
	return f.enabled
}

// CatalogVersion returns the current cached catalog version.
func (f *frictionCollector) CatalogVersion() string {
	if f.catalogCache == nil {
		return ""
	}
	return f.catalogCache.Version()
}

// CatalogData returns the current cached catalog data.
func (f *frictionCollector) CatalogData() *CatalogData {
	if f.catalogCache == nil {
		return nil
	}
	return f.catalogCache.Data()
}

// UpdateCatalog updates the catalog cache with new data.
func (f *frictionCollector) UpdateCatalog(cat *CatalogData) (bool, error) {
	if f.catalogCache == nil {
		return false, nil
	}
	return f.catalogCache.Update(cat)
}

// frictionStats holds friction statistics for status display (internal).
type frictionStats struct {
	Enabled        bool      `json:"enabled"`
	BufferCount    int       `json:"buffer_count"`
	BufferSize     int       `json:"buffer_size"`
	SampleRate     float64   `json:"sample_rate"`
	RetryAfter     time.Time `json:"retry_after"`
	CatalogVersion string    `json:"catalog_version,omitempty"`
}

// stats returns current friction stats.
func (f *frictionCollector) stats() frictionStats {
	s := frictionStats{
		Enabled:     f.enabled,
		BufferCount: f.buffer.Count(),
		BufferSize:  f.buffer.Capacity(),
		SampleRate:  f.client.SampleRate(),
		RetryAfter:  f.client.RetryAfter(),
	}
	if f.catalogCache != nil {
		s.CatalogVersion = f.catalogCache.Version()
	}
	return s
}
