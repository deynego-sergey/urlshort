// src/internal/services/collector/collect.go
package collector

import (
	"context"
	"log"
	"sync"
	"time"
	"urlshort/internal/repository/mongo/stats"
)

type CollectorConfig struct {
	BatchSize     int
	FlushInterval time.Duration
}

type Collector struct {
	repo       stats.IStatsRepository
	cfg        CollectorConfig
	eventsChan chan stats.StatUpdate
	wg         sync.WaitGroup
	cancel     context.CancelFunc
}

// NewCollector -
func NewCollector(repo stats.IStatsRepository, cfg CollectorConfig) *Collector {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	return &Collector{
		repo:       repo,
		cfg:        cfg,
		eventsChan: make(chan stats.StatUpdate, cfg.BatchSize*2),
	}
}

func (c *Collector) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go c.worker(ctx)
}

func (c *Collector) Push(event stats.StatUpdate) {
	c.eventsChan <- event
}

func (c *Collector) worker(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]stats.StatUpdate, 0, c.cfg.BatchSize)

	for {
		select {
		case <-ctx.Done():
			c.flush(context.Background(), batch)
			return

		case event, ok := <-c.eventsChan:
			if !ok {
				c.flush(context.Background(), batch)
				return
			}

			batch = append(batch, event)

			if len(batch) >= c.cfg.BatchSize {
				c.flush(ctx, batch)
				batch = make([]stats.StatUpdate, 0, c.cfg.BatchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				c.flush(ctx, batch)
				batch = make([]stats.StatUpdate, 0, c.cfg.BatchSize)
			}
		}
	}
}

func (c *Collector) flush(ctx context.Context, batch []stats.StatUpdate) {
	if len(batch) == 0 {
		log.Printf("collector flush: nothing to flush")
		return
	}

	log.Printf("[DEBUG] Collector flushing batch of %d items", len(batch))

	if err := c.repo.BulkUpsert(ctx, batch); err != nil {
		log.Printf("collector error flushing stats: %v\n", err)
	}
}

func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	close(c.eventsChan)
	c.wg.Wait()
}
