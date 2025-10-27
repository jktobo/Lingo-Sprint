# 1. Сборка
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Копируем .mod и .sum и скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь остальной код
COPY . .

# Собираем бинарный файл
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server/main.go

# 2. Финальный образ (очень маленький)
FROM alpine:latest

WORKDIR /app

# Копируем только собранный файл из 'builder'
COPY --from=builder /app/server /app/server

# Копируем нашу папку /web (с /static внутри) из 'builder'
COPY --from=builder /app/web /app/web

# Открываем порт, на котором будет работать наш Go-сервер
EXPOSE 8080

# Команда для запуска нашего сервера
ENTRYPOINT [ "/app/server" ]