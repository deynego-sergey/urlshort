// src/internal/repository/mongo/stats/redirect_stats.go
package stats

import (
	"time"

	"urlshort/pkg/httplog"
)

type StatUpdate struct {
	httplog.RequestPayload
}

type RedirectStat struct {
	SourceURL         string           `bson:"source_url"`
	TargetURL         string           `bson:"target_url"`
	TotalClicks       int64            `bson:"total_clicks"`
	DailyStats        map[string]int64 `bson:"daily_stats"` // "2026-08-02": 150
	ClicksBy15min     [96]int64        `bson:"clicks_by_15min"`
	ClicksByDayOfWeek [7]int64         `bson:"clicks_by_day_of_week"`
	Referrers         map[string]int64 `bson:"referrers"`
	LastClickedAt     time.Time        `bson:"last_clicked_at"`
	CreatedAt         time.Time        `bson:"created_at"`
}
