package httplog

import (
	"net"
	"net/http"
	"strings"
)

// ExtractRealIP извлекает фактический IP-адрес клиента с учетом заголовков прокси
func ExtractRealIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if clientIP != "" {
				return clientIP
			}
		}
	}

	if xreal := r.Header.Get("X-Real-IP"); xreal != "" {
		return strings.TrimSpace(xreal)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
