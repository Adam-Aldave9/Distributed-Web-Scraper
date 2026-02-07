package main

import (
	"os"
	Models "worker/Models"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var ScrapingDB *gorm.DB
var JobDB *gorm.DB
var RedisClient *redis.Client

func ConnectScrapingDB() {
	db, err := gorm.Open(postgres.Open(os.Getenv("SCRAPING_DATA_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to database")
	}
	db.AutoMigrate(&Models.Job{}, &Models.ScrapedItem{})
	ScrapingDB = db
}

func ConnectJobDB() {
	db, err := gorm.Open(postgres.Open(os.Getenv("JOB_INFO_DATABASE_URL")), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to database")
	}
	db.AutoMigrate(&Models.Job{})
	JobDB = db
}

func ConnectRedis() {
	redisURL := os.Getenv("REDIS_URL")
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		panic("Failed to parse Redis URL: " + err.Error())
	}

	RedisClient = redis.NewClient(options)
}
