package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	Services "worker/Services"
)

func main() {
	ConnectScrapingDB()
	ConnectJobDB()
	ConnectRedis()

	if err := Services.InitSupervisorClient(); err != nil {
		log.Printf("Warning: Failed to connect to supervisor: %v", err)
		log.Println("Worker will continue running but won't register with supervisor")
	}
	defer Services.CloseSupervisorClient()

	// Register with supervisor, retrying until successful or context cancelled
	go func() {
		sc := Services.GetSupervisorClient()
		if sc == nil {
			return
		}
		maxRetries := 10
		for i := 0; i < maxRetries; i++ {
			if err := Services.RegisterWorker(context.Background(), ""); err != nil {
				log.Printf("Registration attempt %d/%d failed: %v", i+1, maxRetries, err)
				time.Sleep(time.Duration(i+1) * 2 * time.Second)
				continue
			}
			return
		}
		log.Println("Warning: Failed to register with supervisor after all retries")
	}()

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

	go Services.StartHeartbeat(ctx)

	go func() {
		<-c
		log.Println("Received shutdown signal, stopping...")
		cancel()
	}()

	log.Println("Starting worker node...")

	scraperService.StartListening(ctx)

	log.Println("Worker node stopped")
}
