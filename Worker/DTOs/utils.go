package DTOs

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type ScraperService struct {
	RedisClient *redis.Client
	ScrapingDB  *gorm.DB
	JobDB       *gorm.DB
}

type JobPayload struct {
	JobID         int    `json:"job_id"`
	Name          string `json:"name"`
	URLSeedSearch string `json:"url_seed_search"`
	Status        string `json:"status"`
}
