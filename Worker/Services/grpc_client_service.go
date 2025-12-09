package Services

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
	DTOs "worker/DTOs"

	pb "worker/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

// SendCompletionStatus sends job completion status to supervisor
func SendCompletionStatus(ctx context.Context, payload DTOs.JobPayload, status string, result string) error {
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

	req := &pb.CompletionStatusRequest{
		JobId:         int32(payload.JobID),
		JobName:       payload.Name,
		Status:        status,
		UrlSeedSearch: payload.URLSeedSearch,
		LastResult:    result,
		Cron:          "",
		LastRun:       time.Now().Format(time.RFC3339),
		NextRun:       "",
		IsActive:      status == "completed",
		WorkerId:      config.WorkerID,
		CompletedAt:   timestamppb.Now(),
	}

	reqCtx, cancel := context.WithTimeout(ctx, config.RequestTimeout)
	defer cancel()

	resp, err := client.LogCompletionStatus(reqCtx, req)
	if err != nil {
		return fmt.Errorf("failed to send completion status: %w", err)
	}

	if !resp.Success {
		log.Printf("Supervisor reported error: %s", resp.Message)
		return fmt.Errorf("supervisor error: %s", resp.Message)
	}

	log.Printf("Successfully sent completion status for job %d: %s", payload.JobID, resp.Message)
	return nil
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

	// Use provided workerId or fall back to config
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

// HeartbeatStream manages a continuous heartbeat stream to the supervisor
type HeartbeatStream struct {
	stream   pb.SupervisorService_HealthHeartbeatStreamClient
	interval time.Duration
	stopChan chan struct{}
	mu       sync.Mutex
	running  bool
}

var (
	heartbeatStreamInstance *HeartbeatStream
	heartbeatStreamMu       sync.Mutex
)

// StartHeartbeatStream starts a continuous heartbeat stream to the supervisor
func StartHeartbeatStream(ctx context.Context, interval time.Duration) error {
	heartbeatStreamMu.Lock()
	defer heartbeatStreamMu.Unlock()

	if heartbeatStreamInstance != nil && heartbeatStreamInstance.running {
		return nil // Already running
	}

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

	stream, err := client.HealthHeartbeatStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open heartbeat stream: %w", err)
	}

	heartbeatStreamInstance = &HeartbeatStream{
		stream:   stream,
		interval: interval,
		stopChan: make(chan struct{}),
		running:  true,
	}

	// Start the heartbeat sender goroutine
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer func() {
			heartbeatStreamMu.Lock()
			if heartbeatStreamInstance != nil {
				heartbeatStreamInstance.running = false
			}
			heartbeatStreamMu.Unlock()
		}()

		workerId := config.WorkerID
		heartbeatCount := 0

		for {
			select {
			case <-ctx.Done():
				log.Println("Heartbeat stream context cancelled, closing stream")
				resp, err := stream.CloseAndRecv()
				if err != nil {
					log.Printf("Error closing heartbeat stream: %v", err)
				} else {
					log.Printf("Heartbeat stream closed: %s", resp.Message)
				}
				return
			case <-heartbeatStreamInstance.stopChan:
				log.Println("Heartbeat stream stopped")
				resp, err := stream.CloseAndRecv()
				if err != nil {
					log.Printf("Error closing heartbeat stream: %v", err)
				} else {
					log.Printf("Heartbeat stream closed: %s", resp.Message)
				}
				return
			case <-ticker.C:
				heartbeatCount++
				req := &pb.HealthHeartbeatRequest{
					WorkerId:    workerId,
					Timestamp:   time.Now().Format(time.RFC3339),
					CpuUsage:    0, // TODO: Get actual CPU usage
					MemoryUsage: 0, // TODO: Get actual memory usage
					Status:      "active",
				}

				if err := stream.Send(req); err != nil {
					log.Printf("Failed to send heartbeat #%d: %v", heartbeatCount, err)
					return
				}
				log.Printf("Sent heartbeat #%d for worker %s", heartbeatCount, workerId)
			}
		}
	}()

	log.Printf("Started heartbeat stream for worker %s (interval: %v)", config.WorkerID, interval)
	return nil
}

// StopHeartbeatStream stops the heartbeat stream
func StopHeartbeatStream() {
	heartbeatStreamMu.Lock()
	defer heartbeatStreamMu.Unlock()

	if heartbeatStreamInstance != nil && heartbeatStreamInstance.running {
		close(heartbeatStreamInstance.stopChan)
		heartbeatStreamInstance = nil
	}
}

// SendHealthHeartbeat sends a single health heartbeat (for backwards compatibility)
// Note: For continuous heartbeats, use StartHeartbeatStream instead
func SendHealthHeartbeat(ctx context.Context, workerId string) error {
	return SendHealthHeartbeatWithMetrics(ctx, workerId, 0, 0, "active")
}

// SendHealthHeartbeatWithMetrics sends a single health heartbeat with resource metrics
// Note: This opens a stream, sends one heartbeat, and closes the stream
// For continuous heartbeats, use StartHeartbeatStream instead
func SendHealthHeartbeatWithMetrics(ctx context.Context, workerId string, cpuUsage, memoryUsage float64, status string) error {
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

	// Use provided workerId or fall back to config
	if workerId == "" {
		workerId = config.WorkerID
	}

	// Open a stream, send one heartbeat, and close
	stream, err := client.HealthHeartbeatStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open heartbeat stream: %w", err)
	}

	req := &pb.HealthHeartbeatRequest{
		WorkerId:    workerId,
		Timestamp:   time.Now().Format(time.RFC3339),
		CpuUsage:    cpuUsage,
		MemoryUsage: memoryUsage,
		Status:      status,
	}

	if err := stream.Send(req); err != nil {
		return fmt.Errorf("failed to send health heartbeat: %w", err)
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("failed to close heartbeat stream: %w", err)
	}

	if !resp.Success {
		log.Printf("Supervisor reported error for heartbeat: %s", resp.Message)
		return fmt.Errorf("heartbeat error: %s", resp.Message)
	}

	log.Printf("Successfully sent heartbeat for worker %s", workerId)
	return nil
}
