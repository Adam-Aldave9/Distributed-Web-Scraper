package Services

import (
	"context"
	"log"
	"net"
	"testing"

	Models "supervisor/Models"
	pb "supervisor/proto"

	"github.com/glebarez/sqlite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

const testBufSize = 1024 * 1024

// setupTestServer starts an in-memory gRPC server backed by SQLite and returns a client.
func setupTestServer(t *testing.T) (pb.SupervisorServiceClient, func()) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	db.AutoMigrate(&Models.Worker{})
	Models.WorkerDB = db

	lis := bufconn.Listen(testBufSize)
	server := grpc.NewServer()
	pb.RegisterSupervisorServiceServer(server, NewSupervisorGRPCServer(GRPCConfig{Port: "0"}))

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

	client := pb.NewSupervisorServiceClient(conn)

	cleanup := func() {
		conn.Close()
		server.Stop()
		Models.WorkerDB = nil
	}

	return client, cleanup
}

// --- RegisterWorker tests ---

func TestRegisterWorker_Success(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "test-worker-1",
		HostName: "test-host",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("RegisterWorker RPC failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true, got false: %s", resp.Message)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestRegisterWorker_EmptyID(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "",
		HostName: "test-host",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("RegisterWorker RPC failed: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for empty worker ID")
	}
	if resp.Message != "Worker ID is required" {
		t.Errorf("expected 'Worker ID is required', got '%s'", resp.Message)
	}
}

func TestRegisterWorker_PersistsWorker(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "persist-worker",
		HostName: "test-host",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("RegisterWorker RPC failed: %v", err)
	}

	var worker Models.Worker
	result := Models.WorkerDB.Where("name = ?", "persist-worker").First(&worker)
	if result.Error != nil {
		t.Fatalf("worker not found in DB: %v", result.Error)
	}
	if worker.Name != "persist-worker" {
		t.Errorf("expected name 'persist-worker', got '%s'", worker.Name)
	}
	if worker.Status != "active" {
		t.Errorf("expected status 'active', got '%s'", worker.Status)
	}
	if worker.HostName != "test-host" {
		t.Errorf("expected hostname 'test-host', got '%s'", worker.HostName)
	}
}

func TestRegisterWorker_ReregistrationUpdates(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	// First registration
	_, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "reregister-worker",
		HostName: "host-1",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("first RegisterWorker failed: %v", err)
	}

	// Re-register with new host
	resp, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "reregister-worker",
		HostName: "host-2",
		Port:     50052,
	})
	if err != nil {
		t.Fatalf("second RegisterWorker failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true on re-registration")
	}

	// Only one record should exist
	var count int64
	Models.WorkerDB.Model(&Models.Worker{}).Where("name = ?", "reregister-worker").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 worker record, got %d", count)
	}

	// Host should be updated
	var worker Models.Worker
	Models.WorkerDB.Where("name = ?", "reregister-worker").First(&worker)
	if worker.HostName != "host-2" {
		t.Errorf("expected hostname 'host-2' after re-registration, got '%s'", worker.HostName)
	}
}

// --- Heartbeat tests ---

func TestHeartbeat_RegisteredWorker(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "hb-worker",
		HostName: "test-host",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	resp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		WorkerId:   "hb-worker",
		ActiveJobs: 1,
		Capacity:   2,
	})
	if err != nil {
		t.Fatalf("Heartbeat RPC failed: %v", err)
	}
	if !resp.Acknowledged {
		t.Error("expected acknowledged=true for registered worker")
	}
}

func TestHeartbeat_EmptyWorkerID(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		WorkerId:   "",
		ActiveJobs: 0,
		Capacity:   2,
	})
	if err != nil {
		t.Fatalf("Heartbeat RPC failed: %v", err)
	}
	if resp.Acknowledged {
		t.Error("expected acknowledged=false for empty worker ID")
	}
}

func TestHeartbeat_UnknownWorker(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		WorkerId:   "unknown-worker",
		ActiveJobs: 0,
		Capacity:   2,
	})
	if err != nil {
		t.Fatalf("Heartbeat RPC failed: %v", err)
	}
	if resp.Acknowledged {
		t.Error("expected acknowledged=false for unknown worker")
	}
}

func TestHeartbeat_UpdatesActiveJobs(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	_, _ = client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "jobs-worker",
		HostName: "test-host",
		Port:     50051,
	})

	_, _ = client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		WorkerId:   "jobs-worker",
		ActiveJobs: 2,
		Capacity:   4,
	})

	var worker Models.Worker
	Models.WorkerDB.Where("name = ?", "jobs-worker").First(&worker)
	if worker.ActiveJobs != 2 {
		t.Errorf("expected active_jobs=2, got %d", worker.ActiveJobs)
	}
	if worker.Capacity != 4 {
		t.Errorf("expected capacity=4, got %d", worker.Capacity)
	}
}

// --- Workflow tests ---

func TestWorkflow_RegisterAndHeartbeat(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	// Step 1: Register
	regResp, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
		WorkerId: "workflow-worker",
		HostName: "test-host",
		Port:     50051,
	})
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}
	if !regResp.Success {
		t.Fatal("registration should succeed")
	}

	// Step 2: Heartbeat while working
	hb1, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		WorkerId:   "workflow-worker",
		ActiveJobs: 2,
		Capacity:   2,
	})
	if err != nil {
		t.Fatalf("first Heartbeat failed: %v", err)
	}
	if !hb1.Acknowledged {
		t.Error("heartbeat should be acknowledged")
	}

	// Step 3: Heartbeat after finishing work
	hb2, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
		WorkerId:   "workflow-worker",
		ActiveJobs: 0,
		Capacity:   2,
	})
	if err != nil {
		t.Fatalf("second Heartbeat failed: %v", err)
	}
	if !hb2.Acknowledged {
		t.Error("second heartbeat should be acknowledged")
	}

	// Verify final DB state
	var worker Models.Worker
	Models.WorkerDB.Where("name = ?", "workflow-worker").First(&worker)
	if worker.ActiveJobs != 0 {
		t.Errorf("expected active_jobs=0, got %d", worker.ActiveJobs)
	}
	if worker.Status != "active" {
		t.Errorf("expected status 'active', got '%s'", worker.Status)
	}
}

func TestWorkflow_MultipleWorkers(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	workers := []string{"worker-a", "worker-b", "worker-c"}
	for _, id := range workers {
		resp, err := client.RegisterWorker(context.Background(), &pb.RegisterWorkerRequest{
			WorkerId: id,
			HostName: id + "-host",
			Port:     50051,
		})
		if err != nil {
			t.Fatalf("RegisterWorker(%s) failed: %v", id, err)
		}
		if !resp.Success {
			t.Errorf("registration for %s should succeed", id)
		}
	}

	// All workers send heartbeats
	for i, id := range workers {
		resp, err := client.Heartbeat(context.Background(), &pb.HeartbeatRequest{
			WorkerId:   id,
			ActiveJobs: int32(i),
			Capacity:   2,
		})
		if err != nil {
			t.Fatalf("Heartbeat(%s) failed: %v", id, err)
		}
		if !resp.Acknowledged {
			t.Errorf("heartbeat for %s should be acknowledged", id)
		}
	}

	// Verify all workers exist in DB with correct state
	var count int64
	Models.WorkerDB.Model(&Models.Worker{}).Where("status = ?", "active").Count(&count)
	if count != int64(len(workers)) {
		t.Errorf("expected %d active workers, got %d", len(workers), count)
	}
}
