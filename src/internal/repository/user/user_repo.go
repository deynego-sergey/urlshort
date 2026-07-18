package user

import (
	"context"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type IUserRepository interface {
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
		return nil, err // Вернет pgx.ErrNoRows, если не найден
	}
	return &u, nil
}
