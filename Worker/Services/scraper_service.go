package Services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
	DTOs "worker/DTOs"
	Models "worker/Models"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// wrapper for dto to avoid circular dependency
type ScraperServiceWrapper struct {
	*DTOs.ScraperService
}

func NewScraperService(redisClient *redis.Client, scrapingDB *gorm.DB, jobDB *gorm.DB) *ScraperServiceWrapper {
	return &ScraperServiceWrapper{
		ScraperService: &DTOs.ScraperService{
			RedisClient: redisClient,
			ScrapingDB:  scrapingDB,
			JobDB:       jobDB,
		},
	}
}

func (s *ScraperServiceWrapper) StartListening(ctx context.Context) {
	queueName := "scraping_jobs"
	log.Printf("Starting to listen for jobs on queue: %s", queueName)

	sem := make(chan struct{}, 2) // semaphore to allow max 2 parallel scraping tasks per worker node
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping scraper service, waiting for active jobs...")
			wg.Wait()
			log.Println("All active jobs completed.")
			return
		default:
			// result[0] is queue name, result[1] is data
			result, err := s.RedisClient.BRPop(ctx, 5*time.Second, queueName).Result()
			if err != nil {
				if err == redis.Nil {
					continue
				}
				log.Printf("Error reading from Redis queue: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if len(result) < 2 {
				log.Println("Invalid message format received")
				continue
			}

			jobData := result[1]

			sem <- struct{}{} // acquire slot. blocks if limit reached
			wg.Add(1)
			go func(data string) {
				defer wg.Done()
				defer func() { <-sem }()
				s.processJob(ctx, data)
			}(jobData)
		}
	}
}

func (s *ScraperServiceWrapper) processJob(ctx context.Context, jobData string) {
	var payload DTOs.JobPayload
	if err := json.Unmarshal([]byte(jobData), &payload); err != nil {
		log.Printf("Error unmarshalling job payload: %v", err)
		return
	}

	log.Printf("Processing job: ID=%d, Name=%s, URL=%s", payload.JobID, payload.Name, payload.URLSeedSearch)

	if err := s.scrapeURL(payload); err != nil {
		log.Printf("Error scraping URL for job %d: %v", payload.JobID, err)

		if err := SendCompletionStatus(ctx, payload, "failed", "scraping failed"); err != nil {
			log.Printf("Failed to send failure status to supervisor: %v", err)
		}
		return
	}

	if err := SendCompletionStatus(ctx, payload, "completed", "success"); err != nil {
		log.Printf("Failed to send completion status to supervisor: %v", err)
	}

	log.Printf("Successfully completed job %d", payload.JobID)
}

func (s *ScraperServiceWrapper) scrapeURL(payload DTOs.JobPayload) error {
	s.updateJobStatus(payload.JobID, "running")

	config := DefaultScrapeConfig()

	totalItems, err := RunScrape(s.ScrapingDB, payload, config)
	if err != nil {
		s.updateJobResult(payload.JobID, fmt.Sprintf("error: %v (saved %d items before failure)", err, totalItems))
		return err
	}

	s.updateJobResult(payload.JobID, fmt.Sprintf("scraped %d items", totalItems))
	return nil
}

func (s *ScraperServiceWrapper) updateJobStatus(jobID int, status string) {
	result := s.JobDB.Model(&Models.Job{}).Where("id = ?", jobID).Update("status", status)
	if result.Error != nil {
		log.Printf("Error updating job status for job %d: %v", jobID, result.Error)
	}
}

func (s *ScraperServiceWrapper) updateJobResult(jobID int, lastResult string) {
	result := s.JobDB.Model(&Models.Job{}).Where("id = ?", jobID).Update("last_result", lastResult)
	if result.Error != nil {
		log.Printf("Error updating job result for job %d: %v", jobID, result.Error)
	}
}
