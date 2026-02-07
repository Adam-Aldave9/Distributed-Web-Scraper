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

// HealthHeartbeatStream receives a continuous stream of health updates from workers
func (s *SupervisorGRPCServer) HealthHeartbeatStream(stream pb.SupervisorService_HealthHeartbeatStreamServer) error {
	var lastWorkerId string
	var heartbeatCount int

	for {
		req, err := stream.Recv()
		if err != nil {
			if heartbeatCount > 0 {
				log.Printf("Heartbeat stream ended for worker %s after %d heartbeats", lastWorkerId, heartbeatCount)
			}
			return stream.SendAndClose(&pb.HealthHeartbeatResponse{
				Success: true,
				Message: fmt.Sprintf("Received %d heartbeats", heartbeatCount),
			})
		}

		if req.WorkerId == "" {
			continue
		}

		lastWorkerId = req.WorkerId
		heartbeatCount++

		log.Printf("Received heartbeat #%d from worker %s (CPU: %.2f%%, Memory: %.2f%%, Status: %s)",
			heartbeatCount, req.WorkerId, req.CpuUsage, req.MemoryUsage, req.Status)

		if Models.WorkerDB != nil {
			result := Models.WorkerDB.Model(&Models.Worker{}).
				Where("name = ?", req.WorkerId).
				Updates(map[string]interface{}{
					"last_heartbeat": time.Now(),
					"status":         req.Status,
					"updated_at":     time.Now(),
				})

			if result.Error != nil {
				log.Printf("Error updating heartbeat for worker %s: %v", req.WorkerId, result.Error)
			}
		}
	}
}

// LogCompletionStatus logs job completion status from workers
func (s *SupervisorGRPCServer) LogCompletionStatus(ctx context.Context, req *pb.CompletionStatusRequest) (*pb.CompletionStatusResponse, error) {
	if req.JobId <= 0 {
		return &pb.CompletionStatusResponse{
			Success: false,
			Message: "Valid Job ID is required",
		}, nil
	}

	log.Printf("Received completion status from worker %s for job %d", req.WorkerId, req.JobId)

	if Models.JobDB == nil {
		log.Printf("Warning: JobDB not initialized, cannot update job %d", req.JobId)
		return &pb.CompletionStatusResponse{
			Success: false,
			Message: "Database not available",
		}, nil
	}

	updateQuery := `UPDATE jobs SET
		status = ?,
		last_result = ?,
		last_run = ?,
		next_run = ?,
		is_active = ?,
		updated_at = ?
		WHERE id = ?`

	result := Models.JobDB.Exec(updateQuery,
		req.Status,
		req.LastResult,
		req.LastRun,
		req.NextRun,
		req.IsActive,
		time.Now(),
		req.JobId)

	if result.Error != nil {
		log.Printf("Error updating job %d: %v", req.JobId, result.Error)
		return &pb.CompletionStatusResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update job: %v", result.Error),
		}, nil
	}

	if result.RowsAffected == 0 {
		log.Printf("No job found with ID %d", req.JobId)
		return &pb.CompletionStatusResponse{
			Success: false,
			Message: fmt.Sprintf("Job with ID %d not found", req.JobId),
		}, nil
	}

	log.Printf("Successfully updated job %d with completion status from worker %s", req.JobId, req.WorkerId)

	return &pb.CompletionStatusResponse{
		Success: true,
		Message: "Job completion status logged successfully",
	}, nil
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
