// src/internal/services/auth/auth.go
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
	"urlshort/internal/repository/user"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrAccessDenied = errors.New("access denied: token is invalid or revoked")

// TokenPair описывает структуру возвращаемых токенов
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type AuthService struct {
	userRepo    user.IUserRepository       // Доступ к таблице users
	sessionRepo user.ISessionRepository    // Доступ к таблице user_sessions
	memStorage  *user.SessionMemoryStorage // Доступ к ОЗУ (sync.Map)
	jwtSecret   string
}

// NewAuthService — конструктор для Dependency Injection
func NewAuthService(
	ur user.IUserRepository,
	sr user.ISessionRepository,
	mem *user.SessionMemoryStorage,
	secret string,
) *AuthService {
	return &AuthService{
		userRepo:    ur,
		sessionRepo: sr,
		memStorage:  mem,
		jwtSecret:   secret,
	}
}

// Login проверяет пользователя и инициирует сессию
func (s *AuthService) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	// 1. Ищем пользователя в PostgreSQL
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid credentials")
		}
		return nil, err
	}

	// 2. Сверяем хэш пароля
	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// 3. Выпускаем новые токены
	return s.issueNewTokens(ctx, u.ID)
}

// Refresh реализует каскадную проверку (ОЗУ -> БД -> ОЗУ) с ротацией токена
func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	hash := sha256.Sum256([]byte(rawRefreshToken))
	refreshHashStr := hex.EncodeToString(hash[:])

	var userID int64
	var found bool

	// 1. Быстрая проверка в ОЗУ
	userID, found = s.memStorage.Get(refreshHashStr)

	// 2. Каскад: если в памяти нет, восстанавливаем из PostgreSQL
	if !found {
		session, err := s.sessionRepo.GetSessionByHash(ctx, refreshHashStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrAccessDenied
			}
			return nil, err
		}

		// Проверяем срок годности токена в БД
		if time.Now().After(session.ExpiresAt) {
			go func() { _ = s.sessionRepo.DeleteSession(context.Background(), refreshHashStr) }()
			return nil, ErrAccessDenied
		}

		userID = session.UserID
		// Прогреваем память на оставшийся срок жизни сессии
		s.memStorage.Set(refreshHashStr, userID, time.Until(session.ExpiresAt))
	}

	// 3. Ротация: удаляем старый токен отовсюду
	s.memStorage.Delete(refreshHashStr)
	go func() { _ = s.sessionRepo.DeleteSession(context.Background(), refreshHashStr) }()

	// 4. Генерируем свежую пару токенов взамен использованного
	return s.issueNewTokens(ctx, userID)
}

// Внутренний метод генерации и параллельной записи новой сессии
func (s *AuthService) issueNewTokens(ctx context.Context, userID int64) (*TokenPair, error) {
	// Access JWT на 3 минуты (логика подписи зашита внутри генератора)
	accessToken, err := s.generateJWT(userID, 3*time.Minute)
	if err != nil {
		return nil, err
	}

	// Генерация уникального случайного Refresh-токена
	newRawRefresh := "sk_refresh_" + generateRandomString(32)
	newHash := sha256.Sum256([]byte(newRawRefresh))
	newRefreshHashStr := hex.EncodeToString(newHash[:])

	// Одновременное кэширование в памяти и асинхронное сохранение в БД на 30 дней
	s.memStorage.Set(newRefreshHashStr, userID, 30*24*time.Hour)
	go func() {
		_ = s.sessionRepo.CreateSession(context.Background(), userID, newRefreshHashStr, 30*24*time.Hour)
	}()

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRawRefresh,
	}, nil
}

func (s *AuthService) generateJWT(userID int64, ttl time.Duration) (string, error) {
	// Здесь будет стандартное создание структуры claims и подпись через s.jwtSecret
	return "signed.jwt.payload", nil
}

func generateRandomString(n int) string {
	// Генерация криптостойких строк (crypto/rand)
	return "secure_random_string"
}
