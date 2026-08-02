package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"urlshort/internal/repository/mongo/stats"
	"urlshort/internal/services/collector"
	"urlshort/pkg/database/mongoatlas"
)

const STATISTIC_COLLECTION string = "ustat"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Инициализация MongoDB Atlas
	mc, err := mongoatlas.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("failed to load mongo config: %v", err)
	}

	mclient, err := mongoatlas.NewClient(ctx, mc)
	if err != nil {
		log.Fatalf("failed to connect to mongo atlas: %v", err)
	}

	repo := stats.NewMongoStatsRepository(mclient, STATISTIC_COLLECTION)

	// 2. Запуск Collector для батчинга в Mongo
	coll := collector.NewCollector(repo, collector.CollectorConfig{
		BatchSize:     500,
		FlushInterval: 5 * time.Second,
	})
	coll.Start(ctx)
	defer coll.Stop()

	// 3. Запуск чтения Unix-сокета через httplog
	socketPath := os.Getenv("UNIX_SOCKET")
	if socketPath == "" {
		socketPath = "/tmp/stats.sock"
	}

	go func() {
		log.Printf("stats service starting listening on socket: %s", socketPath)
		if err := StartSocketAdapter(ctx, socketPath, coll); err != nil {
			log.Printf("socket adapter error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down stats service...")
}
