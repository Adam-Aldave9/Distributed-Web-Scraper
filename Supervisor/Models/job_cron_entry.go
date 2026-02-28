package Models

import (
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type JobCronEntry struct {
	Id        uuid.UUID    `json:"id" gorm:"primaryKey;type:uuid"`
	JobId     int          `json:"job_id" gorm:"not null"`
	EntryId   cron.EntryID `json:"entry_id" gorm:"not null"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}
