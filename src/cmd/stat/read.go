package main

import (
	"context"
	//"logac"
	"urlshort/internal/services/collector"

	"urlshort/internal/repository/mongo/stats"
	//"urlshort/pkg/collector"
	"urlshort/pkg/httplog"
)

// StartSocketAdapter запускаетhttplog.SocketListener, который читает события из сокета
// и отправляет их в Collector.
func StartSocketAdapter(ctx context.Context, socketPath string, coll *collector.Collector) error {
	handler := func(ctx context.Context, payload *httplog.RequestPayload) error {
		// Игнорируем логи без целевого URL (например, 404 ошибки)
		if payload.TargetURL == "" {
			return nil
		}

		// Преобразуем RequestPayload из пакета httplog в StatUpdate для репозитория
		statUpdate := stats.StatUpdate{
			httplog.RequestPayload{
				RequestURI: payload.URLPath,
				TargetURL:  payload.TargetURL,
				Timestamp:  payload.Timestamp,
				Referrer:   payload.Referer(),
				RemoteAddr: payload.ClientIP,
			},
		}

		// Отправляем событие в накопитель
		coll.Push(statUpdate)
		return nil
	}

	listener := httplog.NewSocketListener(socketPath, handler)
	return listener.ListenAndServe(ctx)
}
