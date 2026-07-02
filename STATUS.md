# 14:48 — Состояние проекта

> Живой документ. Обновляется по мере работы. Точка входа для всех: открой его первым.
> Последнее обновление: 2026-07-02.

**14:48** — геймифицированная SaaS-платформа для компьютерных клубов. Игрок получает XP за время за ПК, прокачивает уровни и таланты, открывает кейсы, получает кэшбек. Конкуренты делают инструмент контроля — мы делаем продолжение игры.

Рынок: Варшава → ЕС. Только Польша.

---

## TL;DR для нового разработчика

1. Поставь Docker Desktop.
2. `cp .env.example .env` (дефолты для dev уже рабочие).
3. `docker compose up -d --build` из корня репо.
4. Применить миграции и seed (см. [Запуск](#запуск-локально)).
5. `curl http://localhost:8080/health` → должен ответить `ok`.
6. Рабочий код бэкенда — в `backend/cmd/server/` (`main.go`, `sessions.go`). С него и начинай.

---

## Что уже работает (проверено end-to-end)

Бэкенд на Go. Все эндпоинты ниже реально реализованы и протестированы curl'ом.

| Метод | Путь | Описание | Auth |
|-------|------|----------|------|
| GET  | `/health` | health-check | — |
| POST | `/api/v1/auth/register` | регистрация (bcrypt + выдача JWT) | — |
| POST | `/api/v1/auth/login` | вход по нику/email + пароль | — |
| POST | `/api/v1/auth/refresh` | обмен refresh-токена на новую пару | — |
| GET  | `/api/v1/me` | профиль текущего игрока | JWT |
| PATCH | `/api/v1/me` | смена ника / аватара | JWT |
| POST | `/api/v1/me/sessions/start` | начать сессию за ПК | JWT |
| POST | `/api/v1/me/sessions/end` | завершить сессию, начислить XP/coins | JWT |
| GET  | `/api/v1/me/sessions` | история сессий игрока | JWT |
| GET  | `/api/v1/ws/shell?computer_id=` | WebSocket-канал для PC Shell (real-time) | computer_id |
| GET  | `/api/v1/me/cases` | список кейсов игрока | JWT |
| POST | `/api/v1/me/cases/:id/open` | открыть кейс (дроп через crypto/rand) | JWT |
| GET  | `/api/v1/me/talents` | дерево талантов + текущие эффекты | JWT |
| POST | `/api/v1/me/talents/invest` | вложить очко навыка в талант | JWT |
| GET  | `/api/v1/me/achievements` | достижения: получено / доступно | JWT |
| GET  | `/api/v1/leaderboard` | топ игроков по опыту + твоё место | JWT |

**XP-движок** (ядро игры) работает целиком на сервере:
- начисление XP и coins за минуты игры;
- повышение уровня по формуле `XP(n) = 1000 · n^1.4`;
- выдача очков навыков (skillpoints) за уровень.

Аутентификация: JWT HS256, `sub = user_id`, claim `typ` разделяет токены.
Access — 15 мин (`JWT_ACCESS_TTL`), refresh — 30 дней (`JWT_REFRESH_TTL`).
Login/register выдают пару; `/auth/refresh` ротирует её. Middleware пускает
**только** `typ=access` — refresh-токеном защищённые роуты не открыть.
Refresh stateless (без хранилища) → отзыв конкретного токена появится вместе
с logout (blacklist в Redis). Логика: `cmd/server/auth.go` (+ `auth_test.go`).

**Гостевой экран** (`web/shell.html`, спринт 1 трека «ПК как клуб»):
- фулскрин-лаунчер: игры первым блоком (CS2, Dota 2, Valorant, Fortnite, LoL, GTA V),
  ниже приложения (Steam, Discord, браузер, Spotify...) и система;
- запуск: `steam://`/`discord://`-протоколы работают уже сейчас (если программа
  установлена); остальное — через shell-agent (спринт 2);
- живьём на API: XP-бар, сессия с таймером, кейсы (открытие с анимацией), таланты
  (вложение очков), ачивки, лидерборд, профиль (ник+аватар), refresh-токены;
- запуск: открыть файл в браузере; киоск-режим:
  `msedge --kiosk "file:///<путь>/web/shell.html" --edge-kiosk-type=fullscreen`.

**Real-time / WebSocket** (мост к PC Shell):
- ПК подключается к `/api/v1/ws/shell?computer_id=...`;
- сервер шлёт команды: `session_start`, `session_end` (далее — `force_unlock`, `xp_update`);
- ПК шлёт: `session_tick` (heartbeat), `admin_call`;
- проверить без десктопа можно через `tools/shell-emulator.html` (открыть в браузере).

**Кейсы** (гача-экономика):
- выдаются за достижения (включая первый вход), за новый уровень и с шансом за каждую
  сессию (шанс — талант `case_hunter`; тир бонусного кейса роллится: чаще Light,
  реже выше — талант `luck_grade`);
- открытие считает дроп через `crypto/rand` **только на сервере** — coins или бустер кэшбека;
- экономика проверена симуляцией на 500k открытий/тир: `tools/case_economy_sim.py`.

**Таланты** (3 ветки: Strength / Agility / Intellect):
- 9 талантов сидятся миграцией 006 (`talent_definitions`);
- игрок вкладывает очки навыков (`POST /me/talents/invest`) — проверка лимитов в `canInvestTalent`;
- эффект таланта возвращается как сырое число (`effect_now = уровень × effect_per_level`);
- **применяются** (3 из 9): `xp_boost` (Agility) — опыт за сессию; `case_hunter` (Strength) —
  шанс бонусного кейса; `luck_grade` (Strength) — шанс Heavy+ тира бонусного кейса;
- остальные (`cashback_master`, `coin_mint`, `double_drop` и т.д.) — по мере появления
  их механик (кэшбек при оплате, депозиты, двойной дроп).

**Достижения** (`achievements`, сидятся миграцией 005):
- движок проверяет условия после сессии и при регистрации, выдаёт награды
  (очки навыков + кейс), запоминает в `user_achievements`;
- сейчас вычисляются `hours_played` (1/10/100 ч) и `login_count` (первый вход);
- `deposit_count`, `phone_verified`, стрики — ждут своих механик;
- логика условий (`conditionMet`) покрыта тестом и проверена симуляцией.

---

## Что в заглушках (возвращают 501)

Маршруты определены в `backend/cmd/server/main.go`, но логики пока нет:
`/auth/otp/*`, `/auth/logout`, `/clubs*`.

---

## Запуск локально

Из корня репозитория. Предполагается запущенный Docker Desktop.

```bash
# 1. окружение (PostgreSQL + Redis + backend + pgAdmin)
docker compose up -d --build

# 2. миграции (создают таблицы) — один раз
for f in backend/migrations/*.sql; do
  docker compose exec -T postgres psql -U 1448_user -d 1448_db < "$f"
done

# 3. демо-данные (клуб + компьютеры) — один раз
docker compose exec -T postgres psql -U 1448_user -d 1448_db < backend/seed_dev.sql

# 4. проверка
curl http://localhost:8080/health
```

Сервисы:

| Сервис | URL | Доступ |
|--------|-----|--------|
| Go API | http://localhost:8080 | — |
| pgAdmin | http://localhost:5050 | admin@1448.dev / admin |
| Redis UI | http://localhost:8081 | — |

Дефолты БД (dev): пользователь `1448_user`, пароль `change_me_in_production`, база `1448_db`.

---

## Как протестировать игровой цикл

```bash
# регистрация (если ещё нет пользователя)
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"nickname":"egor","password":"secret123"}'

# токен в переменную
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"login":"egor","password":"secret123"}' \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

# старт сессии (берёт первый свободный ПК)
curl -X POST http://localhost:8080/api/v1/me/sessions/start -H "Authorization: Bearer $TOKEN"

# стоп. minutes — dev-оверрайд (только при SERVER_ENV!=production),
# чтобы увидеть начисление без ожидания реального времени
curl -X POST http://localhost:8080/api/v1/me/sessions/end \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"minutes":150}'

# профиль и история
curl http://localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
curl http://localhost:8080/api/v1/me/sessions -H "Authorization: Bearer $TOKEN"
```

Ожидаемо за 150 минут: +1500 XP, +300 coins, переход на уровень 2, +1 очко навыка.

---

## Подводные камни (важно)

- **Hot-reload не ловит изменения на Windows.** `air` следит за файлами внутри контейнера,
  но события об изменении через bind-mount с Windows до него не доходят. После правки Go-кода:
  `docker compose restart backend`.
- **`go.sum` обязателен.** Лежит в репо. Если пропадёт — сгенерировать:
  `docker compose run --rm --no-deps backend go mod tidy`.
- **ТЗ не в публичном репо.** Файл с полным ТЗ намеренно не коммитится (отдаётся только после NDA).
- **Два набора роутов.** Сейчас активны роуты из `cmd/server/main.go`. В `internal/api/router/router.go`
  есть альтернативный (более полный) роутер, который **пока не подключён**. Будущая задача —
  консолидировать всё на один роутер. Не путать.

---

## Структура

```
1448/
├── backend/                 Go: Gin + GORM + PostgreSQL + Redis
│   ├── cmd/server/
│   │   ├── main.go          ← АКТИВНАЯ точка входа: конфиг, БД, роуты, auth
│   │   └── sessions.go      ← сессии + XP-движок
│   ├── internal/
│   │   ├── config/          загрузка .env
│   │   ├── models/          User, Session, Case, Computer, Club (+ формула XP, логика дропа кейсов)
│   │   ├── api/             router/handlers/middleware (router пока не подключён, см. подводные камни)
│   │   └── websocket/       Hub для PC Shell — подключён к /ws/shell
│   ├── migrations/          7 SQL-миграций (все таблицы)
│   └── seed_dev.sql         демо-клуб + компьютеры
├── mobile/                  Flutter (скелет: тема, i18n EN/PL/RU, навигация)
├── admin/                   React + Vite + shadcn/ui (скелет)
├── shell/                   C# PC Shell (README, кода ещё нет)
└── docs/                    api.md, architecture.md
```

---

## Игровая экономика (справочник)

- **XP до следующего уровня:** `XP(n) = 1000 · n^1.4` (`models.XPForNextLevel`).
- **Начисление (dev-значения, потом вынесем в Admin Panel):** 10 XP/мин, 2 coins/мин, +1 очко навыка за уровень.
- **Кейсы:** Light / Medium / Heavy / Titan / God's. Дроп считается через `crypto/rand` **только на сервере**
  (логика в `models/case.go`, эндпоинт открытия пока заглушка).
- **Тариф:** PLN, 10–50 zł/час, средний ~23 zł/час. RTP-модификатор кейсов настраивается на клуб.

---

## Жёсткие правила проекта

- Рынок: только Польша → ЕС. Никакого российского рынка.
- Валюта PLN. Платежи: Stripe + BLIK. Карты: Google Maps / OpenStreetMap (не Yandex). Телефоны +48.
- Языки: English (основной), Polski, Русский.
- Юрлицо: Sp. z o.o. Право: GDPR + польское, суд — Варшава.

---

## Что дальше (спринтовый план, согласован 2026-07-02)

Цель ближайших спринтов: «мой ПК — как в клубе». Mobile (Flutter) отложен до после пилота.

1. ✅ **Спринт 1 — гостевой экран** (`web/shell.html`): лаунчер на живом API.
2. **Спринт 2 — shell-agent.exe** (Go): запуск программ с ПК, WS-связь с бэкендом,
   блокировка по концу сессии, вызов админа. Кросс-компиляция в Windows-exe через Docker.
3. **Спринт 3 — клубы и бронь + Admin MVP**: `GET /clubs*`, бронирование (миграция 007
   уже есть), админка (React): гости, статусы ПК real-time, сессии.
4. **Спринт 4 — экономика вглубь**: double_drop, депозиты + coin_mint/cashback_master,
   сгорание кейсов (`expires_at`), daily-квесты.
5. **Спринт 5 — продакшн-минимум**: logout (Redis blacklist), rate limiting, один роутер,
   CI (GitHub Actions), каталог приложений из админки. Убрать `.env` из git.
6. **Спринт 6 — пилот**: OTP (Twilio), Stripe+BLIK, деплой на VPS.

Сделано недавно: реальная авторизация, защищённый профиль, сессии + XP-движок,
WebSocket-канал PC Shell, кейсы (+ бонус за сессию с роллом тира), таланты
(эффекты xp_boost, case_hunter, luck_grade), достижения, `PATCH /me`,
лидерборд, refresh-токены (логика проверена тестами/симуляцией).
⚠️ Токены, выданные до refresh-релиза, отклоняются (нет `typ`) — просто перелогинься.

**Юнит-тесты логики:** `cd backend && go test ./...` — проверяют XP-движок и тиры кейсов
(`cmd/server/cases_test.go`).

Параллельно (трек Б, бесплатно): валидация рынка — показать прототип владельцам клубов Варшавы.

---

## Ритм работы

Каждый день — одна маленькая завершённая задача. Завершил → галочка → коммит.
Это топливо, чтобы двигаться без выгорания.
