package Models

import (
	"time"
)

type ScrapedItem struct {
	ID        int       `json:"id" gorm:"primaryKey"`
	JobID     int       `json:"job_id" gorm:"index"`
	Title     string    `json:"title"`
	Rating    string    `json:"rating"`
	Website   string    `json:"website"`
	Price     float64   `json:"price"`
	URL       string    `json:"url"`
	ScrapedAt time.Time `json:"scraped_at" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
