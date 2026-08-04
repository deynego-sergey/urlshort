FRONTEND_PATH ?= ../urlshort-front
DIST_DEST ?= src/cmd/api/dist

IMAGE_API ?= urlshort-api:latest
IMAGE_REDIRECT ?= urlshort-redirect:latest
IMAGE_STATS ?= urlshort-stats:latest

ARCHIVE_API ?= urlshort-api.tar.gz
ARCHIVE_REDIRECT ?= urlshort-redirect.tar.gz
ARCHIVE_STATS ?= urlshort-stats.tar.gz

.PHONY: all build-front copy-dist build-docker save-archives clean help

all: build-front copy-dist build-docker save-archives

## build-front: Собирает React фронтенд
build-front:
	@echo "==> Сборка React фронтенда..."
	cd $(FRONTEND_PATH) && npm run build

## copy-dist: Копирует собранный dist в папку Go (src/cmd/api/dist)
copy-dist:
	@echo "==> Копирование dist в $(DIST_DEST)..."
	rm -rf $(DIST_DEST)
	mkdir -p $(DIST_DEST)
	cp -r $(FRONTEND_PATH)/dist/* $(DIST_DEST)/

## build-docker: Собирает все 3 Docker-образа по новым именам *.Dockerfile
build-docker:
	@echo "==> Сборка Docker-образа API..."
	docker build -f api.Dockerfile -t $(IMAGE_API) .
	@echo "==> Сборка Docker-образа Redirect..."
	docker build -f redirect.Dockerfile -t $(IMAGE_REDIRECT) .
	@echo "==> Сборка Docker-образа Stats..."
	docker build -f stats.Dockerfile -t $(IMAGE_STATS) .

## save-archives: Экспортирует Docker-образы в .tar.gz архивы
save-archives:
	@echo "==> Экспорт $(ARCHIVE_API)..."
	docker save $(IMAGE_API) | gzip > $(ARCHIVE_API)
	@echo "==> Экспорт $(ARCHIVE_REDIRECT)..."
	docker save $(IMAGE_REDIRECT) | gzip > $(ARCHIVE_REDIRECT)
	@echo "==> Экспорт $(ARCHIVE_STATS)..."
	docker save $(IMAGE_STATS) | gzip > $(ARCHIVE_STATS)
	@echo "==> Все архивы успешно созданы!"

## clean: Удаляет временные файлы, dist и архивы
clean:
	@echo "==> Очистка временно созданных файлов..."
	rm -rf $(DIST_DEST)
	rm -f $(ARCHIVE_API) $(ARCHIVE_REDIRECT) $(ARCHIVE_STATS)

## help: Выводит справку по доступным командам
help:
	@echo "Доступные команды:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST)