package Services

import (
	"context"
	"log"
	"net"
	"os"
	"testing"
	"time"

	pb "worker/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const testBufSize = 1024 * 1024

func setupWorkerTestServer(t *testing.T, shutdownChan chan os.Signal) (pb.WorkerServiceClient, func()) {
	t.Helper()

	lis := bufconn.Listen(testBufSize)
	server := grpc.NewServer()
	workerServer := NewWorkerGRPCServer(shutdownChan, WorkerServerConfig{Port: "0"})
	pb.RegisterWorkerServiceServer(server, workerServer)

	go func() {
		if err := server.Serve(lis); err != nil {
			log.Printf("test server exited: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client := pb.NewWorkerServiceClient(conn)

	cleanup := func() {
		conn.Close()
		server.Stop()
	}

	return client, cleanup
}

func TestShutdown_ReturnsResponse(t *testing.T) {
	shutdownChan := make(chan os.Signal, 1)
	client, cleanup := setupWorkerTestServer(t, shutdownChan)
	defer cleanup()

	resp, err := client.Shutdown(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Shutdown RPC failed: %v", err)
	}
	if resp.Message != "Shutdown initiated" {
		t.Errorf("expected message 'Shutdown initiated', got '%s'", resp.Message)
	}
	if resp.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestShutdown_ValidTimestamp(t *testing.T) {
	shutdownChan := make(chan os.Signal, 1)
	client, cleanup := setupWorkerTestServer(t, shutdownChan)
	defer cleanup()

	resp, err := client.Shutdown(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Shutdown RPC failed: %v", err)
	}

	_, parseErr := time.Parse(time.RFC3339, resp.Timestamp)
	if parseErr != nil {
		t.Errorf("timestamp '%s' is not valid RFC3339: %v", resp.Timestamp, parseErr)
	}
}

func TestShutdown_SendsInterruptSignal(t *testing.T) {
	shutdownChan := make(chan os.Signal, 1)
	client, cleanup := setupWorkerTestServer(t, shutdownChan)
	defer cleanup()

	_, err := client.Shutdown(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Shutdown RPC failed: %v", err)
	}

	// The handler sends os.Interrupt after a 100ms delay
	select {
	case sig := <-shutdownChan:
		if sig != os.Interrupt {
			t.Errorf("expected os.Interrupt, got %v", sig)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for shutdown signal")
	}
}
