package Models

import "time"

type WorkerHealth struct {
	Id                   int       `json:"id" db:"id,primarykey"`
	WorkerID             int       `json:"worker_id" db:"worker_id,index,notnull,foreignkey:worker(id)"`
	Status               string    `json:"status" db:"status,index"`
	CPUUsage             float64   `json:"cpu_usage"`
	MemoryUsage          float64   `json:"memory_usage"`
	LastStarted          time.Time `json:"last_started" db:"last_started"`
	LastErrorMessage     string    `json:"last_error_message" db:"last_error_message,index"`
	LastErrorMessageTime time.Time `json:"last_error_message_time" db:"last_error_message_time,index"` //move into new table? bad nf?
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
