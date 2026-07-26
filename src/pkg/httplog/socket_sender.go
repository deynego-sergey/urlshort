package httplog

import (
	"context"
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
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
		time.Sleep(200 * time.Millisecond)
		return nil
	}

	sort.Strings(matches)

	conn, err := net.Dial("unix", s.socketPath)
	if err != nil {
		// Сокет недоступен. Файлы накапливаются на диске до восстановления.
		return fmt.Errorf("dial unix socket failed: %w", err)
	}
	defer conn.Close()

	for _, filePath := range matches {
		if err := s.sendFile(ctx, conn, filePath); err != nil {
			// В случае сбоя передачи файл не удаляется
			// и вычитается заново при следующей итерации.
			return err
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("remove sent ready file failed: %w", err)
		}
	}

	return nil
}

func (s *SocketSender) sendFile(ctx context.Context, conn net.Conn, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open ready file failed: %w", err)
	}
	defer file.Close()

	for {
		payload, err := ReadPayloadContext(ctx, file)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read payload from ready file failed: %w", err)
		}

		if err := WritePayloadContext(ctx, conn, payload); err != nil {
			return fmt.Errorf("write payload to socket failed: %w", err)
		}
	}

	return nil
}
