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
| GET  | `/api/v1/clubs` | клубы + счётчики ПК (всего/свободно) | — |
| GET  | `/api/v1/clubs/:id` | карточка клуба | — |
| GET  | `/api/v1/clubs/:id/computers` | ПК клуба со статусами | — |
| POST | `/api/v1/clubs/:id/bookings` | бронь ПК (без пересечений) | JWT |
| GET  | `/api/v1/me/bookings` | мои брони | JWT |
| DELETE | `/api/v1/me/bookings/:id` | отмена будущей брони | JWT |
| GET  | `/api/v1/admin/overview` | сводка: гости/сессии/ПК/брони | admin |
| GET  | `/api/v1/admin/users?q=` | гости, поиск по нику | admin |
| POST | `/api/v1/admin/users/:id/ban` · `/unban` | бан/разбан игрока | admin |
| GET  | `/api/v1/admin/computers` | все ПК + кто играет + Shell online | admin |
| GET  | `/api/v1/admin/sessions/active` | активные сессии | admin |
| POST | `/api/v1/admin/sessions/:id/end` | форс-завершение (честное начисление) | admin |
| GET  | `/api/v1/admin/bookings` | ближайшие брони | admin |
| POST | `/api/v1/admin/bookings/:id/cancel` | отмена брони | admin |
| POST | `/api/v1/admin/users/:id/deposit` | пополнение баланса гостю | admin |
| GET  | `/api/v1/me/deposits` | история пополнений | JWT |
| POST | `/api/v1/admin/users/:id/grant` | ручное начисление XP/кейса (причина обязательна) | admin |
| GET  | `/api/v1/admin/grants` | журнал ручных начислений | admin |
| GET  | `/api/v1/catalog` | каталог приложений (экран + агент) | — |
| GET/POST | `/api/v1/admin/catalog` | каталог: список / upsert | admin |
| POST | `/api/v1/admin/catalog/:id/toggle` | вкл/выкл приложение | admin |
| DELETE | `/api/v1/admin/catalog/:id` | удалить приложение | admin |

Роли: `users.role` (player/admin/owner, миграция 008) → JWT-claim `role` →
`adminMiddleware`. Повысить аккаунт:
`UPDATE users SET role='admin' WHERE nickname='<ник>';` (после — перелогин).
Админка: `web/admin.html` (ПК, сессии, гости, брони; автообновление 7с).
Форс-завершение сессии переиспользует `finishSession` — ту же логику начисления,
что и у игрока (рефакторинг `sessions.go`).

**XP-движок** (ядро игры) работает целиком на сервере:
- начисление XP и coins за минуты игры;
- повышение уровня по формуле `XP(n) = 1000 · n^1.4` (бесконечный рост);
- выдача очков навыков (skillpoints) за уровень.

**Ранги аккаунта** (`ranks.go`, престиж по наигранным часам — 7 ступеней):
Новичок(0ч) → Завсегдатай(10) → Ветеран(25) → Мастер(50) → Элита(100) →
Легенда(200) → Бессмертный(400). В отличие от уровня — пассивная награда за
лояльность: множители XP (×1.0→×1.55) и coins (×1.0→×1.40), прибавка к шансу
кейса (+0→+25 п.п.) и бусту тира (+0→+0.55). Стекается с талантами. Симуляция
400k: доля Heavy+ растёт 11%→16%→20% (ранг+макс.luck). `GET /me/economy` отдаёт
ранг, ставки и эффекты талантов для калькулятора койнов. Тесты: `ranks_test.go`.

Аутентификация: JWT HS256, `sub = user_id`, claims `typ` (access/refresh),
`role` (player/admin/owner), `jti` (для отзыва).
Access — 15 мин (`JWT_ACCESS_TTL`), refresh — 30 дней (`JWT_REFRESH_TTL`).
Login/register выдают пару; `/auth/refresh` **ротирует с отзывом** старого
refresh (повторное использование невозможно); `/auth/logout` отзывает refresh.
Отозванные jti — в таблице `revoked_tokens` (миграция 010; Postgres вместо
Redis-blacklist: без новой инфраструктуры, переживает рестарты; Redis остаётся
для кэша/OTP). Access-токены не отзываются — живут ≤15 мин (компромисс).
Middleware пускает **только** `typ=access`. Логика: `cmd/server/auth.go` (+ тест).

