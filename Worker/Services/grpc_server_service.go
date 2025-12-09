package Services

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "worker/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/emptypb"
)

// WorkerServerConfig holds gRPC server configuration for worker
type WorkerServerConfig struct {
	Port string
}

// DefaultWorkerServerConfig returns default gRPC server configuration
func DefaultWorkerServerConfig() WorkerServerConfig {
	port := os.Getenv("WORKER_GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	return WorkerServerConfig{
		Port: port,
	}
}

// RestartSignal is sent when a restart is requested
type RestartSignal struct{}

// WorkerGRPCServer implements the WorkerService gRPC server
type WorkerGRPCServer struct {
	pb.UnimplementedWorkerServiceServer
	shutdownChan chan os.Signal
	restartChan  chan RestartSignal
	config       WorkerServerConfig
}

// NewWorkerGRPCServer creates a new WorkerGRPCServer
func NewWorkerGRPCServer(shutdownChan chan os.Signal, config WorkerServerConfig) *WorkerGRPCServer {
	return &WorkerGRPCServer{
		shutdownChan: shutdownChan,
		restartChan:  make(chan RestartSignal, 1),
		config:       config,
	}
}

// NewWorkerGRPCServerWithRestart creates a WorkerGRPCServer with restart support
func NewWorkerGRPCServerWithRestart(shutdownChan chan os.Signal, restartChan chan RestartSignal, config WorkerServerConfig) *WorkerGRPCServer {
	return &WorkerGRPCServer{
		shutdownChan: shutdownChan,
		restartChan:  restartChan,
		config:       config,
	}
}

// Shutdown handles graceful shutdown requests from the supervisor
func (s *WorkerGRPCServer) Shutdown(ctx context.Context, req *emptypb.Empty) (*pb.ShutdownResponse, error) {
	log.Println("Received shutdown request via gRPC")

	// Send shutdown signal to main process asynchronously
	go func() {
		// Small delay to allow response to be sent
		time.Sleep(100 * time.Millisecond)
		s.shutdownChan <- os.Interrupt
	}()

	return &pb.ShutdownResponse{
		Message:   "Shutdown initiated",
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// Restart handles restart requests from the supervisor
func (s *WorkerGRPCServer) Restart(ctx context.Context, req *emptypb.Empty) (*pb.RestartResponse, error) {
	log.Println("Received restart request via gRPC")

	// Send restart signal asynchronously
	go func() {
		// Small delay to allow response to be sent
		time.Sleep(100 * time.Millisecond)
		if s.restartChan != nil {
			select {
			case s.restartChan <- RestartSignal{}:
				log.Println("Restart signal sent")
			default:
				log.Println("Restart channel full, triggering shutdown for restart")
				s.shutdownChan <- os.Interrupt
			}
		} else {
			// Fall back to shutdown if no restart channel configured
			log.Println("No restart channel configured, falling back to shutdown")
			s.shutdownChan <- os.Interrupt
		}
	}()

	return &pb.RestartResponse{
		Message:   "Restart initiated",
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// StartGRPCServer starts the gRPC server with default configuration
func StartGRPCServer(shutdownChan chan os.Signal) error {
	return StartGRPCServerWithConfig(shutdownChan, DefaultWorkerServerConfig())
}

// StartGRPCServerWithConfig starts the gRPC server with custom configuration
func StartGRPCServerWithConfig(shutdownChan chan os.Signal, config WorkerServerConfig) error {
	address := fmt.Sprintf(":%s", config.Port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	// Configure gRPC server with keepalive settings
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              2 * time.Minute,
			Timeout:           20 * time.Second,
		}),
	)

	workerServer := NewWorkerGRPCServer(shutdownChan, config)
	pb.RegisterWorkerServiceServer(grpcServer, workerServer)

	log.Printf("Worker gRPC server starting on port %s...", config.Port)

	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve gRPC server: %w", err)
	}

	return nil
}
