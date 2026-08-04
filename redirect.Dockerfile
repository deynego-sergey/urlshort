# === Stage 1: Build Binary ===
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Копируем манифесты зависимостей из src/
COPY src/go.mod src/go.sum* ./
RUN go mod download

# Копируем исходники из src/
COPY src/ .

# Собираем сервис редиректа
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/redirect-server ./cmd/redirect/main.go

# === Stage 2: Runtime ===
FROM alpine:3.19
WORKDIR /app
RUN apk --no-cache add ca-certificates

RUN mkdir -p /app/logs /var/run/app

COPY --from=builder /app/redirect-server .

VOLUME ["/app/logs", "/var/run/app"]

EXPOSE 8081

CMD ["./redirect-server"]