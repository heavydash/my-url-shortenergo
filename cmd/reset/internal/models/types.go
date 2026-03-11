package models

import "time"

// generate:reset
type ShortLink struct {
	ID        uint64
	Short     string
	Original  string
	Clicks    int64
	CreatedAt time.Time
	Tags      []string
	Meta      map[string]string
}
