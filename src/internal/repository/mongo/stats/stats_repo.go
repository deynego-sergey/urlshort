// src/internal/repository/mongo/stats/stats_repo.go
package stats

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"urlshort/pkg/database/mongoatlas"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type AggregatedUpdate struct {
	SourceURL         string
	TargetURL         string
	TotalClicks       int64
	LastClickedAt     time.Time
	EarliestTimestamp time.Time
	DailyStats        map[string]int64
	ClicksBy15min     map[int]int64
	ClicksByDayOfWeek map[int]int64
	Referrers         map[string]int64
	Countries         map[string]int64
	Devices           map[string]int64
}

type IStatsRepository interface {
	GetBySourceURL(ctx context.Context, sourceURL string) (*RedirectStat, error)
	GetBySourceURLs(ctx context.Context, sourceURLs []string) ([]RedirectStat, error)
	BulkUpsertAggregated(ctx context.Context, aggregatedMap map[string]*AggregatedUpdate) error
}

type MongoStatsRepository struct {
	coll *mongo.Collection
}

func NewMongoStatsRepository(client *mongoatlas.Client, collectionName string) *MongoStatsRepository {
	return &MongoStatsRepository{
		coll: client.Database().Collection(collectionName),
	}
}

func (r *MongoStatsRepository) GetBySourceURL(ctx context.Context, sourceURL string) (*RedirectStat, error) {
	filter := bson.D{{Key: "source_url", Value: sourceURL}}

	var stat RedirectStat
	err := r.coll.FindOne(ctx, filter).Decode(&stat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get stat by source url: %w", err)
	}

	return &stat, nil
}

func (r *MongoStatsRepository) GetBySourceURLs(ctx context.Context, sourceURLs []string) ([]RedirectStat, error) {
	if len(sourceURLs) == 0 {
		return []RedirectStat{}, nil
	}

	filter := bson.D{{Key: "source_url", Value: bson.D{{Key: "$in", Value: sourceURLs}}}}

	cursor, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find stats by source urls: %w", err)
	}
	defer cursor.Close(ctx)

	var stats []RedirectStat
	if err := cursor.All(ctx, &stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats list: %w", err)
	}

	return stats, nil
}

func (r *MongoStatsRepository) BulkUpsertAggregated(ctx context.Context, aggregatedMap map[string]*AggregatedUpdate) error {
	if len(aggregatedMap) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, len(aggregatedMap))

	for sourceURL, agg := range aggregatedMap {
		filter := bson.D{{Key: "source_url", Value: sourceURL}}

		incDoc := bson.D{
			{Key: "total_clicks", Value: agg.TotalClicks},
		}

		for day, count := range agg.DailyStats {
			incDoc = append(incDoc, bson.E{Key: "daily_stats." + day, Value: count})
		}
		for idx, count := range agg.ClicksBy15min {
			incDoc = append(incDoc, bson.E{Key: "clicks_by_15min." + strconv.Itoa(idx), Value: count})
		}
		for idx, count := range agg.ClicksByDayOfWeek {
			incDoc = append(incDoc, bson.E{Key: "clicks_by_day_of_week." + strconv.Itoa(idx), Value: count})
		}
		for ref, count := range agg.Referrers {
			incDoc = append(incDoc, bson.E{Key: "referrers." + ref, Value: count})
		}
		for country, count := range agg.Countries {
			incDoc = append(incDoc, bson.E{Key: "countries." + country, Value: count})
		}
		for device, count := range agg.Devices {
			incDoc = append(incDoc, bson.E{Key: "devices." + device, Value: count})
		}

		setDoc := bson.D{
			{Key: "last_clicked_at", Value: agg.LastClickedAt},
		}
		if agg.TargetURL != "" {
			setDoc = append(setDoc, bson.E{Key: "target_url", Value: agg.TargetURL})
		}

		setOnInsertDoc := bson.D{
			{Key: "created_at", Value: agg.EarliestTimestamp},
		}

		updateDoc := bson.D{
			{Key: "$inc", Value: incDoc},
			{Key: "$set", Value: setDoc},
			{Key: "$setOnInsert", Value: setOnInsertDoc},
		}

		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(updateDoc).
			SetUpsert(true)

		models = append(models, model)
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := r.coll.BulkWrite(ctx, models, opts)
	if err != nil {
		return fmt.Errorf("failed to execute bulk upsert: %w", err)
	}

	return nil
}
