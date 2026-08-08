// src/internal/services/pipelines/stats.go
package pipeline

import (
	"net/url"
	"strings"
	"urlshort/internal/repository/mongo/stats"
	"urlshort/pkg/httplog"
)

type EnrichedUpdate struct {
	httplog.RequestPayload
	Country    string
	DeviceType string
	UserAgent  string
}

type Pipeline struct {
	geoIP *GeoIPProvider
}

func NewPipeline(geoIP *GeoIPProvider) *Pipeline {
	return &Pipeline{
		geoIP: geoIP,
	}
}

// Process принимает сырые батчи от Collector, валидирует, обогащает и агрегирует их
func (p *Pipeline) Process(updates []stats.StatUpdate) map[string]*stats.AggregatedUpdate {
	if len(updates) == 0 {
		return nil
	}

	valid := p.filterValid(updates)
	if len(valid) == 0 {
		return nil
	}

	enriched := p.enrich(valid)
	return p.aggregate(enriched)
}

func (p *Pipeline) filterValid(updates []stats.StatUpdate) []stats.StatUpdate {
	valid := make([]stats.StatUpdate, 0, len(updates))
	for _, u := range updates {
		urlPath := u.URLPath
		if urlPath == "" {
			urlPath = u.RequestURI
		}
		if urlPath == "" {
			continue
		}
		u.URLPath = urlPath
		valid = append(valid, u)
	}
	return valid
}

func (p *Pipeline) enrich(updates []stats.StatUpdate) []EnrichedUpdate {
	enriched := make([]EnrichedUpdate, 0, len(updates))

	for _, u := range updates {
		item := EnrichedUpdate{
			RequestPayload: u.RequestPayload,
			Country:        p.extractCountryByIP(u.ClientIP),
			DeviceType:     p.extractDeviceType(u.Header),
			UserAgent:      p.extractUserAgent(u.Header),
		}
		enriched = append(enriched, item)
	}

	return enriched
}

func (p *Pipeline) aggregate(updates []EnrichedUpdate) map[string]*stats.AggregatedUpdate {
	aggregatedMap := make(map[string]*stats.AggregatedUpdate)

	for _, update := range updates {
		sourceURL := update.URLPath

		agg, exists := aggregatedMap[sourceURL]
		if !exists {
			agg = &stats.AggregatedUpdate{
				SourceURL:         sourceURL,
				TargetURL:         update.TargetURL,
				LastClickedAt:     update.Timestamp,
				EarliestTimestamp: update.Timestamp,
				DailyStats:        make(map[string]int64),
				ClicksBy15min:     make(map[int]int64),
				ClicksByDayOfWeek: make(map[int]int64),
				Referrers:         make(map[string]int64),
				Countries:         make(map[string]int64),
				Devices:           make(map[string]int64),
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

		dayKey := update.Timestamp.Format("2006-01-02")
		agg.DailyStats[dayKey]++

		idx15min := (update.Timestamp.Hour()*60 + update.Timestamp.Minute()) / 15
		idxDayOfWeek := (int(update.Timestamp.Weekday()) + 6) % 7
		agg.ClicksBy15min[idx15min]++
		agg.ClicksByDayOfWeek[idxDayOfWeek]++

		ref := update.Referer()
		if ref == "" {
			ref = update.Referrer
		}
		if sanitized := p.sanitizeReferrer(ref); sanitized != "" {
			agg.Referrers[sanitized]++
		}

		if update.Country != "" {
			agg.Countries[update.Country]++
		}

		if update.DeviceType != "" {
			agg.Devices[update.DeviceType]++
		}
	}

	return aggregatedMap
}

func (p *Pipeline) sanitizeReferrer(rawRef string) string {
	if rawRef == "" {
		return "direct"
	}
	parsed, err := url.Parse(rawRef)
	if err == nil && parsed.Host != "" {
		rawRef = parsed.Host + "/" + parsed.Path
	}
	rawRef = strings.ReplaceAll(rawRef, ".", "_")
	rawRef = strings.ReplaceAll(rawRef, "$", "_")
	return rawRef
}

func (p *Pipeline) extractUserAgent(headers map[string][]string) string {
	if headers == nil {
		return ""
	}
	if ua, ok := headers["User-Agent"]; ok && len(ua) > 0 {
		return ua[0]
	}
	if ua, ok := headers["user-agent"]; ok && len(ua) > 0 {
		return ua[0]
	}
	return ""
}

func (p *Pipeline) extractDeviceType(headers map[string][]string) string {
	ua := strings.ToLower(p.extractUserAgent(headers))
	if ua == "" {
		return "unknown"
	}

	if strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") {
		return "tablet"
	}
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "iphone") || strings.Contains(ua, "android") {
		return "mobile"
	}
	if strings.Contains(ua, "bot") || strings.Contains(ua, "crawler") || strings.Contains(ua, "spider") {
		return "bot"
	}

	return "desktop"
}

func (p *Pipeline) extractCountryByIP(ip string) string {
	if p.geoIP == nil {
		return "unknown"
	}
	return p.geoIP.GetCountryCode(ip)
}
