package httplog

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
)

type HandlerFunc func(ctx context.Context, payload *RequestPayload) error

type SocketListener struct {
	socketPath string
	handler    HandlerFunc
}

func NewSocketListener(socketPath string, handler HandlerFunc) *SocketListener {
	return &SocketListener{
		socketPath: socketPath,
		handler:    handler,
	}
}

func (l *SocketListener) ListenAndServe(ctx context.Context) error {
	_ = os.Remove(l.socketPath)

	listener, err := net.Listen("unix", l.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket failed: %w", err)
	}
	defer listener.Close()
	defer os.Remove(l.socketPath)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept connection failed: %w", err)
			}
		}

		go l.handleConn(ctx, conn)
	}
}

func (l *SocketListener) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	for {
		payload, err := ReadPayloadContext(ctx, conn)
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}

		if err := l.handler(ctx, payload); err != nil {
			return
		}
	}
}
