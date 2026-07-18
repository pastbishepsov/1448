# 14:48

**Геймифицированная экосистема управления компьютерными клубами**

> Превращаем «прокат железа» в живую RPG-игру с бесконечным циклом удержания клиента.

Каждый конкурент (SmartShell, SENET, LANGAME) делает инструмент контроля.
Мы делаем продолжение игры: XP за время за ПК, уровни, таланты, кейсы,
достижения, кэшбек. Рынок: Варшава → ЕС.

**Точка входа для разработчика — [STATUS.md](STATUS.md):** живое состояние
проекта, все эндпоинты, запуск, подводные камни.

---

## Что уже работает

| Компонент | Состояние |
|-----------|-----------|
| `backend/` (Go + PostgreSQL) | ✅ ядро готово: auth (JWT + refresh с ротацией), XP-движок, кейсы, 6 работающих талантов, достижения, лидерборд, клубы и бронь, депозиты, админ-API, каталог приложений, rate limiting |
| `web/shell.html` | ✅ гостевой экран: фулскрин-лаунчер (игры → приложения → система), lock-экран, кейсы, таланты, бронь — на живом API |
| `web/admin.html` | ✅ админка (треки А+Б): зал-схема real-time, сессии, гости (депозит/XP/кейс/бан), брони, каталог по ролям, чат и вызовы гостей, журнал-аудит, owner: деньги/персонал/экономика |
| `backend/cmd/agent/` | ✅ shell-agent: запуск программ на гостевом ПК по allowlist (локальный + каталог с сервера), WS-связь, lock_action |
| `mobile/` (Flutter) | ⏸ скелет; отложен до после пилота |
| `shell/` (C# / WinUI 3) | 📋 README; промышленная замена агенту — под пилот |

CI: GitHub Actions (`go vet` + тесты + сборка + проверка JS экранов).

---

## Быстрый старт

Требования: Docker Desktop, Git.

```bash
# 1. окружение (PostgreSQL + Redis + backend)
docker compose up -d --build

# 2. миграции + демо-данные (один раз)
for f in backend/migrations/*.sql; do
  docker compose exec -T postgres psql -U 1448_user -d 1448_db < "$f"
done
docker compose exec -T postgres psql -U 1448_user -d 1448_db < backend/seed_dev.sql

# 3. проверка
curl http://localhost:8080/health
```

Дальше:
- **Гостевой экран** — открой `web/shell.html` в браузере, зарегистрируйся, начни сессию.
- **Админка** — `web/admin.html` (аккаунту нужна роль: `UPDATE users SET role='admin' WHERE nickname='<ник>';`).
- **Агент на ПК** — собрать `docker compose run --rm --no-deps -e GOOS=windows -e GOARCH=amd64 backend go build -o bin/shell-agent.exe ./cmd/agent`, настроить `agent.json` (см. `backend/cmd/agent/agent.example.json`).
- **Тесты** — `docker compose run --rm --no-deps backend go test ./...`

---

## Документы

- [STATUS.md](STATUS.md) — живое состояние: эндпоинты, экономика, спринты. **Начни отсюда.**
- [SETUP.md](SETUP.md) — детальная настройка окружения.
- `docs/` — архитектура и API-заметки. Полное ТЗ — не в публичном репо (после NDA).

## Жёсткие правила

Рынок: только Польша → ЕС. Валюта PLN, платежи Stripe + BLIK. Карты Google
Maps/OSM. Языки EN/PL/RU. Юрлицо Sp. z o.o., GDPR. Все расчёты XP/coins/дропов —
только на сервере.
