#!/bin/bash

# Настройки
DOCKER_FILE="service.Dockerfile"
SERVICE="redirect"
IMAGE_NAME="urlshortener-$SERVICE"
TAG="latest"
OUTPUT_FILE="$IMAGE_NAME.tar.gz"

echo "🚀 Начинаем сборку сервиса [$SERVICE] из файла $DOCKER_FILE..."

# 1. Сборка образа
docker build -f "$DOCKER_FILE" --build-arg SERVICE_NAME="$SERVICE" -t "$IMAGE_NAME:$TAG" ./src

if [ $? -ne 0 ]; then
    echo "❌ Ошибка при сборке Docker-образа."
    exit 1
fi

echo "📦 Образ успешно собран. Начинаем экспорт в архив $OUTPUT_FILE..."

# 2. Экспорт и сжатие
docker save "$IMAGE_NAME:$TAG" | gzip > "$OUTPUT_FILE"

if [ $? -eq 0 ]; then
    echo "✅ Готово! Архив сохранен в корне проекта."
    echo "📊 Размер архива: $(du -sh "$OUTPUT_FILE" | cut -f1)"
else
    echo "❌ Ошибка при экспорте образа."
    exit 1
fi