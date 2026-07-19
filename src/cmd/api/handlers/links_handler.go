// src/cmd/api/handlers/links_handler.go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"urlshort/internal/repository/link"
)

// Нам нужен интерфейс репозитория, с которым будет работать экшен.
// Используем DI для его внедрения.
type BatchCreatorRepository interface {
	CreateBatch(ctx context.Context, links []*link.ShortLink) ([]*link.ShortLink, error)
}

type CreateBatchHandler struct {
	repo BatchCreatorRepository // Внедрение зависимости (DI)
}

func NewCreateBatchHandler(repo BatchCreatorRepository) *CreateBatchHandler {
	return &CreateBatchHandler{repo: repo}
}

// Входной Payload для экшена link:create_batch
type CreateBatchPayload struct {
	URLs []string `json:"urls"`
}

// Ответ, который мы вернем в случае успеха
type CreateBatchResponse struct {
	Links []*link.ShortLink `json:"links"`
}

func (h *CreateBatchHandler) Execute(ctx context.Context, userID int64, payload json.RawMessage) (any, error) {
	var req CreateBatchPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", errors.New("bad_request"))
	}

	// Жесткая валидация входных данных (Безопасность)
	if len(req.URLs) == 0 {
		return nil, errors.New("urls array cannot be empty")
	}
	if len(req.URLs) > 100 { // Защита от перегрузки пула/базы (DDOS)
		return nil, errors.New("batch size cannot exceed 100 urls")
	}

	var linksToCreate []*link.ShortLink

	for _, rawURL := range req.URLs {
		// Валидация корректности URL перед сохранением в базу
		parsedURL, err := url.ParseRequestURI(rawURL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return nil, fmt.Errorf("invalid url format: %s", rawURL)
		}

		linksToCreate = append(linksToCreate, &link.ShortLink{
			OriginalURL: rawURL,
			UserID:      userID,
			// ID и CreatedAt/UpdatedAt назначит база данных через RETURNING
		})
	}

	// Вызов слоя данных
	createdLinks, err := h.repo.CreateBatch(ctx, linksToCreate)
	if err != nil {
		// Логируем реальную ошибку на сервере, а клиенту отдаем безопасный текст
		return nil, fmt.Errorf("failed to save batch to storage: %w", err)
	}

	return CreateBatchResponse{Links: createdLinks}, nil
}
