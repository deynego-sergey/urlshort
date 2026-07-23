package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"urlshort/pkg/utils"

	"urlshort/cmd/api/handlers"
	"urlshort/internal/repository/link"
	"urlshort/internal/repository/user"
	"urlshort/internal/services/auth"
	"urlshort/internal/services/notification"
	"urlshort/pkg/database/pg"
)

func main() {
	log.Println("Starting API server...")

	// 1. Контекст, завязанный напрямую на системные сигналы прерывания
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 2. Инициализация пула Supabase с передачей контекста и обработкой ошибки
	pool, err := pg.InitSupabasePool(ctx)
	if err != nil {
		log.Fatalf("Critical: failed to initialize database pool: %v", err)
	}
	defer pool.Close()

	// 3. Инициализация слоя уведомлений
	emailCfg, err := notification.LoadEmailConfigFromEnv()
	if err != nil {
		log.Fatalf("Critical boot error: %v", err)
	}

	senders := map[notification.TargetType]notification.INotificationSender{
		notification.TargetEmail: notification.NewEmailSender(emailCfg),
	}
	notificationService := notification.NewNotificationService(senders)

	// 4. Инициализация репозиториев (передаем готовый pool)
	userRepo := user.NewUserRepository(pool)
	sessionRepo := user.NewSessionRepository(pool)
	linkRepo := link.NewLinkRepository(pool)

	// Потокобезопасный in-memory кэш сессий, привязанный к сигнальному контексту
	sessionMemory := user.NewSessionMemoryStorage(ctx)

	// 5. Инициализация AuthService
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	authService := auth.NewAuthService(userRepo, sessionRepo, sessionMemory, notificationService, jwtSecret)

	// 6. Маршрутизация через единый InternalHandler
	internalHandler := handlers.NewInternalHandler(authService, linkRepo, utils.NewConverter(utils.GetAlphabetString()))

	mux := http.NewServeMux()
	mux.Handle("/v1/internal", internalHandler)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 7. Запуск HTTP сервера
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server listen failed: %v", err)
		}
	}()
	log.Println("Server is running on port :8080")

	// 8. Ожидаем системного сигнала прерывания (блокировка)
	<-ctx.Done()

	log.Println("Shutting down server...")

	// Жесткий таймаут на закрытие сетевых соединений (5 секунд)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server gracefully stopped.")
}
