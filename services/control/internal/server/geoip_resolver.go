package server

import (
	"errors"
	"net"
	"strings"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// GeoLocation 表示 GeoIP 解析得到的地理信息。
type GeoLocation struct {
	Name        string   `json:"name,omitempty"`
	Country     string   `json:"country,omitempty"`
	CountryCode string   `json:"country_code,omitempty"`
	Region      string   `json:"region,omitempty"`
	City        string   `json:"city,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lon         *float64 `json:"lon,omitempty"`
	Timezone    string   `json:"timezone,omitempty"`
	IsPrivate   bool     `json:"is_private,omitempty"`
}

// geoIPResolver 使用 MaxMind 的 mmdb 文件解析 IP 地理位置。
type geoIPResolver struct {
	path string
	mu   sync.Mutex
	db   *geoip2.Reader
}

func newGeoIPResolver(path string) *geoIPResolver {
	return &geoIPResolver{path: strings.TrimSpace(path)}
}

func (r *geoIPResolver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		_ = r.db.Close()
		r.db = nil
	}
}

func (r *geoIPResolver) Lookup(ip string) *GeoLocation {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified() {
		return &GeoLocation{Name: "内网", IsPrivate: true}
	}
	db, err := r.open()
	if err != nil {
		return nil
	}
	rec, err := db.City(parsed)
	if err != nil {
		return nil
	}

	country := pickGeoName(rec.Country.Names)
	region := ""
	if len(rec.Subdivisions) > 0 {
		region = pickGeoName(rec.Subdivisions[0].Names)
	}
	city := pickGeoName(rec.City.Names)
	name := strings.TrimSpace(strings.Join(filterGeoParts(country, region, city), " / "))

	lat := rec.Location.Latitude
	lon := rec.Location.Longitude
	var latPtr *float64
	var lonPtr *float64
	if lat != 0 || lon != 0 {
		latPtr = &lat
		lonPtr = &lon
	}

	return &GeoLocation{
		Name:        name,
		Country:     country,
		CountryCode: strings.TrimSpace(rec.Country.IsoCode),
		Region:      region,
		City:        city,
		Lat:         latPtr,
		Lon:         lonPtr,
		Timezone:    strings.TrimSpace(rec.Location.TimeZone),
	}
}

func (r *geoIPResolver) open() (*geoip2.Reader, error) {
	if r == nil || strings.TrimSpace(r.path) == "" {
		return nil, errors.New("geoip path empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		return r.db, nil
	}
	db, err := geoip2.Open(r.path)
	if err != nil {
		return nil, err
	}
	r.db = db
	return db, nil
}

func pickGeoName(names map[string]string) string {
	if names == nil {
		return ""
	}
	if v := strings.TrimSpace(names["zh-CN"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(names["en"]); v != "" {
		return v
	}
	for _, v := range names {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func filterGeoParts(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
