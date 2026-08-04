package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"urlshort/internal/repository/user"
	"urlshort/internal/services/notification"
)

var (
	ErrUserAlreadyExist = errors.New("user already exists")
	ErrUserPending      = errors.New("user registration pending confirmation")
	ErrInvalidSession   = errors.New("invalid or expired session")
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type AuthService struct {
	userRepo    user.IUserRepository
	sessionRepo user.ISessionRepository
	memStorage  *user.SessionMemoryStorage
	notifier    notification.INotificationService
	jwtSecret   string
}

func NewAuthService(
	ur user.IUserRepository,
	sr user.ISessionRepository,
	mem *user.SessionMemoryStorage,
	notifier notification.INotificationService,
	secret string,
) *AuthService {
	return &AuthService{
		userRepo:    ur,
		sessionRepo: sr,
		memStorage:  mem,
		notifier:    notifier,
		jwtSecret:   secret,
	}
}

func (s *AuthService) Register(ctx context.Context, username, password string) (string, error) {
	_, err := s.userRepo.GetByUsername(ctx, username)
	if err == nil {
		return "", ErrUserAlreadyExist
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		log.Println(err)
		return "", err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Println(err)
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	rawToken := generateNumericCode(6)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	// Корректно принимаем (int64, error)
	_, err = s.userRepo.CreateUser(ctx, username, string(passwordHash), tokenHashStr)
	if err != nil {
		log.Println(err)
		return "", err
	}

	s.notifier.SendAsync(ctx, notification.Notification{
		TargetType: notification.TargetEmail,
		Recipient:  username,
		Subject:    "Подтверждение регистрации",
		Body:       fmt.Sprintf("Ваш код подтверждения: %s", rawToken),
	})

	return rawToken, nil
}

func (s *AuthService) ConfirmRegistration(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	// 1. Сначала находим пользователя по хэшу подтверждения
	u, err := s.userRepo.GetByConfirmationHash(ctx, tokenHashStr)
	if err != nil {
		return err
	}

	// 2. Активируем пользователя строго по его числовиму ID
	return s.userRepo.ActivateUser(ctx, u.ID)
}

// Login -
func (s *AuthService) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	log.Println("AuthService.Login")
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	if u.Status == "pending" {
		return nil, ErrUserPending
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid password")
	}

	return s.generateTokens(ctx, u.ID)
}

// Refresh -
func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	hash := sha256.Sum256([]byte(rawRefreshToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	var userID int64

	// 1. Ищем сессию в in-memory кэше
	id, exists := s.memStorage.Get(tokenHashStr)
	if exists {
		userID = id
	} else {
		// 2. Если в кэше нет, идем в базу через GetSessionByHash
		session, err := s.sessionRepo.GetSessionByHash(ctx, tokenHashStr)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSession, err)
		}
		userID = session.UserID
	}

	// 3. Удаляем старую сессию
	err := s.sessionRepo.DeleteSession(ctx, tokenHashStr)
	if err != nil {
		return nil, err
	}
	s.memStorage.Delete(tokenHashStr)

	// 4. Генерируем новую пару токенов
	return s.generateTokens(ctx, userID)
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, username string) (string, error) {
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return "", err
	}

	rawToken := generateNumericCode(6)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	// Передаем u.ID, хэш и time.Duration
	err = s.userRepo.SetPasswordResetToken(ctx, u.ID, tokenHashStr, 15*time.Minute)
	if err != nil {
		return "", err
	}

	s.notifier.SendAsync(ctx, notification.Notification{
		TargetType: notification.TargetEmail,
		Recipient:  username,
		Subject:    "Сброс пароля",
		Body:       fmt.Sprintf("Код для сброса пароля: %s", rawToken),
	})

	return rawToken, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	// 1. Ищем пользователя по хэшу токена сброса
	u, err := s.userRepo.GetByResetPasswordHash(ctx, tokenHashStr)
	if err != nil {
		return err
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// 2. Обновляем пароль строго по числовому ID пользователя
	return s.userRepo.ResetPassword(ctx, u.ID, string(newHash))
}

// generateTokens -
func (s *AuthService) generateTokens(ctx context.Context, userID int64) (*TokenPair, error) {
	// 1. Генерация настоящих JWT Access Token
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// 2. Генерация криптографически стойкого случайного Refresh Token
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	rawRefreshToken := hex.EncodeToString(randomBytes)

	// Хэширование Refresh токена для сохранения в БД
	hash := sha256.Sum256([]byte(rawRefreshToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	sessionDuration := 30 * 24 * time.Hour

	// Создаем сессию в БД
	err = s.sessionRepo.CreateSession(ctx, userID, tokenHashStr, sessionDuration)
	if err != nil {
		return nil, err
	}

	// Синхронизируем кэш в памяти
	s.memStorage.Set(tokenHashStr, userID, sessionDuration)

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
	}, nil
}

func generateNumericCode(length int) string {
	const digits = "0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "123456"
	}
	for i := 0; i < length; i++ {
		b[i] = digits[b[i]%10]
	}
	return string(b)
}
