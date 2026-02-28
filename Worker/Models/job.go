package Models

import "time"

type Job struct {
	Id            int       `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name"`
	Status        string    `json:"status" gorm:"index"` //(scheduled, pending, running, completed	)
	URLSeedSearch string    `json:"url_seed_search"`
	LastResult    string    `json:"last_result"`
	Cron          string    `json:"cron"`
	LastRun       *time.Time `json:"last_run"`
	NextRun       *time.Time `json:"next_run" gorm:"index"`
	IsActive      bool      `json:"is_active" gorm:"index"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
