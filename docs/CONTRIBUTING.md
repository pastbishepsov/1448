# Участие в разработке 14:48

Добро пожаловать в команду. Здесь всё, что нужно знать чтобы начать.

---

## Что мы строим

**14:48** — SaaS-платформа для компьютерных клубов с полной RPG-геймификацией.
Пилот: Варшава. Цель: рынок ЕС. Условия: equity (доля в проекте).

Подробнее: [README.md](README.md) и [docs/ТЗ_14-48_v1.1.docx](docs/)

---

## Как начать работу

### 1. Настрой окружение

```bash
git clone https://github.com/YOUR_USERNAME/1448.git
cd 1448
docker-compose up -d        # PostgreSQL + Redis
```

### 2. Запусти бэкенд

```bash
cd backend
cp .env.example .env        # Заполни секреты
go mod tidy
go run cmd/server/main.go
```

Проверь: `curl http://localhost:8080/health`

---

## Структура проекта

```
backend/
├── cmd/server/main.go          — точка входа
├── internal/
│   ├── api/
│   │   ├── handlers/           — HTTP-обработчики (твоя зона, Go-разработчик)
│   │   ├── middleware/         — JWT, CORS, Staff Auth
│   │   └── router/             — маршруты
│   ├── models/                 — модели данных + бизнес-логика
│   ├── services/               — сервисный слой (XP, кейсы, таланты)
│   ├── repository/             — работа с БД
│   ├── websocket/              — WebSocket hub для PC Shell
│   └── config/                 — конфигурация
├── migrations/
│   └── init.sql                — полная схема БД
└── .env.example
```

---

## Зоны ответственности

| Роль | Папка | Ключевые задачи |
|------|-------|----------------|
| Go Backend | `backend/` | API, бизнес-логика XP/кейсов, WebSocket |
| C# Developer | `shell/` | PC Shell, Shell Launcher, overlay виджеты |
| Flutter | `mobile/` | iOS + Android приложение |
| React | `admin/` | Admin Panel + Owner Stats |

---

## Правила работы с кодом

**Ветки:**
- `main` — стабильная версия (только через PR)
- `dev` — активная разработка
- `feat/название` — новая функциональность
- `fix/название` — исправление бага

**Коммиты:**
```
feat: добавить алгоритм открытия кейсов
fix: исправить формулу XP для уровня 20+
docs: обновить API документацию
```

**Pull Request:**
- Описание что сделано и зачем
- Скриншот или лог если меняется поведение
- Все тесты должны проходить

---

## Ключевые правила безопасности

1. **Все расчёты XP, Coins, Drop Rate — ТОЛЬКО на сервере**. Клиент только отображает.
2. **Никогда не логировать JWT-токены** или пароли
3. **Переменные окружения** — только через `.env`, не хардкодить в коде
4. **Случайные числа для кейсов** — только `crypto/rand`, не `math/rand`

---

## Связь с командой

Telegram: @YOUR_TELEGRAM (Егор, Product Owner)

Вопросы по ТЗ → в личку  
Баги и задачи → GitHub Issues  
Обсуждения → GitHub Discussions
