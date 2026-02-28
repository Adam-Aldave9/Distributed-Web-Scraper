package Services

import (
	"context"
	"encoding/json"
	"fmt"
	Models "supervisor/Models"
	"time"

	"github.com/google/uuid"
)

func GetAllJobs() ([]Models.Job, error) {
	var jobs []Models.Job
	result := Models.JobDB.Find(&jobs)
	if result.Error != nil {
		return nil, result.Error
	}
	return jobs, nil
}

func GetJobById(id int) (Models.Job, error) {
	var job Models.Job
	result := Models.JobDB.First(&job, id)
	if result.Error != nil {
		return Models.Job{}, result.Error
	}
	return job, nil
}

func CreateJob(job Models.Job) (Models.Job, error) {
	job.Status = "scheduled"
	job.IsActive = true
	job.NextRun = nil

	result := Models.JobDB.Create(&job)
	if result.Error != nil {
		return Models.Job{}, result.Error
	}

	// Schedule the job in the global cron scheduler
	if result.Error == nil && result.RowsAffected > 0 {
		scheduleJob(job)
	}

	return job, nil
}

func UpdateJob(job Models.Job) (Models.Job, error) { //nt:: check raw
	// update next run if necessary
	if job.Cron != "" {
		job.NextRun = nil
	}
	result := Models.JobDB.Save(&job)
	if result.Error != nil {
		return Models.Job{}, result.Error
	}
	if job.Cron != "" {
		var jobCronEntry Models.JobCronEntry
		result := Models.JobDB.Where("job_id = ?", job.Id).First(&jobCronEntry)

		if result.Error == nil {
			// Record exists, remove from scheduler
			Models.CronScheduler.Remove(jobCronEntry.EntryId)
		}
		scheduleJob(job)
	}
	return job, nil
}

func DeleteJob(id int) (string, error) { //nt:: check raw
	var jobCronEntry Models.JobCronEntry
	result := Models.JobDB.Where("job_id = ?", id).First(&jobCronEntry)

	if result.Error == nil {
		Models.CronScheduler.Remove(jobCronEntry.EntryId)
	}

	var job Models.Job
	deleteResult := Models.JobDB.Delete(&job, id)
	if deleteResult.Error != nil {
		return "", deleteResult.Error
	}
	return "Job deleted Successfully", nil
}

func LoadAndScheduleAllJobs() error {
	jobs, err := GetAllJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.IsActive {
			scheduleJob(job)
		}
	}

	return nil
}

// ----------------------Helpers--------------------------------
func scheduleJob(job Models.Job) {
	entryID, err := Models.CronScheduler.AddFunc(job.Cron, func() {
		executeJob(job)
	})
	if err != nil {
		// rollback
		fmt.Println("Error scheduling job: ", err)
		Models.JobDB.Delete(&job)
		Models.JobDB.Exec("DELETE FROM job_cron_entries WHERE job_id = ?", job.Id)
		return
	}

	var cronJobExists bool
	nextRun := Models.CronScheduler.Entry(entryID).Next
	Models.JobDB.Raw("SELECT EXISTS (SELECT 1 FROM job_cron_entries WHERE job_id = ?)", job.Id).Scan(&cronJobExists)
	fmt.Println("Cron job exists: ", cronJobExists)
	if cronJobExists { // update existing job cron entry id and next run time
		res := Models.JobDB.Exec("UPDATE job_cron_entries SET entry_id = ? WHERE job_id = ?", entryID, job.Id)
		if res.Error != nil {
			fmt.Println("Error updating job_cron_entries: ", res.Error)
			return
		}
		res = Models.JobDB.Exec("UPDATE jobs SET next_run = ? WHERE id = ?", nextRun, job.Id)
		if res.Error != nil {
			fmt.Println("Error updating jobs next_run: ", res.Error)
			return
		}
	} else { // create new job cron entry and update next run time
		res := Models.JobDB.Model(&job).Update("next_run", nextRun)

		if res.Error == nil && res.RowsAffected > 0 {
			fmt.Println("Successfully updated next_run for job id:", job.Id)
		} else {
			fmt.Println("Failed to update next_run for job id:", job.Id, "Error:", res.Error)
		}

		jobCronEntry := Models.JobCronEntry{
			Id:      uuid.New(),
			JobId:   job.Id,
			EntryId: entryID,
		}
		res = Models.JobDB.Create(&jobCronEntry)
		if res.Error != nil {
			fmt.Println("Error creating job_cron_entry: ", res.Error)
			return
		}
	}
}

func executeJob(job Models.Job) {
	now := time.Now()
	Models.JobDB.Exec("UPDATE jobs SET status = 'pending', last_run = ? WHERE id = ?", now, job.Id)

	ctx := context.Background()
	queueName := "scraping_jobs"

	jobPayload := map[string]interface{}{
		"job_id":          job.Id,
		"name":            job.Name,
		"url_seed_search": job.URLSeedSearch,
		"status":          "pending",
	}

	jobJSON, err := json.Marshal(jobPayload)
	if err != nil {
		Models.JobDB.Exec("UPDATE jobs SET last_result = ?, status = 'scheduled' WHERE id = ?",
			fmt.Sprintf("failed to marshal job: %v", err), job.Id)
		return
	}

	err = Models.RedisClient.LPush(ctx, queueName, jobJSON).Err()
	if err != nil {
		Models.JobDB.Exec("UPDATE jobs SET last_result = ?, status = 'scheduled' WHERE id = ?",
			fmt.Sprintf("failed to queue job: %v", err), job.Id)
		return
	}

	// Update next_run from the cron scheduler
	var jobCronEntry Models.JobCronEntry
	if res := Models.JobDB.Where("job_id = ?", job.Id).First(&jobCronEntry); res.Error == nil {
		entry := Models.CronScheduler.Entry(jobCronEntry.EntryId)
		if !entry.Next.IsZero() {
			Models.JobDB.Exec("UPDATE jobs SET next_run = ? WHERE id = ?", entry.Next, job.Id)
		}
	}
}
