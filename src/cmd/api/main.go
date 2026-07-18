package main

import (
	"context"
	"net/http"
	"urlshort/cmd/api/handlers"
	"urlshort/internal/repository/user"
	"urlshort/internal/services/auth"
	"urlshort/pkg/database/pg"
)

func main() {

	ctx, cf := context.WithCancel(context.Background())
	defer cf()
	// Инициализируем пул базы данных (параметры берутся из окружения)
	// !!! set env var "DATABASE_URL"
	pool, err := pg.InitSupabasePool(ctx)
	//pool, _ := pgxpool.New(context.Background(), "postgres://user:pass@localhost:5432/db")
	//defer pool.Close()

	// 1. Инициализируем Слой Данных (Repositories)
	userRepo := user.NewUserRepository(pool)
	sessionRepo := user.NewSessionRepository(pool)
	memStorage := user.NewSessionMemoryStorage()

	// 2. Инициализируем Слой Бизнес-логики (Service), внедряя репозитории через DI
	authService := auth.NewAuthService(userRepo, sessionRepo, memStorage, "YOUR_JWT_SECRET_KEY")

	// 3. Инициализируем Слой Представления (Handlers), внедряя сервис
	internalHandler := handlers.NewInternalHandler(authService)

	// Настраиваем мультиплексор для работы за NGINX
	mux := http.NewServeMux()
	mux.Handle("/v1/internal", internalHandler)

	_ = http.ListenAndServe(":8080", mux)
}
