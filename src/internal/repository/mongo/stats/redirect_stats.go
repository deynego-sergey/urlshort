package stats

import (
	"time"
)

type RedirectStat struct {
	ID            string    `bson:"_id,omitempty" json:"id"`
	SourceURL     string    `bson:"source_url" json:"source_url"`
	TargetURL     string    `bson:"target_url" json:"target_url"`
	TotalClicks   int64     `bson:"total_clicks" json:"total_clicks"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	LastClickedAt time.Time `bson:"last_clicked_at" json:"last_clicked_at"`

	// Временные интервалы
	// 96 ячеек по 15 минут (24ч * 4 = 96 ячеек, 00:00..23:45)
	ClicksBy15Min [96]int64 `bson:"clicks_by_15min" json:"clicks_by_15min"`

	// 7 ячеек для дней недели (0 = Пн, 1 = Вт, ..., 6 = Вс)
	ClicksByDayOfWeek [7]int64 `bson:"clicks_by_day_of_week" json:"clicks_by_day_of_week"`

	// Помесячная/подневная статистика за 4-5 месяцев (150 дней)
	ClicksByDays [150]int64 `bson:"clicks_by_days" json:"clicks_by_days"`

	// Источники переходов (Referrers) -> Счётчик
	// Ключ: URL / домен источника (например, "t.me", "google.com", "direct")
	Referrers map[string]int64 `bson:"referrers" json:"referrers"`

	// Устройства (Device types)
	// Ключи: "desktop", "mobile", "tablet", "bot"
	Devices map[string]int64 `bson:"devices" json:"devices"`

	// Геолокация (по ISO-коду страны или города)
	// Ключ: "US", "DE", "RU" и т.д.
	Countries map[string]int64 `bson:"countries" json:"countries"`

	// Интернет-провайдеры (ISP / Autonomous System)
	// Ключ: Название провайдера (например, "Cloudflare", "Comcast", "AS12345")
	Providers map[string]int64 `bson:"providers" json:"providers"`
}
