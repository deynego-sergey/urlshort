package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"urlshort/internal/repository/user"
	"urlshort/internal/services/notification"
)

var (
	ErrUserAlreadyExit = errors.New("user already exists")
	ErrUserPending     = errors.New("user registration pending confirmation")
	ErrTokenExpired    = errors.New("token expired")
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type AuthService struct {
	userRepo    user.IUserRepository
	sessionRepo user.ISessionRepository
	memStorage  *user.SessionMemoryStorage
	notifier    notification.INotificationService // Наш центральный сервис уведомлений
	jwtSecret   string
}

func NewAuthService(
	ur user.IUserRepository,
	sr user.ISessionRepository,
	mem *user.SessionMemoryStorage,
	notifier notification.INotificationService, // Внедрение через DI
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
		return "", ErrUserAlreadyExit
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	rawToken := generateNumericCode(6)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	_, err = s.userRepo.CreateUser(ctx, username, string(passwordHash), tokenHashStr)
	if err != nil {
		return "", err
	}

	// Асинхронная отправка email, чтобы сеть SMTP не блокировала HTTP-ответ
	s.notifier.SendAsync(ctx, notification.Notification{
		TargetType: notification.TargetEmail,
		Recipient:  username,
		Subject:    "Подтверждение регистрации",
		Body:       fmt.Sprintf("Ваш код подтверждения: %s\nИли ссылка: http://localhost:8080/confirm?token=%s", rawToken, rawToken),
	})

	return rawToken, nil
}

func (s *AuthService) ConfirmRegistration(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	u, err := s.userRepo.GetByConfirmationToken(ctx, tokenHashStr)
	if err != nil {
		return err
	}

	return s.userRepo.ActivateUser(ctx, u.ID)
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
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

func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	hash := sha256.Sum256([]byte(rawRefreshToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	session, err := s.memStorage.Get(tokenHashStr)
	if err != nil {
		session, err = s.sessionRepo.GetByRefreshToken(ctx, tokenHashStr)
		if err != nil {
			return nil, err
		}
	}

	if session.ExpiresAt.Before(time.Now()) {
		_ = s.sessionRepo.Delete(ctx, session.ID)
		s.memStorage.Delete(tokenHashStr)
		return nil, errors.New("session expired")
	}

	_ = s.sessionRepo.Delete(ctx, session.ID)
	s.memStorage.Delete(tokenHashStr)

	return s.generateTokens(ctx, session.UserID)
}

func (s *AuthService) RequestPasswordReset(ctx context.Context, username string) (string, error) {
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return "", err
	}

	rawToken := generateNumericCode(6)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	err = s.userRepo.SetPasswordResetToken(ctx, u.ID, tokenHashStr, 15*time.Minute)
	if err != nil {
		return "", err
	}

	// Асинхронная отправка токена сброса пароля
	s.notifier.SendAsync(ctx, notification.Notification{
		TargetType: notification.TargetEmail,
		Recipient:  username,
		Subject:    "Сброс пароля",
		Body:       fmt.Sprintf("Для сброса пароля используйте код: %s", rawToken),
	})

	return rawToken, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	u, err := s.userRepo.GetByResetToken(ctx, tokenHashStr)
	if err != nil {
		return err
	}

	if u.ResetTokenExpiresAt.Before(time.Now()) {
		return ErrTokenExpired
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePasswordAndClearResetToken(ctx, u.ID, string(newHash))
}

func (s *AuthService) generateTokens(ctx context.Context, userID int64) (*TokenPair, error) {
	accessToken := "mock_access_token"
	rawRefreshToken := "mock_refresh_token"

	hash := sha256.Sum256([]byte(rawRefreshToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	_ = s.sessionRepo.CreateSession(ctx, userID, tokenHashStr, expiresAt)
	s.memStorage.Set(tokenHashStr, &user.Session{UserID: userID, ExpiresAt: expiresAt})

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
