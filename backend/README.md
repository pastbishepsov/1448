# 14:48 — Backend (Go)

REST API + WebSocket сервер на Go.

## Стек
- **Go 1.22** + Gin (HTTP роутер)
- **GORM** + PostgreSQL (основная БД)
- **Redis** (кэш, JWT refresh tokens, real-time состояние ПК)
- **gorilla/websocket** (PC Shell ↔ Server)
- **Twilio** (SMS OTP авторизация)

## Структура
```
backend/
├── cmd/server/main.go          # Точка входа
├── internal/
│   ├── config/                 # Загрузка .env конфигурации
│   ├── models/                 # GORM модели (User, Session, Case...)
│   ├── handlers/               # HTTP обработчики (TODO)
│   ├── middleware/             # JWT, CORS, Rate limiting (TODO)
│   ├── routes/                 # Определение всех эндпоинтов
│   ├── services/               # Бизнес-логика: XP, кейсы, достижения (TODO)
│   ├── repository/             # Слой работы с БД (TODO)
│   └── websocket/              # Hub для PC Shell соединений
└── migrations/                 # SQL-файлы миграций
```

## Запуск для разработки
```bash
# 1. Поднять БД и Redis (из корня проекта)
cd .. && docker-compose up -d

# 2. Установить зависимости
go mod download

# 3. Скопировать .env
cp ../.env.example ../.env

# 4. Запустить миграции
docker-compose exec postgres bash -c \
  "for f in /migrations/*.sql; do psql -U postgres -d db_1448 -f \$f; done"

# 5. Запустить сервер
go run cmd/server/main.go
```

## API
- `GET /health` — проверка работоспособности
- `POST /api/v1/auth/register` — регистрация
- `POST /api/v1/auth/otp/send` — отправка OTP на телефон
- Полный список в [../docs/API.md](../docs/API.md)

## Следующие шаги для Go-разработчика
1. Реализовать `internal/handlers/auth.go` (register, login, OTP)
2. Подключить GORM к PostgreSQL в `cmd/server/main.go`
3. Подключить Redis клиент
4. Реализовать JWT middleware в `internal/middleware/auth.go`
5. Написать `internal/services/xp.go` — логика начисления XP
