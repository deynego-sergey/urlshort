// src/internal/repository/link/repository.go
package link

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Стандартизированные ошибки для слоя API бизнес-логики
var (
	ErrLinkNotFound = errors.New("link not found")
	ErrAccessDenied = errors.New("access denied or link already deleted")
)

type ShortLink struct {
	ID          int64     `json:"id"`
	OriginalURL string    `json:"original_url"`
	UserID      int64     `json:"user_id"`
	IsDeleted   bool      `json:"is_deleted"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LinkFilter struct {
	UserID    int64   `json:"user_id"`
	Search    *string `json:"search"`
	IsDeleted *bool   `json:"is_deleted"`
}

type ILinkRepository interface {
	IInitDatabaseRepository
	IRedirectorLinksRepository
	IManegeLinksRepository
	IBatchManageLinksRepository
}

type IInitDatabaseRepository interface {
	CreateTable(ctx context.Context) error
}

type IRedirectorLinksRepository interface {
	GetLinkByID(ctx context.Context, id int64) (*ShortLink, error)
}

type IManegeLinksRepository interface {
	GetByUeserID(ctx context.Context, id int64, userID int64) (*ShortLink, error)
	Create(ctx context.Context, originalURL string, userID int64) (*ShortLink, error)
	UpdateURL(ctx context.Context, id int64, userID int64, newURL string) error
	SoftDelete(ctx context.Context, id int64, userID int64) error
}

type IBatchManageLinksRepository interface {
	List(ctx context.Context, filter LinkFilter) ([]*ShortLink, error)
	CreateBatch(ctx context.Context, links []*ShortLink) ([]*ShortLink, error)
	UpdateBatch(ctx context.Context, ids []int64, userID int64, newURL string) error
	SoftDeleteBatch(ctx context.Context, ids []int64, userID int64) error
}

type repository struct {
	*pgxpool.Pool // ИСПРАВЛЕНО: Встраивание структуры (Embedding) для оптимизации и чистоты вызовов
	builder       squirrel.StatementBuilderType
}

func NewLinkRepository(pool *pgxpool.Pool) ILinkRepository {

	return &repository{
		Pool:    pool,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

func (r *repository) CreateTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS public.short_links (
			id BIGSERIAL PRIMARY KEY,
			original_url TEXT NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			is_deleted BOOLEAN DEFAULT FALSE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT TIMEZONE('utc'::text, NOW()) NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT TIMEZONE('utc'::text, NOW()) NOT NULL
		);
		
		CREATE INDEX IF NOT EXISTS idx_short_links_id_active 
		ON public.short_links(id) 
		WHERE is_deleted = FALSE;

		CREATE INDEX IF NOT EXISTS idx_short_links_user_id 
		ON public.short_links(user_id) 
		WHERE is_deleted = FALSE;
	`

	// Прямой вызов благодаря embedding
	_, err := r.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to initialize short_links table: %w", err)
	}
	return nil
}

func (r *repository) GetLinkByID(ctx context.Context, id int64) (*ShortLink, error) {
	sqlStr, args, err := r.builder.Select("id", "original_url", "user_id", "is_deleted", "created_at", "updated_at").
		From("short_links").
		Where(squirrel.Eq{"id": id, "is_deleted": false}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var link ShortLink
	// Прямой вызов QueryRow благодаря embedding
	err = r.QueryRow(ctx, sqlStr, args...).Scan(
		&link.ID,
		&link.OriginalURL,
		&link.UserID,
		&link.IsDeleted,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("link %d: %w", id, ErrLinkNotFound)
		}
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return &link, nil
}

func (r *repository) GetByUeserID(ctx context.Context, id int64, userID int64) (*ShortLink, error) {
	sqlStr, args, err := r.builder.Select("id", "original_url", "user_id", "is_deleted", "created_at", "updated_at").
		From("short_links").
		Where(squirrel.Eq{"id": id, "user_id": userID, "is_deleted": false}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var link ShortLink
	err = r.QueryRow(ctx, sqlStr, args...).Scan(
		&link.ID,
		&link.OriginalURL,
		&link.UserID,
		&link.IsDeleted,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("link %d: %w", id, ErrAccessDenied)
		}
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return &link, nil
}

func (r *repository) Create(ctx context.Context, originalURL string, userID int64) (*ShortLink, error) {
	query, args, err := r.builder.
		Insert("short_links").
		Columns("original_url", "user_id").
		Values(originalURL, userID).
		Suffix("RETURNING id, original_url, user_id, is_deleted, created_at, updated_at").
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("failed to build insert query: %w", err)
	}

	var link ShortLink
	err = r.QueryRow(ctx, query, args...).Scan(
		&link.ID,
		&link.OriginalURL,
		&link.UserID,
		&link.IsDeleted,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute insert: %w", err)
	}

	return &link, nil
}

func (r *repository) UpdateURL(ctx context.Context, id int64, userID int64, newURL string) error {
	query, args, err := r.builder.
		Update("short_links").
		Set("original_url", newURL).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{"id": id, "user_id": userID, "is_deleted": false}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	cmdTag, err := r.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("link %d: %w", id, ErrAccessDenied)
	}

	return nil
}

