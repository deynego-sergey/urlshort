package pg

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InitSupabasePool инициализирует пул TCP/IP соединений с Supabase
func InitSupabasePool(ctx context.Context) (*pgxpool.Pool, error) {
	// Считываем строку подключения из переменных окружения
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		return nil, fmt.Errorf("DATABASE_URL variable is not set in environment")
	}

	// Парсим конфигурацию строки подключения
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection string: %w", err)
	}

	// Настройки пула для оптимизации под лимиты Supabase (особенно на Free Tier)
	config.MaxConns = 10                      // Ограничиваем пул (у Supabase Free жесткий лимит на коннекты)
	config.MinConns = 2                       // Минимальное удерживаемое количество коннектов
	config.MaxConnIdleTime = 15 * time.Minute // Закрываем неиспользуемые соединения

	// Создаем пул соединений
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Проверяем физическое TCP-подключение быстрым пингом базы данных
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("failed to ping Supabase database: %w", err)
	}

	return pool, nil
}
