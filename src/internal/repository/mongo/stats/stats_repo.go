package stats

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

func (r *MongoStatsRepository) BulkUpsert(ctx context.Context, updates []StatUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, len(updates))

	for _, update := range updates {
		sourceURL := update.URLPath
		targetURL := update.TargetURL
		timestamp := update.Timestamp
		referrer := update.Referer()

		filter := bson.D{{Key: "source_url", Value: sourceURL}}

		idx15min := (timestamp.Hour()*60 + timestamp.Minute()) / 15
		idxDayOfWeek := (int(timestamp.Weekday()) + 6) % 7

		incDoc := bson.D{
			{Key: "total_clicks", Value: 1},
			{Key: "clicks_by_15min." + strconv.Itoa(idx15min), Value: 1},
			{Key: "clicks_by_day_of_week." + strconv.Itoa(idxDayOfWeek), Value: 1},
		}

		if referrer != "" {
			safeReferrer := strings.ReplaceAll(referrer, ".", "_")
			safeReferrer = strings.ReplaceAll(safeReferrer, "$", "_")
			incDoc = append(incDoc, bson.E{Key: "referrers." + safeReferrer, Value: 1})
		}

		setDoc := bson.D{
			{Key: "last_clicked_at", Value: timestamp},
			{Key: "target_url", Value: targetURL},
		}

		setOnInsertDoc := bson.D{
			{Key: "created_at", Value: timestamp},
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
