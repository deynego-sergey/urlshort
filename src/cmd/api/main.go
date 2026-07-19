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

	"urlshort/cmd/api/handlers"
	"urlshort/internal/repository/link"
	"urlshort/internal/repository/user"
	"urlshort/internal/services/auth"
	"urlshort/pkg/database/pg"
)

func main() {
	// Создаем базовый контекст приложения для каскадного управления ресурсами
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализируем пул базы данных (параметры конфигурации берутся строго из окружения)
	// !!! Обязательно должна быть установлена переменная окружения "DATABASE_URL"
	pool, err := pg.InitSupabasePool(ctx)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize Supabase pool: %v", err)
	}

	// 1. Инициализируем Слой Данных (Repositories)
	userRepo := user.NewUserRepository(pool)
	sessionRepo := user.NewSessionRepository(pool)

	// Передаем контекст приложения в MemoryStorage, чтобы фоновый GC завершался вместе с сервером
	memStorage := user.NewSessionMemoryStorage(ctx)

	// Инициализируем репозиторий ссылок
	linkRepo, err := link.NewLinkRepository(ctx)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize link repository: %v", err)
	}

	// Автоматическое создание таблиц при старте (Миграция структур)
	log.Println("[INFO] Initializing database schemas...")
	if err := userRepo.CreateTable(ctx); err != nil {
		log.Fatalf("[FATAL] Users schema init failed: %v", err)
	}
	if err := sessionRepo.CreateTable(ctx); err != nil {
		log.Fatalf("[FATAL] Sessions schema init failed: %v", err)
	}
	if err := linkRepo.CreateTable(ctx); err != nil {
		log.Fatalf("[FATAL] Links schema init failed: %v", err)
	}
	log.Println("[INFO] Database schemas are up to date.")

	// 2. Инициализируем Слой Бизнес-логики (Service), внедряя репозитории через DI
	// TODO: Заменить "YOUR_JWT_SECRET_KEY" на реальный секрет, загружаемый из окружения
	authService := auth.NewAuthService(userRepo, sessionRepo, memStorage, "YOUR_JWT_SECRET_KEY")

	// 3. Инициализируем Слой Представления (Handlers), внедряя зависимости
	internalHandler := handlers.NewInternalHandler(authService, linkRepo)

	// Настраиваем мультиплексор для работы за NGINX
	mux := http.NewServeMux()
	mux.Handle("/v1/internal", internalHandler)

	// Создаем сервер явным образом для управления процессом Shutdown
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Каналы для перехвата системных сигналов прерывания (терминал, systemd, NGINX)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Запуск HTTP-сервера в отдельной горутине, чтобы не блокировать основной поток ожидания сигналов
	go func() {
		log.Println("[INFO] Server is starting on :8080...")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[FATAL] Server failed to start: %v", err)
		}
	}()

	// Блокируемся здесь и ждем сигнал остановки от операционной системы
	sig := <-sigChan
	log.Printf("[INFO] Received signal: %v. Initiating graceful shutdown...", sig)

	// Выделяем жесткий таймаут в 5 секунд на завершение текущих активных сетевых запросов клиентов
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] HTTP server Shutdown failed: %v", err)
	} else {
		log.Println("[INFO] HTTP server stopped cleanly.")
	}

	// Отменяем корневой контекст приложения — останавливаем фоновые горутины (включая GC в memStorage)
	cancel()

	// Закрываем физический пул соединений PostgreSQL к Supabase строго после остановки хендлеров
	log.Println("[INFO] Closing database connection pool...")
	pool.Close()
	log.Println("[INFO] Database pool connection closed. Shutdown complete.")
}
