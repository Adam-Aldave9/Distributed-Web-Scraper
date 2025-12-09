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

	// Initialize supervisor gRPC client
	if err := Services.InitSupervisorClient(); err != nil {
		log.Printf("Warning: Failed to connect to supervisor: %v", err)
		log.Println("Worker will continue running but won't send completion status")
	}
	defer Services.CloseSupervisorClient()

	// Register this worker with the supervisor
	if sc := Services.GetSupervisorClient(); sc != nil {
		ctx := context.Background()
		if err := Services.RegisterWorker(ctx, ""); err != nil {
			log.Printf("Warning: Failed to register with supervisor: %v", err)
		}
	}

	// Create scraper service
	scraperService := Services.NewScraperService(RedisClient, ScrapingDB, JobDB)

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	c := make(chan os.Signal, 1) // create a channel to listen for signals
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start gRPC server in a goroutine
	go func() {
		if err := Services.StartGRPCServer(c); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	go func() {
		<-c // Go routine blocks here until OS sends above specified signal
		log.Println("Received shutdown signal, stopping...")
		cancel()
	}()

	log.Println("Starting worker node...")

	// Start listening for jobs
	scraperService.StartListening(ctx)

	log.Println("Worker node stopped")
}
