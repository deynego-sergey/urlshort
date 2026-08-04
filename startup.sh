#!/usr/bin/bash
# 1. Загрузка образов из .tar.gz архивов
docker load < urlshort-api.tar.gz
docker load < urlshort-redirect.tar.gz
docker load < urlshort-stats.tar.gz

# 2. Запуск контейнеров из загруженных образов
#docker compose up -d
docker compose --env-file .env up -d