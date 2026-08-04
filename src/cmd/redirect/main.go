package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"urlshort/internal/repository/cache"
	"urlshort/internal/repository/link"
	"urlshort/pkg/database/pg"
	"urlshort/pkg/httplog"
	"urlshort/pkg/utils"
)

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func getEnvInt64OrDefault(key string, defaultValue int64) int64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return defaultValue
	}
	return val
}

func main() {
	// 1. Инициализация контекста для Graceful Shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 2. Инициализация БД (Supabase / PostgreSQL) и репозитория
	pool, err := pg.InitSupabasePool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repo := link.NewLinkRepository(pool)
	if err = repo.CreateTable(ctx); err != nil {
		log.Fatal(err)
	}

	// 3. Инициализация кэша в RAM и конвертера кодов
	resolver := cache.NewCache()
	converter := utils.NewConverter(utils.GetAlphabetString())

	// 4. Инициализация параметров и структуры логирования (FileRotator + SocketSender)
	unixSock := getEnvOrDefault("UNIX_SOCKET", "/tmp/stats.sock")
	logDir := getEnvOrDefault("LOG_DIR", "./logs/httplog")
	maxLogSizeBytes := getEnvInt64OrDefault("LOG_MAX_SIZE_BYTES", 1*30*1024) //  100 kb  //10 MB по умолчанию

	rotator, err := httplog.NewFileRotator(logDir, maxLogSizeBytes)
	if err != nil {
		log.Fatalf("failed to initialize file rotator: %v", err)
	}
	defer func() {
		_ = rotator.ForceRotate()
		_ = rotator.Close()
	}()

	// Запускаем воркер отправки накопившихся логов через сокет один раз при старте
	ssender := httplog.NewSocketSender(unixSock, logDir)
	go ssender.Start(ctx)

	// 5. Создаем HTTP-роутер
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{sh}", func(w http.ResponseWriter, r *http.Request) {

		code := r.PathValue("sh")
		// Игнорируем авто-запросы иконки браузером
		if code == "favicon.ico" || code == "" {
			http.NotFound(w, r)
			return
		}
		id, err := converter.ConvertToInt(code)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// Раннее извлечение данных: сразу снимаем метрику HTTP-запроса
		payload := httplog.NewRequestPayload(r, "")
		logDone := make(chan string, 1)

		// Запускаем фоновую горутину записи лога в файл
		go func(p *httplog.RequestPayload) {
			logCtx, logCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer logCancel()

			// Ожидаем целевой URL из основного потока
			p.TargetURL = <-logDone
			if err := rotator.Write(logCtx, p); err != nil {
				log.Println(err)
			}
		}(payload)

		// Проверяем наличие в RAM-кэше
		originalURL, ok := resolver.Get(id)
		if !ok {
			// Если нет в кэше — загружаем из PostgreSQL
			l, err := repo.GetLinkByID(r.Context(), id)
			if err != nil || l.IsDeleted {
				logDone <- "" // Передаем пустой target, если ссылка не найдена
				http.Error(w, "Link not found or expired", http.StatusNotFound)
				return
			}

			originalURL = l.OriginalURL
			resolver.Put(id, originalURL)
		}

		// Передаем итоговый URL в фоновую горутину логирования
		logDone <- originalURL

		// Отправляем редирект клиенту
		http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
	})

	// 6. Настройка и запуск HTTP-сервера
	server := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Println("Redirect service started on :8081")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Ожидание сигналов завершения процесса
	<-ctx.Done()
	log.Println("Shutting down redirect service...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced shutdown error: %v", err)
	}
}
