package Models

import "time"

type Job struct {
	Id            int       `json:"id" db:"id,primarykey"`
	Name          string    `json:"name"`
	Status        string    `json:"status" db:"status,index"` //(pending, running, completed, scheduled	)
	URLSeedSearch string    `json:"url_seed_search"`
	LastResult    string    `json:"last_result"`
	Cron          string    `json:"cron"`
	LastRun       string    `json:"last_run"`
	NextRun       string    `json:"next_run" db:"next_run,index"`
	IsActive      bool      `json:"is_active" db:"is_active,index"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
