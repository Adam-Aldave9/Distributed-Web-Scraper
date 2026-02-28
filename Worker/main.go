package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	Services "worker/Services"
)

func main() {
	ConnectScrapingDB()
	ConnectJobDB()
	ConnectRedis()

	if err := Services.InitSupervisorClient(); err != nil {
		log.Printf("Warning: Failed to connect to supervisor: %v", err)
		log.Println("Worker will continue running but won't send completion status")
	}
	defer Services.CloseSupervisorClient()

	if sc := Services.GetSupervisorClient(); sc != nil {
		ctx := context.Background()
		if err := Services.RegisterWorker(ctx, ""); err != nil {
			log.Printf("Warning: Failed to register with supervisor: %v", err)
		}
	}

	scraperService := Services.NewScraperService(RedisClient, ScrapingDB, JobDB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := Services.StartGRPCServer(ctx, c); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	go func() {
		<-c
		log.Println("Received shutdown signal, stopping...")
		cancel()
	}()

	log.Println("Starting worker node...")

	scraperService.StartListening(ctx)

	log.Println("Worker node stopped")
}
