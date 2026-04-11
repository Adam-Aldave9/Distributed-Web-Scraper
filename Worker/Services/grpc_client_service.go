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

// shared with scraper_service.go for heartbeat reporting
var ActiveJobCount atomic.Int32

const WorkerCapacity = 2 // max parallel jobs per worker

const maxHeartbeatFailures = 3

var consecutiveHeartbeatFailures atomic.Int32

type SupervisorClientConfig struct {
	Address        string
	WorkerID       string
	WorkerHostName string
	WorkerPort     int32
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
}

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

// wraps the gRPC client with thread-safe operations
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

func GetSupervisorClient() *SupervisorClient {
	return supervisorClientInstance
}

func InitSupervisorClient() error {
	return InitSupervisorClientWithConfig(DefaultSupervisorClientConfig())
}

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

// closes the existing gRPC connection and establishes a new one.
// used to recover from connection failures detected by heartbeat.
func (sc *SupervisorClient) Reconnect() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.conn != nil {
		sc.conn.Close()
		sc.conn = nil
		sc.client = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), sc.config.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, sc.config.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to reconnect to supervisor at %s: %w", sc.config.Address, err)
	}

	sc.conn = conn
	sc.client = pb.NewSupervisorServiceClient(conn)
	log.Printf("Reconnected to supervisor gRPC service at %s", sc.config.Address)
	return nil
}

func (sc *SupervisorClient) GetConfig() SupervisorClientConfig {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.config
}

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

// blocks until ctx is cancelled
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

	// No active connection — attempt reconnect before sending
	if client == nil {
		log.Println("Heartbeat: no active connection, attempting reconnect")
		if err := sc.Reconnect(); err != nil {
			log.Printf("Heartbeat skipped, reconnect failed: %v", err)
			return
		}
		sc.mu.RLock()
		client = sc.client
		config = sc.config
		sc.mu.RUnlock()
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
		failures := consecutiveHeartbeatFailures.Add(1)
		log.Printf("Heartbeat failed (%d consecutive): %v", failures, err)

		if failures >= maxHeartbeatFailures {
			log.Printf("Too many heartbeat failures, attempting reconnect")
			if reconnErr := sc.Reconnect(); reconnErr != nil {
				log.Printf("Reconnect failed: %v", reconnErr)
			} else {
				consecutiveHeartbeatFailures.Store(0)
			}
		}
		return
	}

	consecutiveHeartbeatFailures.Store(0)

	if !resp.Acknowledged {
		log.Printf("Heartbeat not acknowledged, re-registering with supervisor")
		if regErr := RegisterWorker(ctx, config.WorkerID); regErr != nil {
			log.Printf("Re-registration failed: %v", regErr)
		}
	}
}