Rate limiting: ~10 rps с IP, burst 20, in-memory (`ratelimit.go`, тест), кроме `/health`.
CI: GitHub Actions (`.github/workflows/ci.yml`) — go vet/test/build + синтаксис JS экранов.

**Гостевой экран** (`web/shell.html`, спринт 1 трека «ПК как клуб»):
- фулскрин-лаунчер: игры первым блоком (CS2, Dota 2, Valorant, Fortnite, LoL, GTA V),
  ниже приложения (Steam, Discord, браузер, Spotify...) и система;
- **каталог грузится с сервера** (`GET /catalog`, миграция 011 `catalog_apps`,
  редактируется во вкладке «Каталог» админки: добавление, правка, вкл/выкл,
  порядок); встроенный каталог остаётся запасным при недоступности API;
- запуск: `steam://`/`discord://`-протоколы работают уже сейчас (если программа
  установлена); exe-пути — через shell-agent (спринт 2);
- живьём на API: XP-бар, сессия с таймером, кейсы (открытие с анимацией), таланты
  (вложение очков), ачивки, лидерборд, профиль (ник+аватар), refresh-токены;
- запуск: открыть файл в браузере; киоск-режим:
  `msedge --kiosk "file:///<путь>/web/shell.html" --edge-kiosk-type=fullscreen`.

