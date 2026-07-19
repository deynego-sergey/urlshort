package user

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserSession struct {
	ID          int64
	UserID      int64
	RefreshHash string
	ExpiresAt   time.Time
}

type ISessionRepository interface {
	CreateTable(ctx context.Context) error
	CreateSession(ctx context.Context, userID int64, refreshHash string, duration time.Duration) error
	GetSessionByHash(ctx context.Context, refreshHash string) (*UserSession, error)
	DeleteSession(ctx context.Context, refreshHash string) error
}

type sessionRepository struct {
	pool    *pgxpool.Pool
	builder squirrel.StatementBuilderType
}

func NewSessionRepository(pool *pgxpool.Pool) ISessionRepository {
	return &sessionRepository{
		pool:    pool,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

func (r *sessionRepository) CreateTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS public.user_sessions (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
			refresh_hash VARCHAR(64) NOT NULL UNIQUE,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT TIMEZONE('utc'::text, NOW()) NOT NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_sessions_hash ON public.user_sessions(refresh_hash);
	`

	_, err := r.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to initialize user_sessions table: %w", err)
	}
	return nil
}

func (r *sessionRepository) CreateSession(ctx context.Context, userID int64, refreshHash string, duration time.Duration) error {
	sqlStr, args, err := r.builder.Insert("user_sessions").
		Columns("user_id", "refresh_hash", "expires_at").
		Values(userID, refreshHash, time.Now().Add(duration)).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}

func (r *sessionRepository) GetSessionByHash(ctx context.Context, refreshHash string) (*UserSession, error) {
	sqlStr, args, err := r.builder.Select("id", "user_id", "refresh_hash", "expires_at").
		From("user_sessions").
		Where(squirrel.Eq{"refresh_hash": refreshHash}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var s UserSession
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&s.ID, &s.UserID, &s.RefreshHash, &s.ExpiresAt)
	return &s, err
}

func (r *sessionRepository) DeleteSession(ctx context.Context, refreshHash string) error {
	sqlStr, args, err := r.builder.Delete("user_sessions").
		Where(squirrel.Eq{"refresh_hash": refreshHash}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}
