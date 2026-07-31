package httplog

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type SocketSender struct {
	socketPath string
	logDir     string
	retryDelay time.Duration
}

func NewSocketSender(socketPath, logDir string) *SocketSender {
	return &SocketSender{
		socketPath: socketPath,
		logDir:     logDir,
		retryDelay: 1 * time.Second,
	}
}

func (s *SocketSender) Start(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.processReadyFiles(ctx); err != nil {
				time.Sleep(s.retryDelay)
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

	sort.Strings(matches)

	conn, err := net.Dial("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("dial unix socket failed: %w", err)
	}
	defer conn.Close()

	enc := gob.NewEncoder(conn)

	for _, filePath := range matches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.sendFile(ctx, enc, filePath); err != nil {
			return err
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("remove sent ready file failed: %w", err)
		}
	}

	return nil
}

func (s *SocketSender) sendFile(ctx context.Context, enc *gob.Encoder, filePath string) error {
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

		if err := WritePayload(enc, payload); err != nil {
			return fmt.Errorf("write payload to socket failed: %w", err)
		}
	}

	return nil
}
