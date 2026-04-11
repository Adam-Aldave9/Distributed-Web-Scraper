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

type WorkerServerConfig struct {
	Port string
}

func DefaultWorkerServerConfig() WorkerServerConfig {
	port := os.Getenv("WORKER_GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	return WorkerServerConfig{
		Port: port,
	}
}

type WorkerGRPCServer struct {
	pb.UnimplementedWorkerServiceServer
	shutdownChan chan os.Signal
	config       WorkerServerConfig
}

func NewWorkerGRPCServer(shutdownChan chan os.Signal, config WorkerServerConfig) *WorkerGRPCServer {
	return &WorkerGRPCServer{
		shutdownChan: shutdownChan,
		config:       config,
	}
}

func (s *WorkerGRPCServer) Shutdown(ctx context.Context, req *emptypb.Empty) (*pb.ShutdownResponse, error) {
	log.Println("Received shutdown request via gRPC")

	go func() {
		time.Sleep(100 * time.Millisecond)
		s.shutdownChan <- os.Interrupt
	}()

	return &pb.ShutdownResponse{
		Message:   "Shutdown initiated",
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

func StartGRPCServer(ctx context.Context, shutdownChan chan os.Signal) error {
	return StartGRPCServerWithConfig(ctx, shutdownChan, DefaultWorkerServerConfig())
}

func StartGRPCServerWithConfig(ctx context.Context, shutdownChan chan os.Signal, config WorkerServerConfig) error {
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

	workerServer := NewWorkerGRPCServer(shutdownChan, config)
	pb.RegisterWorkerServiceServer(grpcServer, workerServer)

	log.Printf("Worker gRPC server starting on port %s...", config.Port)

	go func() {
		<-ctx.Done()
		log.Println("Shutting down gRPC server...")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve gRPC server: %w", err)
	}

	return nil
}
