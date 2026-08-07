// src/internal/services/collector/collect.go
package collector

import (
	"context"
	"log"
	"sync"
	"time"
	pipeline "urlshort/internal/services/pipelines"

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
	pipeline   *pipeline.Pipeline
}

// NewCollector создает экземпляр накопителя событий
func NewCollector(repo stats.IStatsRepository, cfg CollectorConfig, pipe *pipeline.Pipeline) *Collector {
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
		pipeline:   pipe,
	}
}

func (c *Collector) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go c.worker(ctx)
}

func (c *Collector) Push(event stats.StatUpdate) {
	select {
	case c.eventsChan <- event:
		log.Printf("[COLLECTOR PUSH OK] Event queued: %s", event.URLPath)
	default:
		log.Printf("[COLLECTOR WARN] Buffer full, dropped event: %s", event.URLPath)
	}
}

func (c *Collector) worker(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]stats.StatUpdate, 0, c.cfg.BatchSize)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[COLLECTOR SHUTDOWN] Context done, flushing %d items", len(batch))
			c.flush(context.Background(), batch)
			return

		case event, ok := <-c.eventsChan:
			if !ok {
				log.Printf("[COLLECTOR SHUTDOWN] Channel closed, flushing %d items", len(batch))
				c.flush(context.Background(), batch)
				return
			}

			batch = append(batch, event)

			if len(batch) >= c.cfg.BatchSize {
				log.Printf("[COLLECTOR FLUSH] Batch limit reached (%d items)", len(batch))
				c.flush(ctx, batch)
				batch = make([]stats.StatUpdate, 0, c.cfg.BatchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				log.Printf("[COLLECTOR FLUSH] Timer triggered (%d items)", len(batch))
				c.flush(ctx, batch)
				batch = make([]stats.StatUpdate, 0, c.cfg.BatchSize)
			}
		}
	}
}

func (c *Collector) flush(ctx context.Context, batch []stats.StatUpdate) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[COLLECTOR PANIC] recovered in flush: %v", r)
		}
	}()

	if len(batch) > 0 {
		log.Printf("[COLLECTOR EXEC] Flushing batch of %d items to Repo", len(batch))
		agg := c.pipeline.Process(batch)
		if err := c.repo.BulkUpsertAggregated(ctx, agg); err != nil {
			log.Printf("[COLLECTOR ERROR] Error flushing stats: %v\n", err)
		} else {
			log.Printf("[COLLECTOR SUCCESS] Flushed %d items to Repo", len(batch))
		}
	}
}

func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	close(c.eventsChan)
	c.wg.Wait()
}
