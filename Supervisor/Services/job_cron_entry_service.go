package Services

import (
	Models "supervisor/Models"

	"github.com/google/uuid"
)

func GetAllJobCronEntries() ([]Models.JobCronEntry, error) {
	var jobCronEntries []Models.JobCronEntry
	result := Models.JobDB.Find(&jobCronEntries)
	if result.Error != nil {
		return nil, result.Error
	}
	return jobCronEntries, nil
}

func CreateJobCronEntry(jobCronEntry Models.JobCronEntry) (Models.JobCronEntry, error) {
	result := Models.JobDB.Create(&jobCronEntry)
	if result.Error != nil {
		return Models.JobCronEntry{}, result.Error
	}
	return jobCronEntry, nil
}

func DeleteJobCronEntry(id uuid.UUID) (string, error) {
	var jobCronEntry Models.JobCronEntry
	result := Models.JobDB.Where("id = ?", id).Delete(&jobCronEntry)
	if result.Error != nil {
		return "", result.Error
	}
	return "Job cron entry deleted successfully", nil
}
