package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"urlshort/internal/repository/mongo/stats"
	"urlshort/pkg/database/mongoatlas"
)

const STATISTIC_COLLECTION string = "statistic"

/**

Statistic service

read data from unix socket and send to mongodb  results

*/

func main() {
	var mc *mongoatlas.Config
	var err error
	ctx, cf := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cf()
	if mc, err = mongoatlas.LoadConfigFromEnv(); err != nil {
		log.Fatal(err)
	}
	mclient, err := mongoatlas.NewClient(ctx, mc)
	if err != nil {
		log.Fatal(err)
	}

	repo := stats.NewMongoStatsRepository(mclient, STATISTIC_COLLECTION)
	_ = repo

	////

	<-ctx.Done()

}
