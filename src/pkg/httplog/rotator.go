// src/pkg/httplog/rotator.go
package httplog

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileRotator struct {
	mu           sync.Mutex
	dir          string
	maxSizeBytes int64
	currentFile  *os.File
	currentEnc   *gob.Encoder
	currentSize  int64
	activePath   string
}

// NewFileRotator -
func NewFileRotator(dir string, maxSizeBytes int64) (*FileRotator, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Println("Error creating directory:", err)
		return nil, fmt.Errorf("mkdir log dir failed: %w", err)
	}

	activePath := filepath.Join(dir, "active.log")

	if info, err := os.Stat(activePath); err == nil && info.Size() > 0 {
		readyPath := filepath.Join(dir, fmt.Sprintf("chunk_%d.ready", time.Now().UnixNano()))
		_ = os.Rename(activePath, readyPath)
	}

	r := &FileRotator{
		dir:          dir,
		maxSizeBytes: maxSizeBytes,
		activePath:   activePath,
	}

	if err := r.openActiveFile(); err != nil {
		log.Println("Error opening active file:", err)
		return nil, err
	}

	return r, nil
}

func (r *FileRotator) openActiveFile() error {
	f, err := os.OpenFile(r.activePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open active log failed: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat active log failed: %w", err)
	}

	r.currentFile = f
	r.currentEnc = gob.NewEncoder(f)
	r.currentSize = info.Size()
	return nil
}

func (r *FileRotator) RunPeriodicRotation(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.ForceRotate(); err != nil {
				log.Printf("periodic rotate failed: %v", err)
			}
		}
	}
}

// //
func (r *FileRotator) Write(ctx context.Context, p *RequestPayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if r.currentSize >= r.maxSizeBytes {
		if err := r.rotateLocked(); err != nil {
			return err
		}
	}

	err := WritePayload(r.currentEnc, p)
	if err != nil {
		log.Println("Write payload failed:", err)
		return err
	}

	info, err := r.currentFile.Stat()
	if err == nil {
		r.currentSize = info.Size()
	}

	return nil
}

func (r *FileRotator) rotateLocked() error {
	if r.currentFile != nil {
		_ = r.currentFile.Close()
		r.currentEnc = nil
	}

	readyPath := filepath.Join(r.dir, fmt.Sprintf("chunk_%d.ready", time.Now().UnixNano()))
	if err := os.Rename(r.activePath, readyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rotate rename failed: %w", err)
	}

	return r.openActiveFile()
}

func (r *FileRotator) ForceRotate() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSize == 0 {
		return nil
	}
	return r.rotateLocked()
}

func (r *FileRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentFile != nil {
		err := r.currentFile.Close()
		r.currentFile = nil
		r.currentEnc = nil
		return err
	}
	return nil
}
