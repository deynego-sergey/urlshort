package stats

import (
	"context"
	"time"
)

type RedirectStat struct {
	ID            string    `bson:"_id,omitempty" json:"id"`
	SourceURL     string    `bson:"source_url" json:"source_url"`
	TargetURL     string    `bson:"target_url" json:"target_url"`
	TotalClicks   int64     `bson:"total_clicks" json:"total_clicks"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	LastClickedAt time.Time `bson:"last_clicked_at" json:"last_clicked_at"`

	UniqueClicks  int64 `bson:"unique_clicks" json:"unique_clicks"`
	BotClicks     int64 `bson:"bot_clicks" json:"bot_clicks"`
	UnknownClicks int64 `bson:"unknown_clicks" json:"unknown_clicks"`

	ClicksBy15Min     [96]int64  `bson:"clicks_by_15min" json:"clicks_by_15min"`
	ClicksByDayOfWeek [7]int64   `bson:"clicks_by_day_of_week" json:"clicks_by_day_of_week"`
	ClicksByDays      [150]int64 `bson:"clicks_by_days" json:"clicks_by_days"`

	Devices   map[string]int64 `bson:"devices" json:"devices"`
	Browsers  map[string]int64 `bson:"browsers" json:"browsers"`
	Referrers map[string]int64 `bson:"referrers" json:"referrers"`
	Countries map[string]int64 `bson:"countries" json:"countries"`
	Providers map[string]int64 `bson:"providers" json:"providers"`

	VisitorHashes []string `bson:"visitor_hashes,omitempty" json:"-"`
}

type StatUpdate struct {
	SourceURL string    `json:"source_url"`
	TargetURL string    `json:"target_url"`
	Timestamp time.Time `json:"timestamp"`

	VisitorHash string `json:"visitor_hash"`
	IsBot       bool   `json:"is_bot"`
	IsUnknown   bool   `json:"is_unknown"`
	IsUnique    bool   `json:"is_unique"`

	Device   string `json:"device"`
	Browser  string `json:"browser"`
	Referrer string `json:"referrer"`
	Country  string `json:"country"`
	Provider string `json:"provider"`
}

type IStatsRepository interface {
	GetBySourceURL(ctx context.Context, sourceURL string) (*RedirectStat, error)
	GetBySourceURLs(ctx context.Context, sourceURLs []string) ([]RedirectStat, error)
	BulkUpsert(ctx context.Context, updates []StatUpdate) error
}
