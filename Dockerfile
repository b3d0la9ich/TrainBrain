# ======= STAGE 1: build (Go) =======
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Нужен git для скачивания модулей
RUN apk add --no-cache git

# Чтобы не упираться в proxy.golang.org
ENV GOPROXY=https://goproxy.io,direct

# Тащим весь проект
COPY . .

# Подтягиваем зависимости / генерим go.sum
RUN go mod tidy

# Собираем бинарник
RUN go build -o server .

# ======= STAGE 2: runtime =======
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

# Копируем бинарник
COPY --from=builder /app/server .

# Копируем шаблоны и статику из билд-стейджа
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

ENV PORT=5001
EXPOSE 5001

CMD ["./server"]
