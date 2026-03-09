package Services

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	Models "supervisor/Models"
	"time"

	pb "supervisor/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Port string
}

// DefaultGRPCConfig returns default gRPC configuration
func DefaultGRPCConfig() GRPCConfig {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	return GRPCConfig{
		Port: port,
	}
}

// SupervisorGRPCServer implements the SupervisorService gRPC server
type SupervisorGRPCServer struct {
	pb.UnimplementedSupervisorServiceServer
	config GRPCConfig
}

// NewSupervisorGRPCServer creates a new SupervisorGRPCServer with the given config
func NewSupervisorGRPCServer(config GRPCConfig) *SupervisorGRPCServer {
	return &SupervisorGRPCServer{
		config: config,
	}
}

// RegisterWorker registers a new worker with the supervisor
func (s *SupervisorGRPCServer) RegisterWorker(ctx context.Context, req *pb.RegisterWorkerRequest) (*pb.RegisterWorkerResponse, error) {
	if req.WorkerId == "" {
		return &pb.RegisterWorkerResponse{
			Success: false,
			Message: "Worker ID is required",
		}, nil
	}

	log.Printf("Received worker registration: %s (host: %s, port: %d)", req.WorkerId, req.HostName, req.Port)

	worker := Models.Worker{
		Name:          req.WorkerId,
		Status:        "active",
		HostName:      req.HostName,
		LastHeartbeat: time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if Models.WorkerDB != nil {
		result := Models.WorkerDB.Where("name = ?", req.WorkerId).FirstOrCreate(&worker)
		if result.Error != nil {
			log.Printf("Error registering worker %s: %v", req.WorkerId, result.Error)
			return &pb.RegisterWorkerResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to register worker: %v", result.Error),
			}, nil
		}

		// update existing worker with new host info
		if result.RowsAffected == 0 {
			Models.WorkerDB.Model(&worker).Updates(Models.Worker{
				Status:        "active",
				HostName:      req.HostName,
				LastHeartbeat: time.Now(),
				UpdatedAt:     time.Now(),
			})
		}
	}

	return &pb.RegisterWorkerResponse{
		Success: true,
		Message: fmt.Sprintf("Worker %s registered successfully", req.WorkerId),
	}, nil
}

// Heartbeat handles periodic health updates from workers
func (s *SupervisorGRPCServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if req.WorkerId == "" {
		return &pb.HeartbeatResponse{Acknowledged: false}, nil
	}

	if Models.WorkerDB != nil {
		result := Models.WorkerDB.Model(&Models.Worker{}).Where("name = ?", req.WorkerId).Updates(map[string]interface{}{
			"status":         "active",
			"active_jobs":    int(req.ActiveJobs),
			"capacity":       int(req.Capacity),
			"last_heartbeat": time.Now(),
			"updated_at":     time.Now(),
		})
		if result.Error != nil {
			log.Printf("Error updating heartbeat for worker %s: %v", req.WorkerId, result.Error)
			return &pb.HeartbeatResponse{Acknowledged: false}, nil
		}
		if result.RowsAffected == 0 {
			log.Printf("Heartbeat from unknown worker %s, ignoring", req.WorkerId)
			return &pb.HeartbeatResponse{Acknowledged: false}, nil
		}
	}

	return &pb.HeartbeatResponse{Acknowledged: true}, nil
}

// StartGRPCServer starts the gRPC server with the given configuration
func StartGRPCServer() error {
	return StartGRPCServerWithConfig(DefaultGRPCConfig())
}

// StartGRPCServerWithConfig starts the gRPC server with custom configuration
func StartGRPCServerWithConfig(config GRPCConfig) error {
	address := fmt.Sprintf(":%s", config.Port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              2 * time.Minute,
			Timeout:           20 * time.Second,
		}),
	)

	pb.RegisterSupervisorServiceServer(grpcServer, NewSupervisorGRPCServer(config))

	log.Printf("Supervisor gRPC server starting on port %s...", config.Port)

	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve gRPC server: %w", err)
	}

	return nil
}
