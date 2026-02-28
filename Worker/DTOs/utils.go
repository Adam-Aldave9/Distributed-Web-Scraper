package DTOs

type JobPayload struct {
	JobID         int    `json:"job_id"`
	Name          string `json:"name"`
	URLSeedSearch string `json:"url_seed_search"`
	Status        string `json:"status"`
}
