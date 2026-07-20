package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"urlshort/internal/repository/link"
	"urlshort/internal/services/auth"

	"github.com/go-playground/validator"
)

// AuthServiceInterface описывает контракт со слоем бизнес-логики аутентификации
type AuthServiceInterface interface {
	Login(ctx context.Context, username, password string) (*auth.TokenPair, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*auth.TokenPair, error)
	Register(ctx context.Context, username, password string) (string, error)
	ConfirmRegistration(ctx context.Context, rawToken string) error
	RequestPasswordReset(ctx context.Context, username string) (string, error)
	ResetPassword(ctx context.Context, rawToken, newPassword string) error
}

// Входящий JSON-конверт для маршрутизации
type RequestEnvelope struct {
	Action  string          `json:"action" validate:"required"`
	Payload json.RawMessage `json:"payload"`
}

// Декларативные DTO для публичных экшенов безопасности
type RegisterDTO struct {
	Username string `json:"username" validate:"required,email,max=100"`
	Password string `json:"password" validate:"required,min=6,max=100"`
}

type LoginDTO struct {
	Username string `json:"username" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type RequestResetDTO struct {
	Username string `json:"username" validate:"required,email,max=100"`
}

type ConfirmDTO struct {
	Token string `json:"token" validate:"required,min=10,max=100"`
}

type ResetPasswordDTO struct {
	Token       string `json:"token" validate:"required,min=10,max=100"`
	NewPassword string `json:"new_password" validate:"required,min=6,max=100"`
}

// InternalHandler является центральной точкой входа для WebSocket/HTTP JSON-сообщений
type InternalHandler struct {
	authService AuthServiceInterface
	linkRepo    link.ILinkRepository
	validate    *validator.Validate
}

func NewInternalHandler(as AuthServiceInterface, lr link.ILinkRepository) *InternalHandler {
	return &InternalHandler{
		authService: as,
		linkRepo:    lr,
		validate:    validator.New(),
	}
}

func (h *InternalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var env RequestEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		h.sendError(w, http.StatusBadRequest, "bad_request: invalid envelope JSON")
		return
	}

	if err := h.validate.Struct(&env); err != nil {
		h.sendError(w, http.StatusBadRequest, "bad_request: action is required")
		return
	}

	// Маршрутизация экшенов
	switch env.Action {
	case "auth:register":
		h.handleRegister(w, r.Context(), env.Payload)
	case "auth:confirm":
		h.handleConfirm(w, r.Context(), env.Payload)
	case "auth:login":
		h.handleLogin(w, r.Context(), env.Payload)
	case "auth:refresh":
		h.handleRefresh(w, r)
	case "auth:request_reset":
		h.handleRequestReset(w, r.Context(), env.Payload)
	case "auth:reset_password":
		h.handleResetPassword(w, r.Context(), env.Payload)

	// Защищенные роуты работы со ссылками (Вынесены в link_handlers.go)
	case "link:create_batch":
		h.handleCreateBatch(w, r, env.Payload)
	case "link:list":
		h.handleList(w, r, env.Payload)

	default:
		h.sendError(w, http.StatusNotFound, "unknown action")
	}
}

// Вспомогательный дженерик-метод для десериализации и валидации payload
func parseAndValidate[T any](h *InternalHandler, data json.RawMessage) (*T, error) {
	var dto T
	if len(data) == 0 {
		return nil, errors.New("bad_request: missing payload data")
	}
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, errors.New("bad_request: invalid json structure")
	}
	if err := h.validate.Struct(&dto); err != nil {
		return nil, errors.New("bad_request: validation failed")
	}
	return &dto, nil
}

// --- Реализация публичных хендлеров ---

func (h *InternalHandler) handleRegister(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	dto, err := parseAndValidate[RegisterDTO](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	username := strings.TrimSpace(dto.Username)
	confirmToken, err := h.authService.Register(ctx, username, dto.Password)
	if err != nil {
		if errors.Is(err, auth.ErrUserAlreadyExit) {
			h.sendError(w, http.StatusConflict, "username already taken")
			return
		}
		h.sendError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	log.Printf("[TEST ONLY] Confirmation token for %s: %s", username, confirmToken)
	h.sendSuccess(w, map[string]string{"message": "registration initiated, confirmation required"})
}

func (h *InternalHandler) handleConfirm(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	dto, err := parseAndValidate[ConfirmDTO](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	token := strings.TrimSpace(dto.Token)
	if err := h.authService.ConfirmRegistration(ctx, token); err != nil {
		h.sendError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	h.sendSuccess(w, map[string]string{"message": "registration successfully confirmed"})
}

func (h *InternalHandler) handleLogin(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	dto, err := parseAndValidate[LoginDTO](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	username := strings.TrimSpace(dto.Username)
	tokens, err := h.authService.Login(ctx, username, dto.Password)
	if err != nil {
		if errors.Is(err, auth.ErrUserPending) {
			h.sendError(w, http.StatusForbidden, "user registration is not confirmed")
			return
		}
		h.sendError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Установка HTTP-Only Cookie для Refresh токена в целях безопасности
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/v1/internal",
		HttpOnly: true,
		Secure:   true, // Включаем для HTTPS окружения
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})

	h.sendSuccess(w, map[string]string{"access_token": tokens.AccessToken})
}

func (h *InternalHandler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		h.sendError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	tokens, err := h.authService.Refresh(r.Context(), cookie.Value)
	if err != nil {
		h.sendError(w, http.StatusUnauthorized, "access denied: session invalid")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/v1/internal",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})

	h.sendSuccess(w, map[string]string{"access_token": tokens.AccessToken})
}

func (h *InternalHandler) handleRequestReset(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	dto, err := parseAndValidate[RequestResetDTO](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	username := strings.TrimSpace(dto.Username)
	resetToken, err := h.authService.RequestPasswordReset(ctx, username)
	if err == nil {
		log.Printf("[TEST ONLY] Reset token for %s: %s", username, resetToken)
	}

	// Маскируем ошибку для предотвращения перебора логинов (Безопасность)
	h.sendSuccess(w, map[string]string{"message": "if user exists, instructions will be provided"})
}

func (h *InternalHandler) handleResetPassword(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	dto, err := parseAndValidate[ResetPasswordDTO](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	token := strings.TrimSpace(dto.Token)
	if err := h.authService.ResetPassword(ctx, token, dto.NewPassword); err != nil {
		if errors.Is(err, auth.ErrTokenExpired) {
			h.sendError(w, http.StatusBadRequest, "token expired")
			return
		}
		h.sendError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	h.sendSuccess(w, map[string]string{"message": "password has been updated successfully"})
}

// Хелперы стандартизации ответов API
func (h *InternalHandler) sendError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

func (h *InternalHandler) sendSuccess(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": data,
	})
}
