package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
	"urlshort/internal/repository/user"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAccessDenied    = errors.New("access denied: token is invalid or revoked")
	ErrUserPending     = errors.New("user registration is not confirmed")
	ErrTokenExpired    = errors.New("token has expired")
	ErrUserAlreadyExit = errors.New("username is already taken")
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type AuthService struct {
	userRepo    user.IUserRepository
	sessionRepo user.ISessionRepository
	memStorage  *user.SessionMemoryStorage
	jwtSecret   string
}

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

func (s *AuthService) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid credentials")
		}
		return nil, err
	}

	// Защита: не пускаем неподтвержденных пользователей
	if u.Status == user.StatusPending {
		return nil, ErrUserPending
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	return s.issueNewTokens(ctx, u.ID)
}

// Register создает пользователя в статусе pending и возвращает сырой токен подтверждения
func (s *AuthService) Register(ctx context.Context, username, password string) (string, error) {
	// Проверяем, не занят ли username
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

	rawToken := "confirm_" + generateRandomString(32)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	_, err = s.userRepo.CreateUser(ctx, username, string(passwordHash), tokenHashStr)
	if err != nil {
		return "", err
	}

	return rawToken, nil
}

// ConfirmRegistration активирует пользователя по сырому токену
func (s *AuthService) ConfirmRegistration(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	u, err := s.userRepo.GetByConfirmationHash(ctx, tokenHashStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessDenied
		}
		return err
	}

	return s.userRepo.ActivateUser(ctx, u.ID)
}

// RequestPasswordReset генерирует токен восстановления на 15 минут
func (s *AuthService) RequestPasswordReset(ctx context.Context, username string) (string, error) {
	u, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Безопасность: не палим факт наличия пользователя, возвращаем фейковый токен или пустую ошибку.
			// В нашей логике отдадим ошибку, но на хэндлере замаскируем.
			return "", errors.New("user not found")
		}
		return nil, err
	}

	rawToken := "reset_" + generateRandomString(32)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	err = s.userRepo.SetPasswordResetToken(ctx, u.ID, tokenHashStr, 15*time.Minute)
	if err != nil {
		return "", err
	}

	return rawToken, nil
}

// ResetPassword меняет пароль, если токен валиден и не просрочен
func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHashStr := hex.EncodeToString(hash[:])

	u, err := s.userRepo.GetByResetPasswordHash(ctx, tokenHashStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessDenied
		}
		return err
	}

	if !u.ResetExpiresAt.Valid || time.Now().After(u.ResetExpiresAt.Time) {
		return ErrTokenExpired
	}

	newPasswordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	return s.userRepo.ResetPassword(ctx, u.ID, string(newPasswordHash))
}

func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	hash := sha256.Sum256([]byte(rawRefreshToken))
	refreshHashStr := hex.EncodeToString(hash[:])

	var userID int64
	var found bool

	userID, found = s.memStorage.Get(refreshHashStr)

	if !found {
		session, err := s.sessionRepo.GetSessionByHash(ctx, refreshHashStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrAccessDenied
			}
			return nil, err
		}

		if time.Now().After(session.ExpiresAt) {
			go func() { _ = s.sessionRepo.DeleteSession(context.Background(), refreshHashStr) }()
			return nil, ErrAccessDenied
		}

		userID = session.UserID
		s.memStorage.Set(refreshHashStr, userID, time.Until(session.ExpiresAt))
	}

	s.memStorage.Delete(refreshHashStr)
	go func() { _ = s.sessionRepo.DeleteSession(context.Background(), refreshHashStr) }()

	return s.issueNewTokens(ctx, userID)
}

func (s *AuthService) issueNewTokens(ctx context.Context, userID int64) (*TokenPair, error) {
	accessToken, err := s.generateJWT(userID, 3*time.Minute)
	if err != nil {
		return nil, err
	}

	newRawRefresh := "sk_refresh_" + generateRandomString(32)
	newHash := sha256.Sum256([]byte(newRawRefresh))
	newRefreshHashStr := hex.EncodeToString(newHash[:])

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
	return "signed.jwt.payload", nil
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "fallback_secure_string_1234567890"
	}
	return hex.EncodeToString(b)[:n]
}
