# Инструкция по первому запуску

## Шаг 1 — Установить зависимости

Нужно установить один раз:

- **Git**: https://git-scm.com/downloads
- **Docker Desktop**: https://www.docker.com/products/docker-desktop/
- **Go 1.22+**: https://go.dev/dl/ (только для backend-разработчика)

Проверка после установки:
```bash
git --version
docker --version
go version
```

---

## Шаг 2 — Клонировать репозиторий

```bash
git clone https://github.com/YOUR_USERNAME/1448.git
cd 1448
```

---

## Шаг 3 — Настроить окружение

```bash
cp .env.example .env
```

Для разработки ничего менять не нужно — дефолтные значения работают.
Для production — заменить все `*_your_key_here` на реальные ключи.

---

## Шаг 4 — Инициализировать Go зависимости

```bash
cd backend
go mod tidy
cd ..
```

Эта команда скачает все Go библиотеки (~30 сек).

---

## Шаг 5 — Запустить окружение

```bash
docker compose up -d
```

Первый запуск занимает 2–3 минуты (скачиваются образы).
Последующие — несколько секунд.

Проверить что всё запустилось:
```bash
docker compose ps
```

Все сервисы должны быть в статусе `Up` или `healthy`.

---

## Шаг 6 — Применить миграции БД

Открой браузер → http://localhost:5050 (pgAdmin)

Логин: `admin@1448.dev`  
Пароль: `admin`

1. В левом меню: **Servers → postgres → Databases → 1448_db**
2. Правая кнопка на `1448_db` → **Query Tool**
3. Открой файлы из `backend/migrations/` по порядку (001 → 007)
4. Для каждого: вставь содержимое → нажми **F5** (Run)

---

## Шаг 7 — Проверить что сервер работает

```bash
curl http://localhost:8080/health
```

Ответ:
```json
{"service":"14:48 Backend","status":"ok","version":"0.1.0"}
```

---

## Доступные сервисы

| Сервис | URL | Логин |
|--------|-----|-------|
| Go API | http://localhost:8080 | — |
| pgAdmin (БД) | http://localhost:5050 | admin@1448.dev / admin |
| Redis UI | http://localhost:8081 | — |

---

## Полезные команды

```bash
# Остановить всё
docker compose down

# Посмотреть логи бэкенда
docker compose logs -f backend

# Пересобрать после изменений в коде
docker compose up -d --build backend

# Полный сброс (удалит данные БД!)
docker compose down -v
```

---

## Структура проекта

```
1448/
├── backend/          # Go — REST API, WebSocket, PostgreSQL, Redis
│   ├── cmd/server/   # Точка входа
│   ├── internal/     # Бизнес-логика
│   └── migrations/   # SQL-миграции (001–007)
├── mobile/           # Flutter — iOS + Android
│   └── lib/l10n/     # Локализация: EN / PL / RU
├── admin/            # React — Admin Panel + Owner Stats
├── shell/            # C# — PC Shell (Windows)
└── docs/             # ТЗ, API Reference, архитектура
```

---

## Что реализовано сейчас (скелет)

- ✅ Все роуты API (заглушки 501)
- ✅ Модели: User, Session, Case, Computer, Club
- ✅ WebSocket Hub для PC Shell
- ✅ SQL-миграции всех таблиц
- ✅ Docker окружение (PostgreSQL + Redis + pgAdmin)
- ✅ Flutter: тема, i18n EN/PL/RU, навигация
- ✅ Go module path: `github.com/1448-project/backend`

## Что нужно реализовать

Go-разработчик: авторизация (JWT + OTP), логика сессий, XP-движок, алгоритм кейсов, WebSocket  
Flutter-разработчик: экраны Home, Cases, Talents, Profile, Maps  
C#-разработчик: Shell Launcher, overlay виджет, WebSocket клиент  
React-разработчик: Admin Panel, карта ПК, Owner Stats, графики
