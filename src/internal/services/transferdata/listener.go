package transferdata

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"time"
	"urlshort/internal/repository/mongo/stats"
	"urlshort/internal/services/collector"
)

func init() {
	gob.Register(stats.StatUpdate{})
}

type Listener struct {
	socketPath string
	collector  *collector.Collector
	listener   net.Listener
}

func NewListener(socketPath string, collect *collector.Collector) *Listener {
	return &Listener{
		socketPath: socketPath,
		collector:  collect,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	_ = os.Remove(l.socketPath)

	listener, err := net.Listen("unix", l.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %s: %w", l.socketPath, err)
	}
	l.listener = listener

	go func() {
		<-ctx.Done()
		_ = l.listener.Close()
		_ = os.Remove(l.socketPath)
	}()

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():

				return nil
			default:
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		go l.handleConnection(conn)
	}
}

func (l *Listener) handleConnection(conn net.Conn) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)

	for {
		var update stats.StatUpdate
		if err := decoder.Decode(&update); err != nil {
			log.Printf("[ERROR handleConnection] Decode error: %s", err.Error())
			return
		}

		l.collector.Push(update)
	}
}
