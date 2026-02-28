package Models

import "time"

type Worker struct {
	Id            int       `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name"`
	Status        string    `json:"status" gorm:"index"`
	HostName      string    `json:"host_name"`
	LastHeartbeat time.Time `json:"last_heartbeat" gorm:"index"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
