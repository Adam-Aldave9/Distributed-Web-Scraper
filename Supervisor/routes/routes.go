package routes

import (
	Controllers "supervisor/Controllers"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {
	workers := router.Group("/workers")
	{
		workers.GET("", Controllers.GetWorkers)
		workers.GET("/:id", Controllers.GetWorkerById)
		workers.POST("", Controllers.CreateWorker)
		workers.PUT("/:id", Controllers.UpdateWorker)
		workers.DELETE("/:id", Controllers.DeleteWorker)
		workers.POST("/:id/shutdown", Controllers.ShutdownWorker)
	}

	jobs := router.Group("/jobs")
	{
		jobs.GET("", Controllers.GetJobInfo)
		jobs.GET("/:id", Controllers.GetJobById)
		jobs.POST("", Controllers.CreateJob)
		jobs.PUT("/:id", Controllers.UpdateJob)
		jobs.DELETE("/:id", Controllers.DeleteJob)
	}

	jobCronEntries := router.Group("/cron")
	{
		jobCronEntries.GET("", Controllers.GetJobCronEntries)
		jobCronEntries.POST("", Controllers.CreateJobCronEntry)
		jobCronEntries.DELETE("/:id", Controllers.DeleteJobCronEntry)
	}
}
