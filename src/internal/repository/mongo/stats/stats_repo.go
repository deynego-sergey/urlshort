// src/internal/repository/mongo/stats/stats_repo.go
package stats

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"urlshort/pkg/database/mongoatlas"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type IStatsRepository interface {
	GetBySourceURL(ctx context.Context, sourceURL string) (*RedirectStat, error)
	GetBySourceURLs(ctx context.Context, sourceURLs []string) ([]RedirectStat, error)
	BulkUpsert(ctx context.Context, updates []StatUpdate) error
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

// AggregatedUpdate хранит свернутые в памяти метрики по одному source_url
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
}

func (r *MongoStatsRepository) BulkUpsert(ctx context.Context, updates []StatUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	aggregatedMap := make(map[string]*AggregatedUpdate)

	// 1. Пред-агрегация батча в памяти
	for _, update := range updates {
		sourceURL := update.URLPath
		if sourceURL == "" {
			continue
		}

		agg, exists := aggregatedMap[sourceURL]
		if !exists {
			agg = &AggregatedUpdate{
				SourceURL:         sourceURL,
				TargetURL:         update.TargetURL,
				LastClickedAt:     update.Timestamp,
				EarliestTimestamp: update.Timestamp,
				DailyStats:        make(map[string]int64),
				ClicksBy15min:     make(map[int]int64),
				ClicksByDayOfWeek: make(map[int]int64),
				Referrers:         make(map[string]int64),
			}
			aggregatedMap[sourceURL] = agg
		}

		agg.TotalClicks++

		if update.Timestamp.After(agg.LastClickedAt) {
			agg.LastClickedAt = update.Timestamp
			if update.TargetURL != "" {
				agg.TargetURL = update.TargetURL
			}
		}
		if update.Timestamp.Before(agg.EarliestTimestamp) {
			agg.EarliestTimestamp = update.Timestamp
		}

		// Посуточная статистика ("YYYY-MM-DD")
		dayKey := update.Timestamp.Format("2006-01-02")
		agg.DailyStats[dayKey]++

		// 15-минутные интервалы и дни недели
		idx15min := (update.Timestamp.Hour()*60 + update.Timestamp.Minute()) / 15
		idxDayOfWeek := (int(update.Timestamp.Weekday()) + 6) % 7
		agg.ClicksBy15min[idx15min]++
		agg.ClicksByDayOfWeek[idxDayOfWeek]++

		// Обработка Referer
		if ref := sanitizeReferrer(update.Referer()); ref != "" {
			agg.Referrers[ref]++
		}
	}

	if len(aggregatedMap) == 0 {
		return nil
	}

	// 2. Формирование запросов в MongoDB
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

func sanitizeReferrer(rawRef string) string {
	if rawRef == "" {
		return ""
	}
	parsed, err := url.Parse(rawRef)
	if err == nil && parsed.Host != "" {
		rawRef = parsed.Host
	}
	rawRef = strings.ReplaceAll(rawRef, ".", "_")
	rawRef = strings.ReplaceAll(rawRef, "$", "_")
	return rawRef
}
