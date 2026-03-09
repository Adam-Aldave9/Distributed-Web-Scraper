package main

import (
	"context"
	"log"
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

	// Start gRPC server in a separate goroutine
	go func() {
		if err := Services.StartGRPCServer(); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Start worker health check (dead worker detection + job requeue)
	go Services.StartWorkerHealthCheck(context.Background())

	Routes.SetupRoutes(router)

	router.Run(":8080")
}
