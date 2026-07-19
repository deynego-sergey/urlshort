package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

// Интерфейс для инжекции AuthService в слой обработчиков

type AuthServiceInterface interface {
	Login(ctx context.Context, username, password string) (*auth.TokenPair, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*auth.TokenPair, error)
}

type InternalHandler struct {
	authService AuthServiceInterface
}

func NewInternalHandler(as AuthServiceInterface) *InternalHandler {
	return &InternalHandler{authService: as}
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
	h.sendSuccess(w, map[string]any{"action": action, "user_id": userID})
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
