package user

import (
	"sync"
	"time"
)

// MemorySession описывает структуру сессии в оперативной памяти
type MemorySession struct {
	UserID    int64
	ExpiresAt time.Time
}

// SessionMemoryStorage — ин-мемори хранилище на базе sync.Map
type SessionMemoryStorage struct {
	storage sync.Map
}

// NewSessionMemoryStorage — тот самый конструктор, который мы вызываем в main.go
func NewSessionMemoryStorage() *SessionMemoryStorage {
	s := &SessionMemoryStorage{}
	// Фоновая очистка мертвых токенов раз в 5 минут
	go s.startGC(5 * time.Minute)
	return s
}

func (s *SessionMemoryStorage) Set(refreshHash string, userID int64, ttl time.Duration) {
	s.storage.Store(refreshHash, MemorySession{
		UserID:    userID,
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (s *SessionMemoryStorage) Get(refreshHash string) (int64, bool) {
	val, ok := s.storage.Load(refreshHash)
	if !ok {
		return 0, false
	}

	session := val.(MemorySession)
	if time.Now().After(session.ExpiresAt) {
		s.storage.Delete(refreshHash)
		return 0, false
	}

	return session.UserID, true
}

func (s *SessionMemoryStorage) Delete(refreshHash string) {
	s.storage.Delete(refreshHash)
}

func (s *SessionMemoryStorage) startGC(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		now := time.Now()
		s.storage.Range(func(key, value interface{}) bool {
			session := value.(MemorySession)
			if now.After(session.ExpiresAt) {
				s.storage.Delete(key)
			}
			return true
		})
	}
}
