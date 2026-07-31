package stats

import (
	"time"

	"urlshort/pkg/httplog"
)

// StatUpdate содержит полный дамп HTTP-запроса из сокета.
type StatUpdate struct {
	httplog.RequestPayload
}

// RedirectStat отражает агрегированный документ в MongoDB.
type RedirectStat struct {
	SourceURL         string         `bson:"source_url"`
	TargetURL         string         `bson:"target_url"`
	TotalClicks       int64          `bson:"total_clicks"`
	ClicksBy15min     [96]int64      `bson:"clicks_by_15min"`
	ClicksByDayOfWeek [7]int64       `bson:"clicks_by_day_of_week"`
	Referrers         map[string]int `bson:"referrers"`
	LastClickedAt     time.Time      `bson:"last_clicked_at"`
	CreatedAt         time.Time      `bson:"created_at"`
}
