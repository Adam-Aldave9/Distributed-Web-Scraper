package Controllers

import (
	"net/http"
	"strconv"
	DTOs "supervisor/DTOs"
	Models "supervisor/Models"
	Services "supervisor/Services"

	"github.com/gin-gonic/gin"
)

func GetJobInfo(c *gin.Context) {
	Jobs, err := Services.GetAllJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, Jobs)
}

func GetJobById(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	Job, err := Services.GetJobById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, Job)
}

func CreateJob(c *gin.Context) {
	var request DTOs.CreateJobRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	job := Models.Job{
		Name:          request.Name,
		URLSeedSearch: request.URLSeedSearch,
		Cron:          request.Cron,
	}
	job, err := Services.CreateJob(job)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

func UpdateJob(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	existingJob, err := Services.GetJobById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var updateRequest DTOs.UpdateJobRequest
	if err := c.ShouldBindJSON(&updateRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if updateRequest.Name != "" {
		existingJob.Name = updateRequest.Name
	}
	if updateRequest.URLSeedSearch != "" {
		existingJob.URLSeedSearch = updateRequest.URLSeedSearch
	}
	if updateRequest.Cron != "" {
		existingJob.Cron = updateRequest.Cron
	}
	if updateRequest.IsActive == (true || false) {
		existingJob.IsActive = updateRequest.IsActive
	}

	updatedJob, err := Services.UpdateJob(existingJob)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updatedJob)
}

func DeleteJob(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}
	result, err := Services.DeleteJob(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": result})
}
