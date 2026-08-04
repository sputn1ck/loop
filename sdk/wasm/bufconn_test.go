package wasm

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func TestBufconnServerLifecycle(t *testing.T) {
	t.Parallel()

	healthServer := health.NewServer()
	healthServer.SetServingStatus("loopd", healthrpc.HealthCheckResponse_SERVING)
	server, err := StartBufconnServer(
		DefaultBufconnBufferSize, func(server *grpc.Server) error {
			healthrpc.RegisterHealthServer(server, healthServer)

			return nil
		},
	)
	if err != nil {
		t.Fatalf("start bufconn server: %v", err)
	}

	connection, err := server.ClientConn()
	if err != nil {
		t.Fatalf("create client connection: %v", err)
	}
	client := healthrpc.NewHealthClient(connection)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := client.Check(ctx, &healthrpc.HealthCheckRequest{
		Service: "loopd",
	})
	if err != nil {
		t.Fatalf("check health: %v", err)
	}
	if response.Status != healthrpc.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected health status: %v", response.Status)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection: %v", err)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}
	select {
	case <-server.Done():
	case <-time.After(time.Second):
		t.Fatalf("server did not stop")
	}
	if err := server.Err(); err != nil {
		t.Fatalf("unexpected serve error: %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
	if _, err := server.ClientConn(); !errors.Is(err, ErrBufconnServerClosed) {

		t.Fatalf("unexpected connection error: %v", err)
	}
}

func TestBufconnServerRegistrationError(t *testing.T) {
	t.Parallel()

	registrationError := errors.New("registration failed")
	_, err := StartBufconnServer(
		DefaultBufconnBufferSize, func(*grpc.Server) error {
			return registrationError
		},
	)
	if !errors.Is(err, registrationError) {
		t.Fatalf("registration error not preserved: %v", err)
	}
}

func TestBufconnServerConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := StartBufconnServer(0, func(*grpc.Server) error {
		return nil
	}); err == nil {
		t.Fatalf("zero buffer size accepted")
	}
	if _, err := StartBufconnServer(
		DefaultBufconnBufferSize, nil,
	); err == nil {

		t.Fatalf("nil registrar accepted")
	}
}
