// src/cmd/api/main.go
package main

import (
	"context"
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"urlshort/cmd/api/handlers"
	"urlshort/internal/repository/link"
	"urlshort/internal/repository/user"
	"urlshort/internal/services/auth"
	"urlshort/internal/services/notification"
	"urlshort/pkg/database/pg"
	"urlshort/pkg/middleware/cors"
	middleware "urlshort/pkg/middleware/jwtauth"
	"urlshort/pkg/utils"
)

//go:embed dist/*
var webFiles embed.FS

func main() {
	// Передаем эмбеднутые файлы в handlers
	handlers.EmbeddedFiles = webFiles

	log.Println("Starting API server...")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pg.InitSupabasePool(ctx)
	if err != nil {
		log.Fatalf("Critical: failed to initialize database pool: %v", err)
	}
	defer pool.Close()

	emailCfg, err := notification.LoadEmailConfigFromEnv()
	if err != nil {
		log.Fatalf("Critical boot error: %v", err)
	}

	senders := map[notification.TargetType]notification.INotificationSender{
		notification.TargetEmail: notification.NewEmailSender(emailCfg),
	}
	notificationService := notification.NewNotificationService(senders)

	userRepo := user.NewUserRepository(pool)
	sessionRepo := user.NewSessionRepository(pool)
	linkRepo := link.NewLinkRepository(pool)

	sessionMemory := user.NewSessionMemoryStorage(ctx)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	authService := auth.NewAuthService(userRepo, sessionRepo, sessionMemory, notificationService, jwtSecret)

	internalHandler := handlers.NewInternalHandler(authService, linkRepo, utils.NewConverter(utils.GetAlphabetString()))

	authMiddleware := middleware.AuthMiddleware(jwtSecret)
	corsMiddleware := cors.CorsMiddleware

	mux := http.NewServeMux()

	// 1. Ручка API
	mux.Handle("/v1", corsMiddleware(authMiddleware(internalHandler)))

	// 2. Раздача фронтенда (все остальное отправляем в SPA)
	mux.Handle("/", handlers.SPAHandler())

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server listen failed: %v", err)
		}
	}()
	log.Println("Server is running on port :8080")

	<-ctx.Done()

	log.Println("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server gracefully stopped.")
}
