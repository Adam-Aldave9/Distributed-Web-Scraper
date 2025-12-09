package Services

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	Models "supervisor/Models"
	"time"

	pb "supervisor/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// WorkerClientConfig holds configuration for connecting to workers
type WorkerClientConfig struct {
	Port           string
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
}

// DefaultWorkerClientConfig returns default configuration for worker clients
func DefaultWorkerClientConfig() WorkerClientConfig {
	port := os.Getenv("WORKER_GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	return WorkerClientConfig{
		Port:           port,
		ConnectTimeout: 10 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
}

// ShutdownWorker sends a graceful shutdown request to a worker
func ShutdownWorker(workerId string) error {
	return ShutdownWorkerWithConfig(workerId, DefaultWorkerClientConfig())
}

// ShutdownWorkerWithConfig sends a graceful shutdown request with custom config
func ShutdownWorkerWithConfig(workerId string, config WorkerClientConfig) error {
	// Convert workerId to int
	id, err := strconv.Atoi(workerId)
	if err != nil {
		return fmt.Errorf("invalid worker ID format: %s", workerId)
	}

	// Get worker details from database
	worker, err := GetWorkerById(id)
	if err != nil {
		return fmt.Errorf("failed to get worker with ID %d: %w", id, err)
	}

	// Connect to worker's gRPC server
	address := fmt.Sprintf("%s:%s", worker.HostName, config.Port)
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to worker at %s: %w", address, err)
	}
	defer conn.Close()

	// Create worker service client
	client := pb.NewWorkerServiceClient(conn)

	// Create context with timeout for the request
	reqCtx, reqCancel := context.WithTimeout(context.Background(), config.RequestTimeout)
	defer reqCancel()

	// Call shutdown method
	response, err := client.Shutdown(reqCtx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to shutdown worker %s: %w", workerId, err)
	}

	log.Printf("Worker %s shutdown successfully: %s at %s", workerId, response.Message, response.Timestamp)

	// Update worker status in database
	if Models.WorkerDB != nil {
		Models.WorkerDB.Model(&Models.Worker{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"status":     "shutdown",
				"updated_at": time.Now(),
			})
	}

	return nil
}

// RestartWorker sends a restart request to a worker
func RestartWorker(workerId string) error {
	return RestartWorkerWithConfig(workerId, DefaultWorkerClientConfig())
}

// RestartWorkerWithConfig sends a restart request to a worker with custom config
func RestartWorkerWithConfig(workerId string, config WorkerClientConfig) error {
	// Convert workerId to int
	id, err := strconv.Atoi(workerId)
	if err != nil {
		return fmt.Errorf("invalid worker ID format: %s", workerId)
	}

	// Get worker details from database
	worker, err := GetWorkerById(id)
	if err != nil {
		return fmt.Errorf("failed to get worker with ID %d: %w", id, err)
	}

	// Connect to worker's gRPC server
	address := fmt.Sprintf("%s:%s", worker.HostName, config.Port)
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to worker at %s: %w", address, err)
	}
	defer conn.Close()

	// Create worker service client
	client := pb.NewWorkerServiceClient(conn)

	// Create context with timeout for the request
	reqCtx, reqCancel := context.WithTimeout(context.Background(), config.RequestTimeout)
	defer reqCancel()

	// Call restart method
	response, err := client.Restart(reqCtx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to restart worker %s: %w", workerId, err)
	}

	log.Printf("Worker %s restart initiated: %s at %s", workerId, response.Message, response.Timestamp)

	// Update worker status in database
	if Models.WorkerDB != nil {
		Models.WorkerDB.Model(&Models.Worker{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"status":     "restarting",
				"updated_at": time.Now(),
			})
	}

	return nil
}
