package Models

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var WorkerDB *gorm.DB
var JobDB *gorm.DB
var CronScheduler *cron.Cron
var RedisClient *redis.Client

func ConnectWorkerDatabase() {
	db, err := gorm.Open(postgres.Open(os.Getenv("WORKER_INFO_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to database")
	}
	db.AutoMigrate(&Worker{})
	WorkerDB = db
}

func ConnectJobDatabase() {
	db, err := gorm.Open(postgres.Open(os.Getenv("JOB_INFO_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to database")
	}
	db.AutoMigrate(&Job{}, &JobCronEntry{})
	JobDB = db
}

func InitializeCronScheduler() {
	CronScheduler = cron.New(cron.WithSeconds())
	CronScheduler.Start()
}

func ConnectRedis() {
	redisURL := os.Getenv("REDIS_URL")
	// nt:: opt?
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		panic("Failed to parse Redis URL: " + err.Error())
	}

	RedisClient = redis.NewClient(options)
}
