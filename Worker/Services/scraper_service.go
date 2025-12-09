package Services

import (
	"context"
	"encoding/json"
	"log"
	"time"
	DTOs "worker/DTOs"
	Models "worker/Models"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type ScraperServiceWrapper struct { // wrapper for dto to avoid circular dependency
	*DTOs.ScraperService
}

// NewScraperService creates a new ScraperServiceWrapper
func NewScraperService(redisClient *redis.Client, scrapingDB *gorm.DB, jobDB *gorm.DB) *ScraperServiceWrapper {
	return &ScraperServiceWrapper{
		ScraperService: &DTOs.ScraperService{
			RedisClient: redisClient,
			ScrapingDB:  scrapingDB,
			JobDB:       jobDB,
		},
	}
}

// Start the scraper service. Listen for jobs on the Redis queue and scrape them.
// nt:: check out the signature. Method is defined on the wrapper.
func (s *ScraperServiceWrapper) StartListening(ctx context.Context) {
	queueName := "scraping_jobs"
	log.Printf("Starting to listen for jobs on queue: %s", queueName)

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping scraper service...")
			return
		default:
			// Block for up to 5 seconds waiting for a job
			// result is arr of 2 elements. 0 is the queue name, 1 is the data
			result, err := s.RedisClient.BRPop(ctx, 5*time.Second, queueName).Result()
			if err != nil {
				if err == redis.Nil {
					// No message available, continue polling
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

			jobData := result[1] // load job data payload as json string
			s.processJob(ctx, jobData)
		}
	}
}

// process the scraping job from the redis queue
func (s *ScraperServiceWrapper) processJob(ctx context.Context, jobData string) {
	var payload DTOs.JobPayload
	// unmarshal the job data payload as json string and set it to the payload variable
	if err := json.Unmarshal([]byte(jobData), &payload); err != nil {
		log.Printf("Error unmarshalling job payload: %v", err)
		return
	}

	log.Printf("Processing job: ID=%d, Name=%s, URL=%s", payload.JobID, payload.Name, payload.URLSeedSearch)

	// Update job status to running
	//s.updateJobStatus(payload.JobID, "running")

	// Perform the actual scraping work
	if err := s.scrapeURL(payload); err != nil {
		log.Printf("Error scraping URL for job %d: %v", payload.JobID, err)

		// Send failure status to supervisor
		if err := SendCompletionStatus(ctx, payload, "failed", "scraping failed"); err != nil {
			log.Printf("Failed to send failure status to supervisor: %v", err)
		}
		return
	}

	// Send success status to supervisor
	if err := SendCompletionStatus(ctx, payload, "completed", "success"); err != nil {
		log.Printf("Failed to send completion status to supervisor: %v", err)
	}

	log.Printf("Successfully completed job %d", payload.JobID)
}

// scrape the url for the job
func (s *ScraperServiceWrapper) scrapeURL(payload DTOs.JobPayload) error {
	// TODO: Implement actual scraping logic here
	return nil
}

// -------------------------------------
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
