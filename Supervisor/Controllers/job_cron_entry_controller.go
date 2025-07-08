package Controllers

import (
	"net/http"
	Models "supervisor/Models"
	Services "supervisor/Services"

	"github.com/gin-gonic/gin"
)

func GetJobCronEntries(c *gin.Context) {
	jobCronEntries, err := Services.GetAllJobCronEntries()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobCronEntries)
}

func DeleteJobCronEntry(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job cron entry ID"})
		return
	}

	result, err := Services.DeleteJobCronEntry(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": result})
}

func CreateJobCronEntry(c *gin.Context) {
	var jobCronEntry Models.JobCronEntry
	if err := c.ShouldBindJSON(&jobCronEntry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	createdJobCronEntry, err := Services.CreateJobCronEntry(jobCronEntry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, createdJobCronEntry)
}
