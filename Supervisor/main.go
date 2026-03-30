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
	router := gin.Default()
	Models.ConnectWorkerDatabase()
	Models.ConnectJobDatabase()
	Models.ConnectRedis()
	Models.InitializeCronScheduler()

	Services.LoadAndScheduleAllJobs()

	go func() {
		if err := Services.StartGRPCServer(); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	go Services.StartWorkerHealthCheck(context.Background())

	Routes.SetupRoutes(router)

	router.Run(":8080")
}
