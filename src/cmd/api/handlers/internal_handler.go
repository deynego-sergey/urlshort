// src/cmd/api/handlers/internal_handler.go
package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	middleware "urlshort/pkg/middleware/jwtauth"
	"urlshort/pkg/utils"

	"urlshort/internal/repository/link"
	"urlshort/internal/services/auth"
)

// RequestRouter определяет структуру входящего JSON-запроса через WebSocket/HTTP
type RequestRouter struct {
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data"`
}

// Структуры данных (Payloads) для авторизации
type RegisterPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ConfirmPayload struct {
	Token string `json:"token"`
}

type LoginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RefreshPayload struct {
	RefreshToken string `json:"refresh_token"`
}

type ResetRequestPayload struct {
	Username string `json:"username"`
}

type ResetPasswordPayload struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// Структуры данных (Payloads) для управления ссылками
type CreateLinkPayload struct {
	OriginalURL string `json:"original_url"`
}

type DeleteLinkPayload struct {
	ID string `json:"id"`
}

type CreateLinkItem struct {
	OriginalURL string `json:"original_url"`
}

type CreateBatchPayload struct {
	Items []CreateLinkItem `json:"items"`
}

type ListLinksPayload struct {
	Search    *string `json:"search"`
	IsDeleted *bool   `json:"is_deleted"`
}

// InternalHandler объединяет все сервисы и репозитории для маршрутизации JSON-команд
type InternalHandler struct {
	authService *auth.AuthService
	linkRepo    link.ILinkRepository
	converter   utils.Converter
}

func NewInternalHandler(as *auth.AuthService, lr link.ILinkRepository, cnv *utils.Converter) *InternalHandler {
	return &InternalHandler{
		authService: as,
		linkRepo:    lr,
		converter:   *cnv,
	}
}

