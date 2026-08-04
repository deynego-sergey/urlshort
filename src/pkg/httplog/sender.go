// src/pkg/httplog/sender.go
package httplog

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type SocketSender struct {
	socketPath string
	logDir     string
	mu         sync.RWMutex
	clients    map[net.Conn]*gob.Encoder
}

func NewSocketSender(socketPath, logDir string) *SocketSender {
	return &SocketSender{
		socketPath: socketPath,
		logDir:     logDir,
		clients:    make(map[net.Conn]*gob.Encoder),
	}
}

func (s *SocketSender) Start(ctx context.Context) error {
	_ = os.Remove(s.socketPath)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket failed: %w", err)
	}
	defer listener.Close()
	defer os.Remove(s.socketPath)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
		_ = os.Remove(s.socketPath)
	}()

	go s.processLoop(ctx)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Printf("accept connection failed: %v", err)
				continue
			}
		}

		s.mu.Lock()
		s.clients[conn] = gob.NewEncoder(conn)
		s.mu.Unlock()

		log.Printf("client connected to socket: %s", conn.RemoteAddr())
	}
}

func (s *SocketSender) processLoop(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.processReadyFiles(ctx); err != nil {
				log.Println("error processing ready files:", err)
			}
		}
	}
}

func (s *SocketSender) processReadyFiles(ctx context.Context) error {
	matches, err := filepath.Glob(filepath.Join(s.logDir, "*.ready"))
	if err != nil {
		return fmt.Errorf("glob ready files failed: %w", err)
	}

	if len(matches) == 0 {
		return nil
	}

	s.mu.RLock()
	clientCount := len(s.clients)
	s.mu.RUnlock()

	if clientCount == 0 {
		return nil
	}

	sort.Strings(matches)

	for _, filePath := range matches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.sendFile(ctx, filePath); err != nil {
			log.Printf("send file %s failed: %v", filePath, err)
			return err
		}

		if err := os.Remove(filePath); err != nil {
			log.Printf("remove file %s failed: %v", filePath, err)
			return fmt.Errorf("remove sent ready file failed: %w", err)
		}
	}

	return nil
}

func (s *SocketSender) sendFile(ctx context.Context, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open ready file failed: %w", err)
	}
	defer file.Close()

	dec := gob.NewDecoder(file)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := ReadPayload(dec)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read payload from ready file failed: %w", err)
		}

		s.broadcastPayload(payload)
	}

	return nil
}

func (s *SocketSender) broadcastPayload(p *RequestPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn, enc := range s.clients {
		if err := WritePayload(enc, p); err != nil {
			log.Printf("write to client %s failed, closing connection: %v", conn.RemoteAddr(), err)
			_ = conn.Close()
			delete(s.clients, conn)
		}
	}
}
