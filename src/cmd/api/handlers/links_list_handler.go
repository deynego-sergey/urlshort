package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"urlshort/internal/repository/link"
)

// Интерфейс репозитория для чтения списка ссылок (DI)
type ListLinksRepository interface {
	List(ctx context.Context, filter link.LinkFilter) ([]*link.ShortLink, error)
}

type ListLinksHandler struct {
	repo ListLinksRepository
}

func NewListLinksHandler(repo ListLinksRepository) *ListLinksHandler {
	return &ListLinksHandler{repo: repo}
}

// Входной Payload для экшена link:list
type ListLinksPayload struct {
	Search    *string `json:"search,omitempty"`
	IsDeleted *bool   `json:"is_deleted,omitempty"`
}

// Ответ со списком ссылок
type ListLinksResponse struct {
	Links []*link.ShortLink `json:"links"`
}

func (h *ListLinksHandler) Execute(ctx context.Context, userID int64, payload json.RawMessage) (any, error) {
	var req ListLinksPayload

	// Payload может быть пустым, если фильтры не переданы
	if len(payload) > 0 && string(payload) != "null" {
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("failed to parse payload: %w", errors.New("bad_request"))
		}
	}

	// Вызов слоя данных с маппингом фильтров
	links, err := h.repo.List(ctx, link.LinkFilter{
		UserID:    userID,
		Search:    req.Search,
		IsDeleted: req.IsDeleted,
	})
	if err != nil {
		// Безопасный текст ошибки наружу
		return nil, fmt.Errorf("failed to fetch links from storage: %w", err)
	}

	return ListLinksResponse{Links: links}, nil
}
