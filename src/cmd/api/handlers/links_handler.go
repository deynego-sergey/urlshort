package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"urlshort/internal/repository/link"
)

// Декларативные DTO для входящих JSON-запросов
type CreateLinkItem struct {
	OriginalURL string `json:"original_url" validate:"required,url"`
}

type CreateBatchPayload struct {
	Items []CreateLinkItem `json:"items" validate:"required,dive,min=1,max=100"`
}

func (h *InternalHandler) handleCreateBatch(w http.ResponseWriter, r *http.Request, payload json.RawMessage) {
	// 1. Проверка авторизации через контекст
	userID, ok := r.Context().Value("userID").(int64)
	if !ok || userID <= 0 {
		h.sendError(w, http.StatusUnauthorized, "unauthorized: invalid session or missing token")
		return
	}

	// 2. Валидация входных данных
	dto, err := parseAndValidate[CreateBatchPayload](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 3. Маппинг DTO в слайс моделей репозитория link.ShortLink
	// Мы создаем структуры, которые ожидает CreateBatch вместо сырых строк
	linksInput := make([]*link.ShortLink, len(dto.Items))
	for i, item := range dto.Items {
		linksInput[i] = &link.ShortLink{
			UserID:      userID,
			OriginalURL: strings.TrimSpace(item.OriginalURL),
		}
	}

	// Вызов репозитория с корректным типом []*ShortLink
	createdLinks, err := h.linkRepo.CreateBatch(r.Context(), linksInput)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "failed to create links batch")
		return
	}

	h.sendSuccess(w, map[string]any{"links": createdLinks})
}

func (h *InternalHandler) handleList(w http.ResponseWriter, r *http.Request, payload json.RawMessage) {
	userID, ok := r.Context().Value("userID").(int64)
	if !ok || userID <= 0 {
		h.sendError(w, http.StatusUnauthorized, "unauthorized: invalid session")
		return
	}

	dto, err := parseAndValidate[ListLinksPayload](h, payload)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Готовим указатель на строку для фильтра репозитория
	var searchPtr *string
	if dto.Search != nil {
		cleaned := strings.TrimSpace(*dto.Search)
		searchPtr = &cleaned
	}

	// Упаковка параметров в объект фильтра LinkFilter с передачей указателя *string
	filter := link.LinkFilter{
		UserID:    userID,
		Search:    searchPtr, // Теперь типы (*string) идеально совпадают
		IsDeleted: dto.IsDeleted,
	}

	// Вызов репозитория с передачей фильтра
	links, err := h.linkRepo.List(r.Context(), filter)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "failed to retrieve links")
		return
	}

	h.sendSuccess(w, map[string]any{"links": links})
}
