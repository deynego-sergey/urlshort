package main

import (
	"log"
	"net/http"
	"time"

	//"yourproject/repository"
	//"yourproject/services"
)

func main() {
	// 1. Инициализация БД (Supabase) и репозитория
	// db := initPostgres()
	var repo repository.LinkRepository // Наша реализация с Soft Delete

	// 2. Инициализация легковесного кэша (кэш живет в RAM вашего сервера)
	resolver := services.NewLinkResolverService(repo, 24*time.Hour)

	// 3. Создаем стандартный роутер Go 1.22+
	mux := http.NewServeMux()

	// Главный и единственный эндпоинт для редиректа
	mux.HandleFunc("GET /r/{code}", func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		if code == "" {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Ищем в кэше, если нет — в Supabase
		originalURL, err := resolver.Resolve(r.Context(), code)
		if err != nil {
			http.Error(w, "Link not found or expired", http.StatusNotFound)
			return
		}

		// Мгновенный временный редирект (302 Found)
		http.Redirect(w, r, originalURL, http.StatusFound)
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
}
