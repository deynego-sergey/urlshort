package user

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type IUserRepository interface {
	CreateTable(ctx context.Context) error
	CreateUser(ctx context.Context, username, passwordHash string) (int64, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
}

type userRepository struct {
	pool    *pgxpool.Pool
	builder squirrel.StatementBuilderType
}

func NewUserRepository(pool *pgxpool.Pool) IUserRepository {
	return &userRepository{
		pool:    pool,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

func (r *userRepository) CreateTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS public.users (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(255) NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT TIMEZONE('utc'::text, NOW()) NOT NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON public.users(username);
	`

	_, err := r.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to initialize users table: %w", err)
	}
	return nil
}

func (r *userRepository) CreateUser(ctx context.Context, username, passwordHash string) (int64, error) {
	sqlStr, args, err := r.builder.Insert("users").
		Columns("username", "password_hash").
		Values(username, passwordHash).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&id)
	return id, err
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	sqlStr, args, err := r.builder.Select("id", "username", "password_hash").
		From("users").
		Where(squirrel.Eq{"username": username}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var u User
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
