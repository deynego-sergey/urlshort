cd ./src
go mod tidy
# 1. Собираем Docker-образы

docker build -f Dockerfile.redirect -t urlshort-redirect:latest .
docker build -f Dockerfile.stats -t urlshort-stats:latest .
docker build -f Dockerfile.api -t urlshort-api:latest .

# 2. Сохраняем образы в tar-архивы
docker save urlshort-api:latest | gzip > urlshort-api.tar.gz
docker save urlshort-redirect:latest | gzip > urlshort-redirect.tar.gz
docker save urlshort-stats:latest | gzip > urlshort-stats.tar.gz