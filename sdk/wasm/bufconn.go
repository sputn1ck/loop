package wasm

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const (
	// DefaultBufconnBufferSize is the default in-process gRPC connection
	// buffer size.
	DefaultBufconnBufferSize = 1 << 20

	bufconnTarget = "passthrough:///loop-bufconn"
)

var (
	// ErrBufconnServerClosed indicates that the in-process server has
	// stopped accepting connections.
	ErrBufconnServerClosed = errors.New("bufconn server is closed")
)

// RegisterBufconnServices registers services before the gRPC server starts.
type RegisterBufconnServices func(*grpc.Server) error

// BufconnServer owns an in-process gRPC server and listener. It is useful for
// connecting browser-facing wrappers to an embedded daemon without a TCP
// listener.
type BufconnServer struct {
	listener *bufconn.Listener
	server   *grpc.Server
	done     chan struct{}

	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error

	errMu    sync.RWMutex
	serveErr error
}

// StartBufconnServer registers services and starts an in-process gRPC server.
func StartBufconnServer(bufferSize int, register RegisterBufconnServices,
	serverOptions ...grpc.ServerOption) (*BufconnServer, error) {

	if bufferSize <= 0 {
		return nil, errors.New("bufconn buffer size must be positive")
	}
	if register == nil {
		return nil, errors.New("bufconn service registrar is required")
	}

	listener := bufconn.Listen(bufferSize)
	grpcServer := grpc.NewServer(serverOptions...)
	if err := register(grpcServer); err != nil {
		grpcServer.Stop()
		_ = listener.Close()

		return nil, err
	}

	server := &BufconnServer{
		listener: listener,
		server:   grpcServer,
		done:     make(chan struct{}),
	}
	go server.serve()

	return server, nil
}

// ClientConn creates a lazy gRPC client connection to the in-process server.
// The bufconn dialer and local insecure transport credentials override any
// conflicting caller options.
func (s *BufconnServer) ClientConn(
	dialOptions ...grpc.DialOption) (*grpc.ClientConn, error) {

	if s == nil || s.closed.Load() {
		return nil, ErrBufconnServerClosed
	}

	options := append([]grpc.DialOption(nil), dialOptions...)
	options = append(
		options,
		grpc.WithContextDialer(
			func(ctx context.Context, _ string) (net.Conn, error) {
				return s.listener.DialContext(ctx)
			},
		),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	connection, err := grpc.NewClient(bufconnTarget, options...)
	if err != nil {
		return nil, err
	}

	return connection, nil
}

// Done is closed after the gRPC serve loop exits.
func (s *BufconnServer) Done() <-chan struct{} {
	return s.done
}

// Err returns the terminal serve error, if any, after Done is closed.
func (s *BufconnServer) Err() error {
	s.errMu.RLock()
	defer s.errMu.RUnlock()

	return s.serveErr
}

// Close immediately stops the gRPC server and releases the listener. Close is
// idempotent.
func (s *BufconnServer) Close() error {
	if s == nil {
		return nil
	}

	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.server.Stop()
		_ = s.listener.Close()
		<-s.done
		s.closeErr = s.Err()
	})

	return s.closeErr
}

func (s *BufconnServer) serve() {
	err := s.server.Serve(s.listener)
	if errors.Is(err, grpc.ErrServerStopped) || errors.Is(err, net.ErrClosed) {
		err = nil
	}

	s.errMu.Lock()
	s.serveErr = err
	s.errMu.Unlock()
	s.closed.Store(true)
	close(s.done)
}