// ServeHTTP реализует обработку HTTP-запросов, дублируя логику маршрутизации WebSocket
func (h *InternalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	//w.Header().Set("Access-Control-Allow-Origin", "*")
	//w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	//w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	//	if r.Method == http.MethodOptions {
	//		w.WriteHeader(http.StatusOK)
	//		return
	//	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var router RequestRouter
	if err := json.NewDecoder(r.Body).Decode(&router); err != nil {
		http.Error(w, "Invalid JSON envelope", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch router.Action {
	// Маршруты авторизации
	case "auth.register":
		h.handleRegister(w, r, router.Data)
	case "auth.confirm":
		h.handleConfirm(w, r, router.Data)
	case "auth.login":
		h.handleLogin(w, r, router.Data)
	case "auth.refresh":
		h.handleRefresh(w, r, router.Data)
	case "auth.request_reset":
		h.handleRequestReset(w, r, router.Data)
	case "auth.reset_password":
		h.handleResetPassword(w, r, router.Data)

	// Маршруты управления ссылками
	// create short link
	case "link.create":
		h.handleCreateLink(w, r, router.Data)
	// deactivate link
	case "link.delete":
		h.handleDeleteLink(w, r, router.Data)
	//batch links
	case "link.create_batch":
		h.handleCreateBatch(w, r, router.Data)
	// list links
	case "link.list":
		h.handleList(w, r, router.Data)
	//update link
	case "link.link":
		h.handleUpdate(w, r, router.Data)
	// statistic by short link
	case "stat.short":
		h.handleLinkStat(w, r, router.Data)
	// statistic for link - full info
	case "stat.full":
		h.handleFullLinkStat(w, r, router.Data)
	// statistic for userstate total
	case "stat.user":
		h.handleUserState(w, r, router.Data)

	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unknown action"}`))
	}
}

// === ОБРАБОТЧИКИ АВТОРИЗАЦИИ ===
// handleRegister -
func (h *InternalHandler) handleRegister(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload RegisterPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.Username == "" || payload.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	code, err := h.authService.Register(r.Context(), payload.Username, payload.Password)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		if errors.Is(err, auth.ErrUserAlreadyExist) {
			_, _ = w.Write([]byte(`{"error":"user already exists"}`))
			return
		}
		log.Printf("[ERROR] Register failed: %v", err)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","message":"confirmation code sent","code":"` + code + `"}`))
}

// handleConfirm
func (h *InternalHandler) handleConfirm(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload ConfirmPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	err := h.authService.ConfirmRegistration(r.Context(), payload.Token)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("[ERROR] Confirmation failed: %v", err)
		_, _ = w.Write([]byte(`{"error":"invalid or expired confirmation token"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","message":"account activated"}`))
}

// handleLogin
func (h *InternalHandler) handleLogin(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload LoginPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.Username == "" || payload.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	tokens, err := h.authService.Login(r.Context(), payload.Username, payload.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		if errors.Is(err, auth.ErrUserPending) {
			_, _ = w.Write([]byte(`{"error":"user registration pending confirmation"}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
		return
	}

	resp, _ := json.Marshal(tokens)
	w.WriteHeader(http.StatusOK)
	setRefreshTokenCookie(w, tokens.RefreshToken, 60*60*24*3)

	_, _ = w.Write(resp)
}

// handleRefresh -
func (h *InternalHandler) handleRefresh(w http.ResponseWriter, r *http.Request, data json.RawMessage) {

	rft, err := extractRefreshToken(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"failed to get refresh token"}`))
		return
	}

	actx, ok := getAuthContext(r)
	if !ok || !actx.IsValid || actx.UserID <= 0 {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized: invalid session"}`))
		return // ОБЯЗАТЕЛЬНО
	}

	// Пример логики обновления токенов/данных
	tokens, err := h.authService.Refresh(r.Context(), rft)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to refresh token"}`))
		return // ОБЯЗАТЕЛЬНО
	}

	resp, err := json.Marshal(tokens)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to marshal response"}`))
		return // ОБЯЗАТЕЛЬНО
	}

	// Успешный ответ: WriteHeader вызывается ровно 1 раз в самом конце
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

// handleRequestReset -
func (h *InternalHandler) handleRequestReset(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload ResetRequestPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	code, err := h.authService.RequestPasswordReset(r.Context(), payload.Username)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","message":"if account exists, reset code sent"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","message":"reset code sent","code":"` + code + `"}`))
}

// handleResetPassword
func (h *InternalHandler) handleResetPassword(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload ResetPasswordPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.Token == "" || payload.NewPassword == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	err := h.authService.ResetPassword(r.Context(), payload.Token, payload.NewPassword)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("[ERROR] Reset password failed: %v", err)
		_, _ = w.Write([]byte(`{"error":"invalid or expired reset token"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","message":"password has been reset successfully"}`))
}

// === ОБРАБОТЧИКИ ССЫЛОК ===
// handleCreateLink -
func (h *InternalHandler) handleCreateLink(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload CreateLinkPayload
	actx, ok := getAuthContext(r)
	if !ok || !actx.IsValid {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// todo: check source link(original)
	if err := json.Unmarshal(data, &payload); err != nil || payload.OriginalURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	// todo: here create short code
	shortLink, err := h.linkRepo.Create(r.Context(), payload.OriginalURL, actx.UserID)
	if err != nil {
		log.Printf("[ERROR] Failed to create link: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to create short link"}`))
		return
	}

	resp, err := json.Marshal(link.ShortLinkGen{
		ID:          shortLink.ID,
		ShortLink:   h.converter.ConvertToStr(shortLink.ID),
		OriginalURL: shortLink.OriginalURL,
		UserID:      actx.UserID,
		IsDeleted:   shortLink.IsDeleted,
		CreatedAt:   shortLink.CreatedAt,
		UpdatedAt:   shortLink.UpdatedAt,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

// handleDeleteLink -
func (h *InternalHandler) handleDeleteLink(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var payload DeleteLinkPayload
	actx, ok := getAuthContext(r)
	if !ok || !actx.IsValid {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.ID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	linkID, err := strconv.ParseInt(payload.ID, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid link id format"}`))
		return
	}

	//userID, _ := r.Context().Value("userID").(int64)

	err = h.linkRepo.SoftDelete(r.Context(), linkID, actx.UserID)
	if err != nil {
		log.Printf("[ERROR] Failed to delete link: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to delete link"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","message":"link deleted"}`))
}

// handleCreateBatch -
func (h *InternalHandler) handleCreateBatch(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	actx, ok := getAuthContext(r)
	if !ok || !actx.IsValid {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if actx.UserID <= 0 {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized: invalid session or missing token"}`))
		return
	}

	var dto CreateBatchPayload
	if err := json.Unmarshal(data, &dto); err != nil || len(dto.Items) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	linksInput := make([]*link.ShortLink, len(dto.Items))
	for i, item := range dto.Items {
		linksInput[i] = &link.ShortLink{
			UserID:      actx.UserID,
			OriginalURL: strings.TrimSpace(item.OriginalURL),
		}
	}

	createdLinks, err := h.linkRepo.CreateBatch(r.Context(), linksInput)
	if err != nil {
		log.Printf("[ERROR] Failed to create batch: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to create links batch"}`))
		return
	}

	resp, err := json.Marshal(map[string]any{"links": createdLinks})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

// handleList -
func (h *InternalHandler) handleList(w http.ResponseWriter, r *http.Request, data json.RawMessage) {

	actx, ok := getAuthContext(r)
	if !ok || !actx.IsValid {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	//userID, ok := r.Context().Value("userID").(int64)
	if actx.UserID <= 0 {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized: invalid session"}`))
		return
	}

	var dto ListLinksPayload
	if err := json.Unmarshal(data, &dto); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid payload"}`))
		return
	}

	var searchPtr *string
	if dto.Search != nil {
		cleaned := strings.TrimSpace(*dto.Search)
		searchPtr = &cleaned
	}

	filter := link.LinkFilter{
		UserID:    actx.UserID,
		Search:    searchPtr,
		IsDeleted: dto.IsDeleted,
	}

	links, err := h.linkRepo.List(r.Context(), filter)
	if err != nil {
		log.Printf("[ERROR] Failed to list links: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to retrieve links"}`))
		return
	}

	sl := make([]*link.ShortLinkGen, len(links))
	for _, lnk := range links {
		sl = append(sl, &link.ShortLinkGen{
			ID:          lnk.ID,
			ShortLink:   h.converter.ConvertToStr(lnk.ID),
			OriginalURL: lnk.OriginalURL,
			UserID:      lnk.UserID,
			IsDeleted:   lnk.IsDeleted,
			CreatedAt:   lnk.CreatedAt,
			UpdatedAt:   lnk.UpdatedAt,
		})
	}
	resp, err := json.Marshal(map[string]any{"links": sl})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

type UpdateLinkRequest struct {
	ID          int64  `json:"id"`
	OriginalURL string `json:"original_url"`
}

// handleUpdate
func (h *InternalHandler) handleUpdate(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	actx, ok := getAuthContext(r)
	if !ok || !actx.IsValid || actx.UserID <= 0 {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized: invalid session"}`))
		return
	}

	var req UpdateLinkRequest
	if err := json.Unmarshal(data, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request payload"}`))
		return
	}

	if req.ID <= 0 || req.OriginalURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"id and original_url are required"}`))
		return
	}

	err := h.linkRepo.UpdateURL(r.Context(), req.ID, actx.UserID, req.OriginalURL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to update link"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))

}

func setRefreshTokenCookie(w http.ResponseWriter, refreshToken string, ttlSeconds int) {
	isProd := false
	sameSite := http.SameSiteLaxMode
	if isProd {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "l100xyz",
		Value:    refreshToken,
		Path:     "/v1", // Ограничиваем область отправки куки только ручкой обновления
		MaxAge:   ttlSeconds,
		HttpOnly: true,     // Защита от XSS (JS на клиенте не сможет прочитать cookie)
		Secure:   isProd,   // Передача только по HTTPS
		SameSite: sameSite, // Защита от CSRF
	})
}

func extractRefreshToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie("l100xyz")
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return "", errors.New("refresh token cookie missing")
		}
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("refresh token is empty")
	}
	return cookie.Value, nil
}

func clearRefreshTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "l100xyz",
		Value:    "",
		Path:     "/v1",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// Вспомогательный метод для извлечения AuthContext из r.Context()
func getAuthContext(r *http.Request) (middleware.AuthContext, bool) {
	authCtx, ok := r.Context().Value(middleware.AuthContextKey).(middleware.AuthContext)
	if !ok || !authCtx.IsValid || authCtx.UserID <= 0 {
		return middleware.AuthContext{}, false
	}
	return authCtx, true
}
