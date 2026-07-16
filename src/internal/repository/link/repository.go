package link

import (
	"context"
	"errors"
	"fmt"
	"time"
	"urlshort/pkg/database/pg"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Masterminds/squirrel"
)

type ShortLink struct {
	ID          int64     `json:"id"`
	OriginalURL string    `json:"original_url"`
	IsDeleted   bool      `json:"is_deleted"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type ILinkRepository interface {
	GetByID(ctx context.Context, id int64) (*ShortLink, error)
	Create(ctx context.Context, originalURL string) (*ShortLink, error)
	UpdateURL(ctx context.Context, id int64, newURL string) error
	SoftDelete(ctx context.Context, id int64) error
}

type repository struct {
	pool    *pgxpool.Pool
	builder squirrel.StatementBuilderType
}

// NewLinkRepository -
func NewLinkRepository(ctx context.Context) (ILinkRepository, error) {
	var pool *pgxpool.Pool
	var err error
	if pool, err = pg.InitSupabasePool(ctx); err != nil {
		return nil, err
	}
	return &repository{pool: pool,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}, nil

}

// GetByID - get target link by number ID
func (r repository) GetByID(ctx context.Context, id int64) (*ShortLink, error) {

	sql, args, err := r.builder.Select("id", "original_url", "is_deleted", "created_at", "updated_at").
		From("short_links").
		Where(squirrel.Eq{"id": id, "is_deleted": false}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var link ShortLink
	err = r.pool.QueryRow(ctx, sql, args...).Scan(
		&link.ID,
		&link.OriginalURL,
		&link.IsDeleted,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("link with ID %d not found or deleted", id)
		}
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return &link, nil
}

// Create -
func (r repository) Create(ctx context.Context, originalURL string) (*ShortLink, error) {
	query, args, err := r.builder.
		Insert("short_links").
		Columns("original_url").
		Values(originalURL).
		Suffix("RETURNING id, original_url, is_deleted, created_at, updated_at").
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("failed to build insert query: %w", err)
	}

	var link ShortLink
	err = r.pool.QueryRow(ctx, query, args...).Scan(
		&link.ID,
		&link.OriginalURL,
		&link.IsDeleted,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute insert: %w", err)
	}

	return &link, nil
}

// UpdateURL -
func (r repository) UpdateURL(ctx context.Context, id int64, newURL string) error {
	query, args, err := r.builder.
		Update("short_links").
		Set("original_url", newURL).
		Where(squirrel.Eq{"id": id, "is_deleted": false}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	cmdTag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("link not found or already deleted")
	}

	return nil
}

// SoftDelete -
func (r repository) SoftDelete(ctx context.Context, id int64) error {
	query, args, err := r.builder.
		Update("short_links").
		Set("is_deleted", true).
		Where(squirrel.Eq{"id": id, "is_deleted": false}).
		ToSql()

	if err != nil {
		return fmt.Errorf("failed to build soft delete query: %w", err)
	}

	cmdTag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute soft delete: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("link already deleted or does not exist")
	}

	return nil
}
