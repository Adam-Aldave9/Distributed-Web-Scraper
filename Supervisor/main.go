package main

import (
	Models "supervisor/Models"
	Services "supervisor/Services"
	Routes "supervisor/routes"

	"github.com/gin-gonic/gin"
)

func main() {
	// err := godotenv.Load()
	// if err != nil {
	// 	fmt.Printf("Error loading .env file: %v\n", err)
	// }

	router := gin.Default()
	Models.ConnectWorkerDatabase()
	Models.ConnectJobDatabase()
	Models.ConnectRedis()
	Models.InitializeCronScheduler()

	Services.LoadAndScheduleAllJobs()

	Routes.SetupRoutes(router)

	router.Run(":8080")
}
