package DTOs

type CreateJobRequest struct {
	Name          string `json:"name" binding:"required"`
	URLSeedSearch string `json:"url_seed_search" binding:"required"`
	Cron          string `json:"cron" binding:"required"`
}

type UpdateJobRequest struct {
	Name          *string `json:"name"`
	URLSeedSearch *string `json:"url_seed_search"`
	Cron          *string `json:"cron"`
	IsActive      *bool   `json:"is_active"`
}

type CreateJobCronEntryRequest struct {
	JobId   string `json:"job_id" binding:"required"`
	EntryId string `json:"entry_id" binding:"required"`
}

type UpdateJobCronEntryRequest struct {
	JobId   string `json:"job_id"`
	EntryId string `json:"entry_id"`
}

type CreateWorkerRequest struct {
	Name     string `json:"name" binding:"required"`
	HostName string `json:"host_name" binding:"required"`
	Status   string `json:"status" binding:"required"`
}

type UpdateWorkerRequest struct {
	Name     string `json:"name"`
	HostName string `json:"host_name"`
	Status   string `json:"status"`
}
