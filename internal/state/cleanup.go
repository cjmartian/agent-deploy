// Package state provides background cleanup for expired plans.
package state

import (
	"context"
	"log"
	"sync"
	"time"
)

// CleanupConfig configures the background cleanup service.
type CleanupConfig struct {
	// Interval is how often to check for expired plans (default: 1 hour)
	Interval time.Duration
	// OnCleanup is called after each cleanup with the number of plans deleted
	OnCleanup func(deleted int)
}

// DefaultCleanupConfig returns sensible defaults for cleanup.
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Interval:  1 * time.Hour,
		OnCleanup: nil,
	}
}

// CleanupService runs background cleanup of expired plans.
type CleanupService struct {
	store  *Store
	config CleanupConfig

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Stats
	totalDeleted int
	lastRun      time.Time
	lastErr      error
}

// NewCleanupService creates a new cleanup service for the given store.
func NewCleanupService(store *Store, config CleanupConfig) *CleanupService {
	if config.Interval == 0 {
		config.Interval = 1 * time.Hour
	}
	return &CleanupService{
		store:  store,
		config: config,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins the background cleanup loop.
func (c *CleanupService) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.doneCh = make(chan struct{})
	c.mu.Unlock()

	go c.runLoop(ctx)
	log.Printf("[cleanup] Started with interval %v", c.config.Interval)
	return nil
}

// Stop gracefully stops the cleanup service.
func (c *CleanupService) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	close(c.stopCh)
	<-c.doneCh

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	log.Printf("[cleanup] Stopped. Total plans deleted: %d", c.totalDeleted)
}

// IsRunning returns whether the service is active.
func (c *CleanupService) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Stats returns cleanup statistics.
func (c *CleanupService) Stats() CleanupStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CleanupStats{
		Running:      c.running,
		TotalDeleted: c.totalDeleted,
		LastRun:      c.lastRun,
		LastError:    c.lastErr,
		Interval:     c.config.Interval,
	}
}

// CleanupStats contains statistics about the cleanup service.
type CleanupStats struct {
	Running      bool          `json:"running"`
	TotalDeleted int           `json:"total_deleted"`
	LastRun      time.Time     `json:"last_run"`
	LastError    error         `json:"last_error,omitempty"`
	Interval     time.Duration `json:"interval"`
}

// CleanupNow performs an immediate cleanup outside the regular interval.
func (c *CleanupService) CleanupNow() (int, error) {
	return c.performCleanup()
}

func (c *CleanupService) runLoop(ctx context.Context) {
	defer close(c.doneCh)

	// Perform initial cleanup
	if deleted, err := c.performCleanup(); err != nil {
		log.Printf("[cleanup] Initial cleanup failed: %v", err)
	} else if deleted > 0 {
		log.Printf("[cleanup] Initial cleanup deleted %d expired plans", deleted)
	}

	ticker := time.NewTicker(c.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[cleanup] Context cancelled, stopping")
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			if deleted, err := c.performCleanup(); err != nil {
				log.Printf("[cleanup] Cleanup failed: %v", err)
			} else if deleted > 0 {
				log.Printf("[cleanup] Deleted %d expired plans", deleted)
			}
		}
	}
}

func (c *CleanupService) performCleanup() (int, error) {
	deleted, err := c.store.DeleteExpiredPlans()

	c.mu.Lock()
	c.lastRun = time.Now()
	c.lastErr = err
	if err == nil {
		c.totalDeleted += deleted
	}
	c.mu.Unlock()

	if err == nil && c.config.OnCleanup != nil {
		c.config.OnCleanup(deleted)
	}

	return deleted, err
}
