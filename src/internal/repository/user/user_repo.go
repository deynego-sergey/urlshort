package user

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserStatus string

const (
	StatusPending  UserStatus = "pending"
	StatusActive   UserStatus = "active"
	StatusRecovery UserStatus = "recovery"
)

type User struct {
	ID                int64
	Username          string
	PasswordHash      string
	Status            UserStatus
	ConfirmationHash  sql.NullString
	ResetPasswordHash sql.NullString
	ResetExpiresAt    sql.NullTime
}

type IUserRepository interface {
	CreateTable(ctx context.Context) error
	CreateUser(ctx context.Context, username, passwordHash, confirmationHash string) (int64, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByConfirmationHash(ctx context.Context, hash string) (*User, error)
	GetByResetPasswordHash(ctx context.Context, hash string) (*User, error)
	ActivateUser(ctx context.Context, id int64) error
	SetPasswordResetToken(ctx context.Context, id int64, resetHash string, ttl time.Duration) error
	ResetPassword(ctx context.Context, id int64, newPasswordHash string) error
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
			status VARCHAR(50) DEFAULT 'pending' NOT NULL,
			confirmation_hash VARCHAR(64),
			reset_password_hash VARCHAR(64),
			reset_expires_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT TIMEZONE('utc'::text, NOW()) NOT NULL
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON public.users(username);
		CREATE INDEX IF NOT EXISTS idx_users_confirmation_hash ON public.users(confirmation_hash) WHERE confirmation_hash IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_users_reset_hash ON public.users(reset_password_hash) WHERE reset_password_hash IS NOT NULL;
	`

	_, err := r.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to initialize users table: %w", err)
	}
	return nil
}

func (r *userRepository) CreateUser(ctx context.Context, username, passwordHash, confirmationHash string) (int64, error) {
	sqlStr, args, err := r.builder.Insert("users").
		Columns("username", "password_hash", "status", "confirmation_hash").
		Values(username, passwordHash, string(StatusPending), confirmationHash).
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
	sqlStr, args, err := r.builder.Select("id", "username", "password_hash", "status", "confirmation_hash", "reset_password_hash", "reset_expires_at").
		From("users").
		Where(squirrel.Eq{"username": username}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var u User
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Status, &u.ConfirmationHash, &u.ResetPasswordHash, &u.ResetExpiresAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) GetByConfirmationHash(ctx context.Context, hash string) (*User, error) {
	sqlStr, args, err := r.builder.Select("id", "username", "password_hash", "status").
		From("users").
		Where(squirrel.Eq{"confirmation_hash": hash, "status": string(StatusPending)}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var u User
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Status)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) GetByResetPasswordHash(ctx context.Context, hash string) (*User, error) {
	sqlStr, args, err := r.builder.Select("id", "username", "reset_expires_at").
		From("users").
		Where(squirrel.Eq{"reset_password_hash": hash}).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, err
	}

	var u User
	err = r.pool.QueryRow(ctx, sqlStr, args...).Scan(&u.ID, &u.Username, &u.ResetExpiresAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepository) ActivateUser(ctx context.Context, id int64) error {
	sqlStr, args, err := r.builder.Update("users").
		Set("status", string(StatusActive)).
		Set("confirmation_hash", nil).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}

func (r *userRepository) SetPasswordResetToken(ctx context.Context, id int64, resetHash string, ttl time.Duration) error {
	sqlStr, args, err := r.builder.Update("users").
		Set("reset_password_hash", resetHash).
		Set("reset_expires_at", time.Now().Add(ttl)).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}

func (r *userRepository) ResetPassword(ctx context.Context, id int64, newPasswordHash string) error {
	sqlStr, args, err := r.builder.Update("users").
		Set("password_hash", newPasswordHash).
		Set("reset_password_hash", nil).
		Set("reset_expires_at", nil).
		Set("status", string(StatusActive)). // На случай, если сбрасывают из другого особого статуса
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, sqlStr, args...)
	return err
}
