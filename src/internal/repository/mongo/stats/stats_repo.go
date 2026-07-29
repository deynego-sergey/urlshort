package stats

import (
	"context"
	"fmt"
	"strconv"
	"urlshort/pkg/database/mongoatlas"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStatsRepository struct {
	coll *mongo.Collection
}

func NewMongoStatsRepository(client *mongoatlas.Client, collectionName string) *MongoStatsRepository {
	return &MongoStatsRepository{
		coll: client.Database().Collection(collectionName),
	}
}

func (r *MongoStatsRepository) GetBySourceURL(ctx context.Context, sourceURL string) (*RedirectStat, error) {
	filter := bson.M{"source_url": sourceURL}

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

	filter := bson.M{"source_url": bson.M{"$in": sourceURLs}}

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
		filter := bson.M{"source_url": update.SourceURL}

		// Расчет индексов массивов времени
		idx15min := (update.Timestamp.Hour()*60 + update.Timestamp.Minute()) / 15
		idxDayOfWeek := (int(update.Timestamp.Weekday()) + 6) % 7 // 0 = Пн, ..., 6 = Вс

		incDoc := bson.M{
			"total_clicks": 1,
			"clicks_by_15min." + strconv.Itoa(idx15min):           1,
			"clicks_by_day_of_week." + strconv.Itoa(idxDayOfWeek): 1,
		}

		if update.IsBot {
			incDoc["bot_clicks"] = 1
			if update.Device != "" {
				incDoc["devices."+update.Device] = 1
			}
			if update.Browser != "" {
				incDoc["browsers."+update.Browser] = 1
			}
		} else if update.IsUnknown {
			incDoc["unknown_clicks"] = 1
			if update.Device != "" {
				incDoc["devices."+update.Device] = 1
			}
			if update.Browser != "" {
				incDoc["browsers."+update.Browser] = 1
			}
		} else {
			if update.Device != "" {
				incDoc["devices."+update.Device] = 1
			}
			if update.Browser != "" {
				incDoc["browsers."+update.Browser] = 1
			}
			if update.IsUnique {
				incDoc["unique_clicks"] = 1
			}
		}

		if update.Referrer != "" {
			incDoc["referrers."+update.Referrer] = 1
		}
		if update.Country != "" {
			incDoc["countries."+update.Country] = 1
		}
		if update.Provider != "" {
			incDoc["providers."+update.Provider] = 1
		}

		updateDoc := bson.M{
			"$inc": incDoc,
			"$set": bson.M{
				"last_clicked_at": update.Timestamp,
				"target_url":      update.TargetURL,
			},
			"$setOnInsert": bson.M{
				"created_at": update.Timestamp,
			},
		}

		if update.IsUnique && update.VisitorHash != "" {
			updateDoc["$addToSet"] = bson.M{
				"visitor_hashes": update.VisitorHash,
			}
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