func (r *repository) SoftDelete(ctx context.Context, id int64, userID int64) error {
	query, args, err := r.builder.
		Update("short_links").
		Set("is_deleted", true).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{"id": id, "user_id": userID, "is_deleted": false}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build soft delete query: %w", err)
	}

	cmdTag, err := r.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute soft delete: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("link %d: %w", id, ErrAccessDenied)
	}

	return nil
}

func (r *repository) List(ctx context.Context, filter LinkFilter) ([]*ShortLink, error) {
	queryBuilder := r.builder.Select("id", "original_url", "user_id", "is_deleted", "created_at", "updated_at").
		From("short_links").
		Where(squirrel.Eq{"user_id": filter.UserID})

	if filter.IsDeleted != nil {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"is_deleted": *filter.IsDeleted})
	} else {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"is_deleted": false})
	}

	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		searchTerm := fmt.Sprintf("%%%s%%", *filter.Search)
		queryBuilder = queryBuilder.Where(squirrel.Like{"original_url": searchTerm})
	}

	queryBuilder = queryBuilder.OrderBy("created_at DESC")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build list query: %w", err)
	}

	rows, err := r.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []*ShortLink
	for rows.Next() {
		var link ShortLink
		err := rows.Scan(
			&link.ID,
			&link.OriginalURL,
			&link.UserID,
			&link.IsDeleted,
			&link.CreatedAt,
			&link.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		links = append(links, &link)
	}

	return links, nil
}

func (r *repository) CreateBatch(ctx context.Context, links []*ShortLink) ([]*ShortLink, error) {
	if len(links) == 0 {
		return nil, nil
	}

	queryBuilder := r.builder.Insert("short_links").Columns("original_url", "user_id")

	for _, link := range links {
		queryBuilder = queryBuilder.Values(link.OriginalURL, link.UserID)
	}

	query, args, err := queryBuilder.Suffix("RETURNING id, original_url, user_id, is_deleted, created_at, updated_at").ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build batch insert query: %w", err)
	}

	rows, err := r.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute batch insert: %w", err)
	}
	defer rows.Close()

	var result []*ShortLink
	for rows.Next() {
		var link ShortLink
		err := rows.Scan(
			&link.ID,
			&link.OriginalURL,
			&link.UserID,
			&link.IsDeleted,
			&link.CreatedAt,
			&link.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan batch row: %w", err)
		}
		result = append(result, &link)
	}

	return result, nil
}

func (r *repository) UpdateBatch(ctx context.Context, ids []int64, userID int64, newURL string) error {
	if len(ids) == 0 {
		return nil
	}

	query, args, err := r.builder.Update("short_links").
		Set("original_url", newURL).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{"id": ids, "user_id": userID, "is_deleted": false}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build batch update query: %w", err)
	}

	_, err = r.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch update: %w", err)
	}

	return nil
}

func (r *repository) SoftDeleteBatch(ctx context.Context, ids []int64, userID int64) error {
	if len(ids) == 0 {
		return nil
	}

	query, args, err := r.builder.Update("short_links").
		Set("is_deleted", true).
		Set("updated_at", time.Now()).
		Where(squirrel.Eq{"id": ids, "user_id": userID, "is_deleted": false}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build batch delete query: %w", err)
	}

	_, err = r.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute batch delete: %w", err)
	}

	return nil
}
