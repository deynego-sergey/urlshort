package pipeline

import (
	"fmt"
	"net"

	"github.com/oschwald/geoip2-golang"
)

type GeoIPProvider struct {
	db *geoip2.Reader
}

func NewGeoIPProvider(dbPath string) (*GeoIPProvider, error) {
	if dbPath == "" {
		return &GeoIPProvider{}, nil
	}
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open geoip db at %s: %w", dbPath, err)
	}
	return &GeoIPProvider{db: db}, nil
}

func (g *GeoIPProvider) Close() error {
	if g.db != nil {
		return g.db.Close()
	}
	return nil
}

func (g *GeoIPProvider) GetCountryCode(ipStr string) string {
	if g == nil || g.db == nil {
		return "unknown"
	}
	if ipStr == "" || ipStr == "127.0.0.1" || ipStr == "::1" {
		return "internal"
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "unknown"
	}

	record, err := g.db.City(ip)
	if err != nil || record.Country.IsoCode == "" {
		return "unknown"
	}

	return record.Country.IsoCode
}
