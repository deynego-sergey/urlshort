package main

import (
	"context"
	"log"

	"urlshort/internal/repository/mongo/stats"
	"urlshort/internal/services/collector"
	"urlshort/pkg/httplog"
)

// StartSocketAdapter запускает httplog.SocketListener, который читает события из сокета
// и отправляет их в Collector.
func StartSocketAdapter(ctx context.Context, socketPath string, coll *collector.Collector) error {
	handler := func(ctx context.Context, payload *httplog.RequestPayload) error {
		if payload == nil {
			return nil
		}

		// Игнорируем логи без целевого URL (например, 404 ошибки)
		if payload.TargetURL == "" {
			payload.TargetURL = "unknown"
			//return nil
		}

		// Проверяем наличие URLPath (если по какой-то причине пуст, берем RequestURI)
		if payload.URLPath == "" && payload.RequestURI != "" {
			payload.URLPath = payload.RequestURI
		}

		// Передаем весь полученный payload без потери полей
		statUpdate := stats.StatUpdate{
			RequestPayload: *payload,
		}

		log.Printf("[DEBUG] Pushing stat update to collector: path=%s, target=%s", payload.URLPath, payload.TargetURL)

		// Отправляем событие в накопитель
		coll.Push(statUpdate)
		return nil
	}

	listener := httplog.NewSocketListener(socketPath, handler)
	return listener.ListenAndServe(ctx)
}
