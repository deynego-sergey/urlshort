# === Stage 1: Build Go Backend ===
FROM golang:1.26-alpine AS builder
WORKDIR /app

# 1. Копируем go.mod и go.sum из подпапки src/
COPY src/go.mod src/go.sum* ./
RUN go mod download

# 2. Копируем весь исходный код Go
COPY src/ .

# 3. Копируем собранный React-фронтенд из cmd/api/dist в папку бэкенда
COPY cmd/api/dist ./cmd/api/dist

# 4. Собираем бинарник из подпапки cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/api-server ./cmd/api

# === Stage 2: Minimal Runtime ===
FROM alpine:3.19
WORKDIR /app
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/api-server .

EXPOSE 8080

CMD ["./api-server"]