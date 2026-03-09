package Services

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	pb "worker/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ActiveJobCount tracks the number of currently running jobs on this worker.
// Exported so scraper_service.go can increment/decrement it.
var ActiveJobCount atomic.Int32

const WorkerCapacity = 2 // max parallel jobs per worker

// SupervisorClientConfig holds configuration for connecting to the supervisor
type SupervisorClientConfig struct {
	Address        string
	WorkerID       string
	WorkerHostName string
	WorkerPort     int32
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
}

// DefaultSupervisorClientConfig returns default configuration for supervisor client
func DefaultSupervisorClientConfig() SupervisorClientConfig {
	address := os.Getenv("SUPERVISOR_GRPC_ADDRESS")
	if address == "" {
		address = "localhost:50051"
	}
	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		hostname, _ := os.Hostname()
		workerID = fmt.Sprintf("worker-%s", hostname)
	}
	workerHostName := os.Getenv("WORKER_HOSTNAME")
	if workerHostName == "" {
		workerHostName, _ = os.Hostname()
	}

	return SupervisorClientConfig{
		Address:        address,
		WorkerID:       workerID,
		WorkerHostName: workerHostName,
		WorkerPort:     50051,
		ConnectTimeout: 10 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
}

// SupervisorClient wraps the gRPC client with thread-safe operations
type SupervisorClient struct {
	mu     sync.RWMutex
	conn   *grpc.ClientConn
	client pb.SupervisorServiceClient
	config SupervisorClientConfig
}

var (
	supervisorClientInstance *SupervisorClient
	supervisorClientOnce     sync.Once
)

// GetSupervisorClient returns the singleton supervisor client instance
func GetSupervisorClient() *SupervisorClient {
	return supervisorClientInstance
}

// InitSupervisorClient initializes the supervisor client with default config
func InitSupervisorClient() error {
	return InitSupervisorClientWithConfig(DefaultSupervisorClientConfig())
}

// InitSupervisorClientWithConfig initializes the supervisor client with custom config
func InitSupervisorClientWithConfig(config SupervisorClientConfig) error {
	var initErr error

	supervisorClientOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
		defer cancel()

		conn, err := grpc.DialContext(ctx, config.Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                30 * time.Second,
				Timeout:             10 * time.Second,
				PermitWithoutStream: true,
			}),
		)
		if err != nil {
			initErr = fmt.Errorf("failed to connect to supervisor at %s: %w", config.Address, err)
			return
		}

		supervisorClientInstance = &SupervisorClient{
			conn:   conn,
			client: pb.NewSupervisorServiceClient(conn),
			config: config,
		}

		log.Printf("Connected to supervisor gRPC service at %s (worker: %s)", config.Address, config.WorkerID)
	})

	return initErr
}

// CloseSupervisorClient closes the supervisor connection
func CloseSupervisorClient() {
	if supervisorClientInstance != nil {
		supervisorClientInstance.mu.Lock()
		defer supervisorClientInstance.mu.Unlock()

		if supervisorClientInstance.conn != nil {
			supervisorClientInstance.conn.Close()
			supervisorClientInstance.conn = nil
			supervisorClientInstance.client = nil
		}
	}
}

// GetConfig returns the current configuration
func (sc *SupervisorClient) GetConfig() SupervisorClientConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.config
}

// RegisterWorker registers this worker with the supervisor
func RegisterWorker(ctx context.Context, workerId string) error {
	sc := GetSupervisorClient()
	if sc == nil {
		return fmt.Errorf("supervisor client not initialized")
	}

	sc.mu.RLock()
	client := sc.client
	config := sc.config
	sc.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("supervisor client connection lost")
	}

	if workerId == "" {
		workerId = config.WorkerID
	}

	req := &pb.RegisterWorkerRequest{
		WorkerId: workerId,
		HostName: config.WorkerHostName,
		Port:     config.WorkerPort,
	}

	reqCtx, cancel := context.WithTimeout(ctx, config.RequestTimeout)
	defer cancel()

	resp, err := client.RegisterWorker(reqCtx, req)
	if err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	if !resp.Success {
		log.Printf("Supervisor reported error during registration: %s", resp.Message)
		return fmt.Errorf("registration error: %s", resp.Message)
	}

	log.Printf("Successfully registered worker %s: %s", workerId, resp.Message)
	return nil
}

// StartHeartbeat sends periodic heartbeats to the supervisor.
// Blocks until ctx is cancelled.
func StartHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping heartbeat")
			return
		case <-ticker.C:
			sendHeartbeat(ctx)
		}
	}
}

func sendHeartbeat(ctx context.Context) {
	sc := GetSupervisorClient()
	if sc == nil {
		return
	}

	sc.mu.RLock()
	client := sc.client
	config := sc.config
	sc.mu.RUnlock()

	if client == nil {
		return
	}

	req := &pb.HeartbeatRequest{
		WorkerId:   config.WorkerID,
		ActiveJobs: ActiveJobCount.Load(),
		Capacity:   WorkerCapacity,
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.Heartbeat(reqCtx, req)
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
		return
	}

	if !resp.Acknowledged {
		log.Printf("Heartbeat not acknowledged by supervisor")
	}
}