**Дизайн-язык «14:48 Noir»** (гостевой экран, редизайн по референсам reactbits/
glass3d/cta.gallery/designspells): чёрная база, **красный — единственный акцент**
(`--acc #ff2740`: шрифт, активное, свечение, «что выделяется»), вторичное —
нейтральный светлый `--acc2`. Фирменный красно-чёрный фон (приглушённые blob'ы),
glassmorphism, пульсирующее двоеточие в лого, premium-кнопки (shimmer+glow+magnet),
3D-tilt игровых тайлов (единая тёмная подложка, различие по TAG) со spotlight,
count-up чисел, star-border на Titan/God's кейсах, конфетти (красно-белые) при
дропе/левел-апе, click-spark, пружинные переходы вкладок. Тиры — монохромные глифы
по красной шкале (`--t-*`). Всё ванильным CSS/JS (движок `FX`), офлайн.
Перф-гард: `prefers-reduced-motion`, `hardwareConcurrency<=2`, FPS<32 → `perf-lite`.

**Студия сенсы** (вкладка «🎯», S1 — бесплатно всем игрокам, фича-отличие):
конвертер чувствительности мыши между играми (CS2, Valorant, Apex, Overwatch 2,
Call of Duty, Quake). Математика cm/360 и eDPI, `yaw`-коэффициенты игр (выверено
на CS2↔Valorant: множители ×3.18/×0.314, обратимость точна). DPI запоминается
(localStorage). Режим **только показываем** (игрок вписывает сам) — ноль античит-риска.

**S2 — детект мыши через агента** (`backend/cmd/agent/mouse*.go`): агент читает
акселерацию («повышенная точность указателя»), скорость указателя (1–20, идеал 10),
находит вендорский софт (Logitech/Razer/SteelSeries/Corsair/Glorious/Pulsar) по
процессам. Эндпоинты `GET /mouse-info`, `POST /mouse-accel-off` (единственное
действие записи — выключить акселерацию, безопасно). Windows-вызовы (`user32.dll`)
изолированы build-тегами: `mouse_windows.go` (реальные syscalls) / `mouse_other.go`
(заглушки для Linux/CI). Чистая логика (accel/вендоры) — `mouse.go` + `mouse_test.go`.
UI: блок «Твоя мышь» в студии сенсы — статус акселерации с кнопкой «Выключить»,
скорость, найденный софт + подсказка про DPI. Без агента — ручная инструкция.
**S3 — профиль сенсы в аккаунте** (`sensitivity.go`, миграция 013): DPI + сенсы
по играм (JSONB) сохраняются в `sensitivity_profiles` и синхронизируются с аккаунтом.
`GET /me/sensitivity` (дефолт, если пусто) и `PUT /me/sensitivity` (upsert,
валидация DPI 100–32000 и sens>0). Гостевой экран грузит профиль в `loadAll`,
автоподставляет сохранённый DPI и сенсу, кнопка «💾 Сохранить в аккаунт». Итог:
настройка прицела едет за игроком на любой ПК сети — то, чего нет у конкурентов.
**S4 — полировка студии**: пошаговая подача (Шаг 1 DPI → Шаг 2 игры), быстрые
пресеты DPI (400/800/1600/3200), таблица «твоя сенса во всех играх сразу»
(один cm/360 → готовые значения для каждой игры), добавлена Deadlock (Source 2).
Игр в конвертере: 7 с выверенным yaw. Fortnite/PUBG/R6 отложены сознательно —
нелинейные системы, добавим с точными формулами. Студия сенсы (S1–S4) завершена.

**Real-time / WebSocket** (мост к PC Shell):
- ПК подключается к `/api/v1/ws/shell?computer_id=...`;
- сервер шлёт команды: `session_start`, `session_end` (далее — `force_unlock`, `xp_update`);
- ПК шлёт: `session_tick` (heartbeat), `admin_call`;
- проверить без десктопа можно через `tools/shell-emulator.html` (открыть в браузере).

**Shell-agent** (`backend/cmd/agent`, спринт 2 — демо-класс, не античит-класс):
- локальный агент гостевого ПК: HTTP на `127.0.0.1:1448` для гостевого экрана —
  `POST /launch {app_id}` запускает программы строго по allowlist из `agent.json`
  (exe-пути, `steam://`-протоколы, `ms-settings:`), `POST /admin-call`, `GET /ping`;
- WS-клиент к бэкенду: heartbeat `session_tick`, приём `session_start`/`session_end`,
  автопереподключение;
- **allowlist = локальный `agent.json` + каталог сервера** (`catalog_url`, обновление
  каждые 5 мин; локальные записи главнее — для машинных путей);
- **`lock_action`** при `session_end`: `none` (лочит сам гостевой экран) |
  `lock_windows` (rundll32 LockWorkStation) | `kiosk` (поднять `kiosk_url` поверх);
- конфиг: скопировать `agent.example.json` → `agent.json` рядом с exe, вписать
  `computer_id` (UUID из таблицы `computers`);
- сборка Windows-exe через Docker (из корня):
  `docker compose run --rm --no-deps -e GOOS=windows -e GOARCH=amd64 backend go build -o bin/shell-agent.exe ./cmd/agent`
  → `backend/bin/shell-agent.exe`;
- гостевой экран сам находит агента (индикатор «🖥 ПК ●» в топбаре) и запускает
  всё через него; без агента — fallback на протоколы браузера.

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
- **применяются** (8 из 9): `xp_boost` (Agility) — опыт за сессию; `case_hunter` (Strength) —
  шанс бонусного кейса; `luck_grade` (Strength) — тир бонусного кейса;
  `double_drop` (Strength) — шанс второго бонусного кейса; `coin_mint` (Intellect) —
  бонусные монеты к депозиту; `cashback_master` (Intellect) — скидка на тариф;
  `night_mode` (Agility) — доп. скидка при старте сессии ночью (22:00–07:59);
  `priority_booking` (Agility) — бронь без предоплаты (флаг, платежей пока нет);
- ждёт механики: `investor` (TTL монет — сгорание монет не реализовано, бэклог).

**Экономика денег** (спринт 4):
- **депозиты**: админ оформляет пополнение (`POST /admin/users/:id/deposit`,
  cash/card/blik) — курс 1 zł = 10 монет, бонус `coin_mint`, кейс за депозит от 20 zł,
  ачивка `first_deposit`; история — `GET /me/deposits` (миграция 009, `deposits`);
- **скидка на тариф**: `effective_rate_pln` при старте сессии = базовый тариф минус
  (кэшбек игрока + `cashback_master`), потолок 30% (`effectiveRate`, тест);
- **первый визит дня**: +50 XP фиксировано при первой завершённой сессии дня;
- **сгорание кейсов**: `expires_at` уже ставился при выдаче; сгоревшие не считаются
  в unopened и показываются в UI с таймером «сгорит через N дн».

**Достижения** (`achievements`, сидятся миграцией 005):
- движок проверяет условия после сессии и при регистрации, выдаёт награды
  (очки навыков + кейс), запоминает в `user_achievements`;
- сейчас вычисляются `hours_played` (1/10/100 ч) и `login_count` (первый вход);
- `deposit_count`, `phone_verified`, стрики — ждут своих механик;
- логика условий (`conditionMet`) покрыта тестом и проверена симуляцией.

---

## Что в заглушках (возвращают 501)

Маршруты определены в `backend/cmd/server/main.go`, но логики пока нет:
`/auth/otp/*` (ждёт Twilio).

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
- ~~Два набора роутов~~ — решено в спринте 5: неиспользуемый `internal/api/*` удалён,
  единственный роутер — в `cmd/server/main.go`.

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
├── shell/                   C# PC Shell (.NET 8): Shared + Service + App(WPF) + Tests
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
2. ✅ **Спринт 2 — shell-agent** (`backend/cmd/agent`): запуск программ с ПК по
   allowlist, WS (heartbeat, admin_call), интеграция с гостевым экраном.
   Блокировка экрана по концу сессии перенесена в спринт 3.
3. 🔶 **Спринт 3 — клубы/бронь + lock + Admin MVP**:
   ✅ 3а: lock-экран гостевого ПК (без сессии — блокировка, авто-лок при завершении
   извне, поллинг 10с); клубы (`GET /clubs*`) и бронь (создание с проверкой
   пересечений — `bookingOverlaps` + тест, отмена, список).
   ✅ 3б: Admin MVP — роль admin (миграция 008, role в JWT), `/admin/*`,
   `web/admin.html`: гости (бан), ПК (Shell online), форс-завершение сессий, брони.
4. ✅ **Спринт 4 — экономика вглубь**: double_drop, депозиты (миграция 009) +
   coin_mint, скидка cashback_master, сгорание кейсов в UI/счётчике, +50 XP за
   первый визит дня. Полноценные daily/weekly-квесты — в бэклог.
5. ✅ **Спринт 5 — продакшн-минимум**: logout + отзыв/ротация refresh-токенов
   (revoked_tokens, миграция 010), rate limiting 10 rps, один роутер (internal/api
   удалён), CI на GitHub Actions.
   ✅ **Бэклог**: каталог приложений из админки (миграция 011) — сервер управляет
   и гостевым экраном, и allowlist'ом агента; lock_action агента по session_end.
   ✅ **Бэклог**: ручное начисление XP/кейса из админки (ТЗ 7.1) — миграция 012
   `admin_grants`, причина обязательна, XP идёт через общий `applyXP` (с левел-апами
   и кейсами за уровень), вкладка «📜 Журнал» в админке; при активной сессии игрока
   на его Shell уходит `xp_update`.
   ✅ **Бэклог-финал**: бронь с гостевого экрана (вкладка «📅»: форма + список + отмена);
   night_mode (ночная скидка 22:00–07:59); priority_booking (без предоплаты);
   README актуализирован.
   ✅ **Промышленный PC Shell** (`shell/`, C#/.NET 8, фаза 1): Windows-сервис
   (WS к бэкенду, heartbeat, fail-safe lock при потере связи >2 мин) + WPF-киоск
   (fullscreen WebView2 с `shell.html`, блокировка Win/Alt+Tab/Alt+F4, запуск игр
   по каталогу, XP-оверлей, аварийный выход Ctrl+Alt+Shift+Q). Связь сервис↔киоск —
   Named Pipe. `shell.html` сам определяет киоск (`window.chrome.webview`) и шлёт
   запуск в C#. Тесты (ShellState/PipeProtocol/CatalogAllowlist) + CI на windows-runner.
   Фаза 2 (Shell Launcher, нужен Windows Enterprise) и фаза 3 (античиты, образ ПК) —
   в `shell/README.md`.

**Осталось только требующее твоих аккаунтов/железа:** спринт 6 (OTP/Twilio,
Stripe+BLIK, деплой на VPS), фазы 2–3 PC Shell (Windows Enterprise), валидация рынка.
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
