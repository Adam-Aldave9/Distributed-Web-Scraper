package main

import (
	Models "supervisor/Models"
	Services "supervisor/Services"
	Routes "supervisor/routes"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()
	Models.ConnectWorkerDatabase()
	Models.ConnectJobDatabase()
	Models.InitializeCronScheduler()

	// Load and schedule existing jobs
	// TODO: change for polling for jobs that run within next 10 mihutes
	// poll every 5 seconds
	Services.LoadAndScheduleAllJobs()

	// Setup all routes
	Routes.SetupRoutes(router)

	router.Run(":8080")
}
