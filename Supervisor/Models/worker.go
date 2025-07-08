package Models

import "time"

type Worker struct {
	Id            int       `json:"id" db:"id,primarykey"`
	Name          string    `json:"name"`
	Status        string    `json:"status" db:"status,index"`
	HostName      string    `json:"host_name"`
	LastHeartbeat time.Time `json:"last_heartbeat" db:"last_heartbeat,index"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
