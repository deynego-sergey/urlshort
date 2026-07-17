# Stage 1: Сборка
FROM golang:1.26-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Этим аргументом мы будем задавать, какой именно сервис собирать (api или redirect)
ARG SERVICE_NAME

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем бинарник, подставляя аргумент в путь cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o app ./cmd/${SERVICE_NAME}/main.go

# Stage 2: Запуск
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/app /app

EXPOSE 8080

ENTRYPOINT ["/app"]