package Models

import (
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type JobCronEntry struct {
	Id        uuid.UUID    `json:"id" db:"id,primarykey,type:uuid"`
	JobId     int          `json:"job_id" db:"job_id,notnull,foreignkey:job(id)"`
	EntryId   cron.EntryID `json:"entry_id" db:"entry_id,notnull"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}
