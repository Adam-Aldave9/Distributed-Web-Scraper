package Services

import (
	"context"
	"fmt"
	"log"
	"os"
	Models "supervisor/Models"
	"time"

	pb "supervisor/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type WorkerClientConfig struct {
	Port           string
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
}

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

func ShutdownWorker(workerId int) error {
	return ShutdownWorkerWithConfig(workerId, DefaultWorkerClientConfig())
}

func ShutdownWorkerWithConfig(workerId int, config WorkerClientConfig) error {
	worker, err := GetWorkerById(workerId)
	if err != nil {
		return fmt.Errorf("failed to get worker with ID %d: %w", workerId, err)
	}

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

	client := pb.NewWorkerServiceClient(conn)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), config.RequestTimeout)
	defer reqCancel()

	response, err := client.Shutdown(reqCtx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to shutdown worker %d: %w", workerId, err)
	}

	log.Printf("Worker %d shutdown successfully: %s at %s", workerId, response.Message, response.Timestamp)

	if Models.WorkerDB != nil {
		Models.WorkerDB.Model(&Models.Worker{}).
			Where("id = ?", workerId).
			Updates(map[string]interface{}{
				"status":     "shutdown",
				"updated_at": time.Now(),
			})
	}

	return nil
}
