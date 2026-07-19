package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"urlshort/internal/repository/link"
	"urlshort/internal/services/auth"
)

const refreshCookieName = "refresh_token"

type RESTRequest struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

type RESTResponse struct {
	Status string          `json:"status"`
	Error  string          `json:"error,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type IAuthServiceInterface interface {
	Login(ctx context.Context, username, password string) (*auth.TokenPair, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*auth.TokenPair, error)
	Register(ctx context.Context, username, password string) (string, error)
	ConfirmRegistration(ctx context.Context, rawToken string) error
	RequestPasswordReset(ctx context.Context, username string) (string, error)
	ResetPassword(ctx context.Context, rawToken, newPassword string) error
}

type InternalHandler struct {
	authService IAuthServiceInterface
	linkRepo    link.ILinkRepository
}

func NewInternalHandler(as IAuthServiceInterface, lr link.ILinkRepository) *InternalHandler {
	return &InternalHandler{
		authService: as,
		linkRepo:    lr,
	}
}

func (h *InternalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "only POST allowed")
		return
	}

	var req RESTRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid json envelope")
		return
	}

	switch req.Action {
	case "auth:login":
		h.handleLogin(w, r.Context(), req.Payload)
	case "auth:refresh":
		h.handleRefresh(w, r)
	case "auth:register":
		h.handleRegister(w, r.Context(), req.Payload)
	case "auth:confirm":
		h.handleConfirm(w, r.Context(), req.Payload)
	case "auth:request_reset":
		h.handleRequestReset(w, r.Context(), req.Payload)
	case "auth:reset_password":
		h.handleResetPassword(w, r.Context(), req.Payload)
	case "link:list", "link:create_batch", "link:update_batch", "link:delete_batch",
		"analytics:summary", "analytics:link_details", "analytics:top_links":

		userID, ok := r.Context().Value("userID").(int64)
		if !ok || userID == 0 {
			h.sendError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h.routeProtectedAction(w, r.Context(), userID, req.Action, req.Payload)
	default:
		h.sendError(w, http.StatusBadRequest, "unknown action")
	}
}

func (h *InternalHandler) handleLogin(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	var dto struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	tokenPair, err := h.authService.Login(ctx, dto.Username, dto.Password)
	if err != nil {
		h.sendError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.setRefreshCookie(w, tokenPair.RefreshToken)
	h.sendSuccess(w, map[string]string{"access_token": tokenPair.AccessToken})
}

func (h *InternalHandler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		h.sendError(w, http.StatusUnauthorized, "missing refresh cookie")
		return
	}

	tokenPair, err := h.authService.Refresh(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, auth.ErrAccessDenied) {
			h.clearRefreshCookie(w)
			h.sendError(w, http.StatusUnauthorized, err.Error())
			return
		}
		h.sendError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.setRefreshCookie(w, tokenPair.RefreshToken)
	h.sendSuccess(w, map[string]string{"access_token": tokenPair.AccessToken})
}

func (h *InternalHandler) routeProtectedAction(w http.ResponseWriter, ctx context.Context, userID int64, action string, payload json.RawMessage) {
	var (
		res any
		err error
	)

	switch action {
	case "link:create_batch":
		handler := NewCreateBatchHandler(h.linkRepo)
		res, err = handler.Execute(ctx, userID, payload)

	case "link:list":
		handler := NewListLinksHandler(h.linkRepo)
		res, err = handler.Execute(ctx, userID, payload)

	// Будущие экшены добавятся сюда без изменения общей логики роутера:
	// case "link:update_batch":
	// case "link:delete_batch":

	default:
		h.sendError(w, http.StatusBadRequest, "unknown protected action")
		return
	}

	if err != nil {
		// Если ошибка содержит маркер bad_request (из h.repo или валидации), отдаем 400
		if errors.Is(err, errors.New("bad_request")) || fmt.Sprintf("%v", err) == "bad_request" {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.sendSuccess(w, res)
}

func (h *InternalHandler) setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/v1/internal",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *InternalHandler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/v1/internal",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *InternalHandler) sendError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(RESTResponse{Status: "error", Error: msg})
}

func (h *InternalHandler) sendSuccess(w http.ResponseWriter, data any) {
	raw, _ := json.Marshal(data)
	_ = json.NewEncoder(w).Encode(RESTResponse{Status: "success", Data: raw})
}
func (h *InternalHandler) handleRegister(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	var dto struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if dto.Username == "" || len(dto.Password) < 6 {
		h.sendError(w, http.StatusBadRequest, "bad_request: username empty or password too short")
		return
	}

	confirmToken, err := h.authService.Register(ctx, dto.Username, dto.Password)
	if err != nil {
		if errors.Is(err, auth.ErrUserAlreadyExit) {
			h.sendError(w, http.StatusConflict, "username already taken")
			return
		}
		h.sendError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Имитируем отправку токена (в продакшене тут будет вызов очереди/сервиса уведомлений)
	log.Printf("[TEST ONLY] Confirmation token for %s: %s", dto.Username, confirmToken)

	h.sendSuccess(w, map[string]string{"message": "registration initiated, confirmation required"})
}

func (h *InternalHandler) handleConfirm(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	var dto struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if err := h.authService.ConfirmRegistration(ctx, dto.Token); err != nil {
		h.sendError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	h.sendSuccess(w, map[string]string{"message": "registration successfully confirmed"})
}

func (h *InternalHandler) handleRequestReset(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	var dto struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	resetToken, err := h.authService.RequestPasswordReset(ctx, dto.Username)
	// Маскируем ошибку отсутствия пользователя для защиты от перебора данных (Безопасность)
	if err == nil {
		log.Printf("[TEST ONLY] Reset token for %s: %s", dto.Username, resetToken)
	}

	h.sendSuccess(w, map[string]string{"message": "if user exists, instructions will be provided"})
}

func (h *InternalHandler) handleResetPassword(w http.ResponseWriter, ctx context.Context, payload json.RawMessage) {
	var dto struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if dto.Token == "" || len(dto.NewPassword) < 6 {
		h.sendError(w, http.StatusBadRequest, "bad_request: data invalid")
		return
	}

	if err := h.authService.ResetPassword(ctx, dto.Token, dto.NewPassword); err != nil {
		if errors.Is(err, auth.ErrTokenExpired) {
			h.sendError(w, http.StatusBadRequest, "token expired")
			return
		}
		h.sendError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	h.sendSuccess(w, map[string]string{"message": "password has been updated successfully"})
}
