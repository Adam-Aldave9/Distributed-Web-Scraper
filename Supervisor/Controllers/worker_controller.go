package Controllers

import (
	"net/http"
	"strconv"
	DTOs "supervisor/DTOs"
	Models "supervisor/Models"
	Services "supervisor/Services"

	"github.com/gin-gonic/gin"
)

func GetWorkers(c *gin.Context) {
	workers, err := Services.GetAllWorkers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workers)
}

func GetWorkerById(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid worker ID"})
		return
	}
	worker, err := Services.GetWorkerById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, worker)
}

func CreateWorker(c *gin.Context) {
	var request DTOs.CreateWorkerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	worker := Models.Worker{
		Name:     request.Name,
		HostName: request.HostName,
		Status:   request.Status,
	}
	createdWorker, err := Services.CreateWorker(worker)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, createdWorker)
}

func UpdateWorker(c *gin.Context) {
	var request DTOs.UpdateWorkerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	worker := Models.Worker{
		Name:     request.Name,
		HostName: request.HostName,
		Status:   request.Status,
	}
	updatedWorker, err := Services.UpdateWorker(worker)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updatedWorker)
}

func DeleteWorker(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid worker ID"})
		return
	}
	message, err := Services.DeleteWorker(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, message)
}

func ShutdownWorker(c *gin.Context) {
	id := c.Param("id")
	if _, err := strconv.Atoi(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid worker ID"})
		return
	}
	if err := Services.ShutdownWorker(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Shutdown signal sent to worker " + id})
}
