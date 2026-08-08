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

const writeTimeout = 2 * time.Second

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
		log.Println("Listener failed:", err)
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
				time.Sleep(100 * time.Millisecond) // защита от busy-loop при постоянной ошибке accept
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
		log.Println("Glob failed:", err)
		return fmt.Errorf("glob ready files failed: %w", err)
	}

	if len(matches) == 0 {
		return nil
	}

	if s.clientCount() == 0 {
		return nil // нет клиентов — файлы остаются копиться, как и задумано
	}

	sort.Strings(matches)

	for _, filePath := range matches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		delivered, err := s.sendFile(ctx, filePath)
		if err != nil {
			log.Printf("send file %s failed: %v", filePath, err)
			return err // прерываем этот тик, файл НЕ удалён, попробуем на следующем
		}

		if !delivered {
			// все клиенты отвалились в процессе отправки — файл не трогаем,
			// при следующем тике либо появится клиент и файл отправится заново с начала,
			// либо клиентов снова нет и он просто продолжит копиться
			log.Printf("no clients left mid-send, keeping file: %s", filePath)
			return nil
		}

		if err := os.Remove(filePath); err != nil {
			log.Printf("remove file %s failed: %v", filePath, err)
			return fmt.Errorf("remove sent ready file failed: %w", err)
		}
	}

	return nil
}

// sendFile отправляет файл целиком. Возвращает delivered=true, только если
// файл был отправлен от начала до конца и в момент завершения был хотя бы один живой клиент.
func (s *SocketSender) sendFile(ctx context.Context, filePath string) (delivered bool, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("open ready file failed: %w", err)
	}
	defer file.Close()

	dec := gob.NewDecoder(file)

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		payload, err := ReadPayload(dec)
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("read payload from ready file failed: %w", err)
		}

		if s.broadcastPayload(payload) == 0 {
			// ни одному клиенту не удалось доставить этот payload —
			// прекращаем отправку файла, он останется недоудалённым
			return false, nil
		}
	}

	return true, nil
}

// broadcastPayload рассылает payload всем текущим клиентам.
// Возвращает число клиентов, которым удалось успешно отправить.
// Сетевой I/O выполняется без удержания блокировки на всё время записи.
func (s *SocketSender) broadcastPayload(p *RequestPayload) int {
	s.mu.RLock()
	snapshot := make(map[net.Conn]*gob.Encoder, len(s.clients))
	for c, e := range s.clients {
		snapshot[c] = e
	}
	s.mu.RUnlock()

	if len(snapshot) == 0 {
		return 0
	}

	var dead []net.Conn
	delivered := 0

	for conn, enc := range snapshot {
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := WritePayload(enc, p); err != nil {
			log.Printf("write to client %s failed, closing connection: %v", conn.RemoteAddr(), err)
			_ = conn.Close()
			dead = append(dead, conn)
			continue
		}
		delivered++
	}

	if len(dead) > 0 {
		s.mu.Lock()
		for _, c := range dead {
			delete(s.clients, c)
		}
		s.mu.Unlock()
	}

	return delivered
}

func (s *SocketSender) clientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
