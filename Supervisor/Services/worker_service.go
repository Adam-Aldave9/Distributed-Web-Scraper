package Services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	Models "supervisor/Models"

	"gorm.io/gorm"
)

func GetAllWorkers() ([]Models.Worker, error) {
	var workers []Models.Worker
	result := Models.WorkerDB.Find(&workers)
	if result.Error != nil {
		return nil, result.Error
	}
	return workers, nil
}

func GetWorkerById(id int) (Models.Worker, error) {
	var worker Models.Worker
	result := Models.WorkerDB.First(&worker, id)
	if result.Error != nil {
		return Models.Worker{}, result.Error
	}
	return worker, nil
}

func CreateWorker(worker Models.Worker) (Models.Worker, error) {
	worker.Status = "online"
	worker.LastHeartbeat = time.Now()
	result := Models.WorkerDB.Create(&worker)
	if result.Error != nil {
		return Models.Worker{}, result.Error
	}
	return worker, nil
}

func UpdateWorker(worker Models.Worker) (Models.Worker, error) {
	worker.LastHeartbeat = time.Now()
	worker.Status = "online"
	result := Models.WorkerDB.Save(&worker)
	if result.Error != nil {
		return Models.Worker{}, result.Error
	}
	return worker, nil
}

func DeleteWorker(id int) (string, error) {
	var worker Models.Worker
	result := Models.WorkerDB.Delete(&worker, id)
	if result.Error != nil {
		return "", result.Error
	}
	return "Worker deleted successfully", nil
}

const (
	heartbeatTimeout    = 45 * time.Second // worker considered dead after this
	healthCheckInterval = 30 * time.Second
)

// periodically checks for dead workers and requeues their jobs.
// Blocks until ctx is cancelled.
func StartWorkerHealthCheck(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	log.Println("Worker health check started")

	for {
		select {
		case <-ctx.Done():
			log.Println("Worker health check stopped")
			return
		case <-ticker.C:
			checkAndRecover(ctx)
		}
	}
}

func checkAndRecover(ctx context.Context) {
	cutoff := time.Now().Add(-heartbeatTimeout)

	// Find workers that are active but have stale heartbeats
	var staleWorkers []Models.Worker
	result := Models.WorkerDB.Where("status = ? AND last_heartbeat < ?", "active", cutoff).Find(&staleWorkers)
	if result.Error != nil {
		log.Printf("Error querying stale workers: %v", result.Error)
		return
	}

	for _, worker := range staleWorkers {
		log.Printf("Worker %s (id=%d) missed heartbeat (last: %s), marking offline",
			worker.Name, worker.Id, worker.LastHeartbeat.Format(time.RFC3339))

		Models.WorkerDB.Model(&worker).Updates(map[string]interface{}{
			"status":     "offline",
			"updated_at": time.Now(),
		})

		requeueWorkerJobs(ctx, worker.Name)
	}
}

func requeueWorkerJobs(ctx context.Context, workerName string) {
	var stuckJobs []Models.Job
	result := Models.JobDB.Where("worker_id = ? AND status = ?", workerName, "running").Find(&stuckJobs)
	if result.Error != nil {
		log.Printf("Error querying stuck jobs for worker %s: %v", workerName, result.Error)
		return
	}

	if len(stuckJobs) == 0 {
		return
	}

	queueName := "scraping_jobs"
	requeuedCount := 0

	for _, job := range stuckJobs {
		// Marshal payload before the transaction
		jobPayload := map[string]interface{}{
			"job_id":          job.Id,
			"name":            job.Name,
			"url_seed_search": job.URLSeedSearch,
			"status":          "pending",
		}

		jobJSON, err := json.Marshal(jobPayload)
		if err != nil {
			log.Printf("Error marshalling requeued job %d: %v", job.Id, err)
			continue
		}

		// DB update + Redis push in a transaction. If Redis fails, the DB update rolls back and the job stays as "running"
		// The next health check cycle will retry it.
		err = Models.JobDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&job).Updates(map[string]interface{}{
				"status":    "pending",
				"worker_id": "",
			}).Error; err != nil {
				return fmt.Errorf("db update failed: %w", err)
			}

			if err := Models.RedisClient.LPush(ctx, queueName, jobJSON).Err(); err != nil {
				return fmt.Errorf("redis push failed: %w", err)
			}

			return nil
		})

		if err != nil {
			log.Printf("Error requeuing job %d from worker %s: %v", job.Id, workerName, err)
			continue
		}

		requeuedCount++
		log.Printf("Requeued job %d (%s) from dead worker %s", job.Id, job.Name, workerName)
	}

	if requeuedCount > 0 {
		fmt.Printf("Requeued %d/%d jobs from dead worker %s\n", requeuedCount, len(stuckJobs), workerName)
	}
}
