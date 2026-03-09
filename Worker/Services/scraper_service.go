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

type ScraperService struct {
	RedisClient *redis.Client
	ScrapingDB  *gorm.DB
	JobDB       *gorm.DB
}

func NewScraperService(redisClient *redis.Client, scrapingDB *gorm.DB, jobDB *gorm.DB) *ScraperService {
	return &ScraperService{
		RedisClient: redisClient,
		ScrapingDB:  scrapingDB,
		JobDB:       jobDB,
	}
}

func (s *ScraperService) StartListening(ctx context.Context) {
	queueName := "scraping_jobs"
	log.Printf("Starting to listen for jobs on queue: %s", queueName)

	sem := make(chan struct{}, WorkerCapacity) // semaphore to allow max parallel scraping tasks per worker node
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
			ActiveJobCount.Add(1)
			wg.Add(1)
			go func(data string) {
				defer wg.Done()
				defer func() {
					<-sem
					ActiveJobCount.Add(-1)
				}()
				s.processJob(ctx, data)
			}(jobData)
		}
	}
}

func (s *ScraperService) processJob(ctx context.Context, jobData string) {
	var payload DTOs.JobPayload
	if err := json.Unmarshal([]byte(jobData), &payload); err != nil {
		log.Printf("Error unmarshalling job payload: %v", err)
		return
	}

	log.Printf("Processing job: ID=%d, Name=%s, URL=%s", payload.JobID, payload.Name, payload.URLSeedSearch)

	if err := s.scrapeURL(payload); err != nil {
		log.Printf("Error scraping URL for job %d: %v", payload.JobID, err)
		s.updateJobStatus(payload.JobID, "failed")
		return
	}

	s.updateJobStatus(payload.JobID, "completed")
	log.Printf("Successfully completed job %d", payload.JobID)
}

func (s *ScraperService) scrapeURL(payload DTOs.JobPayload) error {
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

func (s *ScraperService) updateJobStatus(jobID int, status string) {
	updates := map[string]interface{}{"status": status}
	if status == "running" {
		// Claim this job by setting worker_id
		workerID := ""
		if sc := GetSupervisorClient(); sc != nil {
			workerID = sc.GetConfig().WorkerID
		}
		updates["worker_id"] = workerID
	} else if status == "completed" || status == "failed" {
		// Release ownership when done
		updates["worker_id"] = ""
	}
	result := s.JobDB.Model(&Models.Job{}).Where("id = ?", jobID).Updates(updates)
	if result.Error != nil {
		log.Printf("Error updating job status for job %d: %v", jobID, result.Error)
	}
}

func (s *ScraperService) updateJobResult(jobID int, lastResult string) {
	result := s.JobDB.Model(&Models.Job{}).Where("id = ?", jobID).Update("last_result", lastResult)
	if result.Error != nil {
		log.Printf("Error updating job result for job %d: %v", jobID, result.Error)
	}
}
