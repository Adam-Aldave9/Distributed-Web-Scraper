package Services

import (
	Models "supervisor/Models"
	"time"

	"github.com/robfig/cron/v3"
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
	// Calculate next run time based on cron expression

	job.Status = "scheduled"
	job.IsActive = true
	job.NextRun = ""

	result := Models.JobDB.Create(&job)
	if result.Error != nil {
		return Models.Job{}, result.Error
	}

	// Schedule the job in the global cron scheduler
	scheduleJob(job)

	return job, nil
}

func UpdateJob(job Models.Job) (Models.Job, error) {
	// update next run if necessary
	if job.Cron != "" {
		job.NextRun = ""
	}
	result := Models.JobDB.Save(&job)
	if result.Error != nil {
		return Models.Job{}, result.Error
	}
	if job.Cron != "" {
		var entryID cron.EntryID
		Models.JobDB.Raw("SELECT entry_id FROM job_cron_entries WHERE job_id = ?", job.Id).Scan(&entryID)
		Models.CronScheduler.Remove(entryID)
		scheduleJob(job)
	}
	return job, nil
}

func DeleteJob(id int) (string, error) {
	var entryID cron.EntryID
	Models.JobDB.Raw("SELECT entry_id FROM job_cron_entries WHERE job_id = ?", id).Scan(&entryID)
	Models.CronScheduler.Remove(entryID)
	var job Models.Job
	result := Models.JobDB.Delete(&job, id)
	if result.Error != nil {
		return "", result.Error
	}
	return "Job deleted Successfully", nil
}

// LoadAndScheduleAllJobs loads all active jobs from database and schedules them
// TODO3: update and replace later to poll for jobs that run within next 10 minutes
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

//----------------------Helpers--------------------------------

// Helper function to schedule a job in the global cron scheduler
func scheduleJob(job Models.Job) {
	entryID, err := Models.CronScheduler.AddFunc(job.Cron, func() {
		executeJob(job)
	})
	if err != nil {
		// rollback
		Models.JobDB.Delete(&job)
		Models.JobDB.Raw("DELETE FROM job_cron_entries WHERE job_id = ?", job.Id)
		return
	}

	var cronJobExists bool
	nextRun := Models.CronScheduler.Entry(entryID).Next.Format(time.RFC3339)
	Models.JobDB.Raw("SELECT EXISTS (SELECT 1 FROM job_cron_entries WHERE job_id = ?)", job.Id).Scan(&cronJobExists)
	if cronJobExists {
		Models.JobDB.Raw("UPDATE job_cron_entries SET entry_id = ? WHERE job_id = ?", entryID, job.Id)
		Models.JobDB.Raw("UPDATE jobs SET next_run = ? WHERE id = ?", nextRun, job.Id)
	} else {
		Models.JobDB.Raw("UPDATE jobs SET next_run = ? where id = ?", nextRun, job.Id)
		jobCronEntry := Models.JobCronEntry{
			JobId:   job.Id,
			EntryId: entryID,
		}
		Models.JobDB.Create(&jobCronEntry)
	}
}

// Function to execute a job (placeholder for now)
func executeJob(job Models.Job) {
	// TODO4: Implement job execution logic
	// This could involve:
	// 1. Updating job status to "running"
	// 2. Sending job to workers via redis
}
