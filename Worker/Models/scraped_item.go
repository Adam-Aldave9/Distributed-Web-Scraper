package Models

import (
	"time"
)

type ScrapedItem struct {
	ID        int       `json:"id" db:"id,primarykey"`
	JobID     int       `json:"job_id" db:"job_id,index"`
	Title     string    `json:"title" db:"title"`
	Brand     string    `json:"brand" db:"brand"`
	Website   string    `json:"website" db:"website"`
	Price     float64   `json:"price" db:"price"`
	URL       string    `json:"url" db:"url"`
	ScrapedAt time.Time `json:"scraped_at" db:"scraped_at,index"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
