// src/pkg/httplog/listener.go
package httplog

import (
	"context"
	"encoding/gob"
	"errors"
	"io"
	"log"
	"net"
	"time"
)

type HandlerFunc func(ctx context.Context, payload *RequestPayload) error

type SocketListener struct {
	socketPath string
	handler    HandlerFunc
	retryDelay time.Duration
}

func NewSocketListener(socketPath string, handler HandlerFunc) *SocketListener {
	return &SocketListener{
		socketPath: socketPath,
		handler:    handler,
		retryDelay: 2 * time.Second,
	}
}

func (l *SocketListener) ListenAndServe(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := net.Dial("unix", l.socketPath)
		if err != nil {
			log.Printf("failed to connect to socket %s: %v. Retrying in %v...", l.socketPath, err, l.retryDelay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(l.retryDelay):
				continue
			}
		}

		log.Printf("connected to socket at %s", l.socketPath)
		l.readLoop(ctx, conn)
	}
}

func (l *SocketListener) readLoop(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	dec := gob.NewDecoder(conn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		payload, err := ReadPayload(dec)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Println("socket connection closed by server")
				return
			}
			log.Printf("read payload failed: %v", err)
			return
		}

		if err := l.handler(ctx, payload); err != nil {
			log.Printf("handler failed: %v", err)
		}
	}
}
