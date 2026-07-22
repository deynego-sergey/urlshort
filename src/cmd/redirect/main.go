package main

import (
	"context"
	"log"
	"net/http"
	"time"
	"urlshort/internal/repository/cache"
	"urlshort/internal/repository/link"
	"urlshort/pkg/database/pg"
	"urlshort/pkg/utils"
	//"yourproject/repository"
	//"yourproject/services"
)

const alphabet = "aBcDeFgHiJkLmNoPqRsTuVwXyZ8642097531AbCdRfGhIjKlMnOpQrStUvWxYz"

func main() {

	ctx, cf := context.WithCancel(context.Background())
	// 1. Инициализация БД (Supabase) и репозитория
	// db := initPostgres()
	pool, err := pg.InitSupabasePool(ctx)

	if err != nil {
		log.Fatal(err)
	}
	repo := link.NewLinkRepository(pool)
	if err = repo.CreateTable(ctx); err != nil {
		log.Fatal(err)
	}

	// 2. Инициализация легковесного кэша (кэш живет в RAM вашего сервера)
	resolver := cache.NewCache()

	converter := utils.NewConverter(alphabet)

	// 3. Создаем стандартный роутер Go 1.22+
	mux := http.NewServeMux()

	// Главный и единственный эндпоинт для редиректа
	mux.HandleFunc("GET /{sh}", func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("sh")
		if code == "" {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		id := converter.ConvertToInt(code)
		// Ищем в кэше, если нет — в Supabase
		originalURL, ok := resolver.Get(id)
		if !ok {
			if l, err := repo.GetLinkByID(r.Context(), id); err == nil {
				if !l.IsDeleted {
					resolver.Put(id, l.OriginalURL)
					http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
				}
			}

			http.Error(w, "Link not found or expired", http.StatusNotFound)
			return
		}

		http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
	})

	// 4. Запуск сервера с таймаутами для защиты от зависших соединений
	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("Redirect service started on :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}

	<-ctx.Done()
	defer cf()
}
