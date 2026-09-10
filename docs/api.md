# 14:48 API Reference

Base URL: `http://localhost:8080/api/v1`

Аутентификация: `Authorization: Bearer <access_token>`

## Auth

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/auth/register` | Регистрация |
| POST | `/auth/login` | Вход |
| POST | `/auth/otp/send` | Отправить OTP на телефон |
| POST | `/auth/otp/verify` | Проверить OTP |
| POST | `/auth/refresh` | Обновить токен |
| POST | `/auth/logout` | Выход |
| POST | `/auth/qr` | К6-и1: код для QR на экране входа киоска — `{computer_id}` → `201 {code, expires_in: 120}`; публично, не чаще 6 запросов в минуту с IP (`429 qr_rate_limited`), неизвестный ПК — `404 computer_not_found` |
| GET | `/auth/qr/:code` | К6-и1: поллинг киоска (раз в 2 с) — `{status: "pending", expires_in}`; после подтверждения с телефона — `{status: "approved", access_token, refresh_token, token_type, expires_in, user}` РОВНО ОДИН РАЗ, дальше `404 qr_code_not_found` (сгоревший/чужой код — тоже 404) |
| GET | `/auth/qr/:code/info` | К6-и1: что подтверждаем — `{computer {id, name}, club {id, name}, status, expires_in}` (для карточки «Войти на PC-01 в …?» в PWA) |
| POST | `/me/auth/qr/approve` | К6-и1: телефон (JWT гостя) подтверждает вход на ПК — `{code}` → `{status: "approved", computer, club}`; временный пароль — `409 password_change_required`, бан — `409 banned`, код сгорел/уже подтверждён — 404 |

## Профиль

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/me` | Профиль + XP + coins + таланты |
| PATCH | `/me` | Обновить аватарку / email |
| GET | `/me/cases` | Кейсы: неоткрытые сверху, до 100; `unopened` — COUNT по всем несгоревшим (К3-и5) |
| POST | `/me/cases/:id/open` | Открыть кейс |
| GET | `/me/talents` | Дерево талантов |
| POST | `/me/talents/invest` | Вложить SP в талант |
| GET | `/me/achievements` | Достижения с прогрессом; идущая сессия досчитывается на лету (минуты, активные минуты, визит, часы), `progress {value, target, unit: min\|hours\|days\|count}` (К5) |
| GET | `/me/sessions` | История сессий; 15 мин после завершения без активной сессии — ещё и `receipt` (К3-и6) |
| POST | `/me/sessions/end` | Завершить сессию; в ответе `receipt {wallet_grosz, kitchen_unpaid_pln, streak_days}` — чек на экран «Сессия завершена» (К3-и6) |
| GET / PUT | `/me/sensitivity` | Профиль сенсы: `{dpi, games {cs2: 2.5, …}, saved}` — DPI 100…32000, сенса > 0, до 64 игр; студия шлёт ВСЕ игры из одного cm/360 (С2-и5). Едет за гостем на любой ПК: сервер кладёт его в `session_start` агенту, пересчитав под DPI мыши клуба (`mouse_dpi`, С2-и1: `sens × dpi_профиля / mouse_dpi`, в payload `rescaled_from`). PUT принимает `own_mouse: true` (гость со своей мышью — без пересчёта) и при активной сессии на ПК, который на связи, шлёт агенту `sens_apply` — в ответе `pushed: true` (С2-и4) |
| GET | `/me/economy` | Ранг, таланты и `rates` — ставки из настроек клуба, по ним считает калькулятор (К3-и1); `pause_limit_min` (0 = паузы нет, кнопку не показывать) и `afk_stop_min` — правила паузы и AFK (К4-и4); `mouse_dpi` — DPI мыши клуба, 0 = не задан (С2-и1: студия на клубном ПК запирает поле DPI на нём); `mousepad_cm` — ширина коврика клуба, эталон для замера DPI свайпом, 0 = не задана (С4-и3) |
| POST | `/me/aim-sessions` | Прогон AI-калибратора (С4-и1): тело = `aim_result` калибратора `{sens, dpi, cur, lo, hi, conf: high\|mid\|low, verdict: rec\|cur\|tie\|none, gain_pct, acc1?, rt_ms, mt_ms, tp, mode: quick\|precise\|custom, rounds, ab_rounds, partial, raw?, hz, coach: over\|under\|none, warm, lang, sim}` → `{saved, id, at, cm360}`; cm/360 сервер считает сам из sens×dpi; ошибки `400 bad_sens\|bad_dpi\|bad_range\|bad_conf\|bad_verdict\|bad_gain\|bad_acc1\|bad_ms\|bad_tp\|bad_mode\|bad_rounds\|bad_hz\|bad_coach`; хранится 200 последних на гостя |
| GET | `/me/aim-sessions?limit=10` | История калибровок (новые первыми, до 50) + `warm {cm360, sens, dpi, verdict, at}` — от последнего настоящего полного прогона (A/B подтвердил текущую → её cm/360; sim и частичные мимо; `null` — истории нет) + `stats {count, first_rt, last_rt, best_tp, best_acc1?}` по настоящим полным прогонам |
| GET | `/aim-rating` | Аим-рейтинг клуба (С4-и4): `top` (до 10: `rank, user_id, nickname, avatar_id, level, tp, acc1, rt_ms, at, is_me`) — лучший настоящий полный прогон (`sim=false, partial=0, rounds ≥ 6`) каждого активного игрока по throughput (бит/с), при равенстве — точнее первый выстрел; `me {rank, tp, total}`, `rank 0` — нет полной калибровки |
| GET | `/me/notifications?client=kiosk\|app` | Непрочитанные уведомления (до 50), выдача помечает. С `client=` — своя отметка у киоска и PWA, тост доходит до обоих; без параметра — как раньше, «прочитано вообще» (К3-и2, миграция 055). Тип `achievement_earned {id, title, category, reward_xp, reward_skillpoints, reward_case_tier}` — ачивка выдана, в т.ч. прямо во время сессии (К5-и2). Тип `incident_compensation {kind, minutes, wallet_pln, note}` — админ компенсировал проблему с ПК (К8). Тип `sens_applied {applied [cs2, apex], skipped {valorant: cloud, …}, dry_run, running [cs2]}` — агент подставил сенсу по команде сервера после «Сохранить» в студии (С2-и4) |
| GET | `/me/chat?after=&mark=1` | Переписка со стойкой: без `after` — ПОСЛЕДНИЕ 100, старые сверху; с `after` — доливка (К3-и5) |

## Ошибки (К3-и3)

Всегда `{ "code": "...", "message": "..." }`. `message` — фраза для человека,
клиенты показывают её как есть. Невалидный JSON-запрос — `400` с кодом
`<поле>_<что не так>`: `password_short` («Пароль — минимум 6 символов»),
`nickname_long`, `email_invalid`, `login_required`; нечитаемый JSON —
`invalid_request`. Сбой базы — `500 db_error` с одной фразой, причина только
в логе сервера. Отказ в старте по деньгам — `409 wallet_low`: «Не хватает на
старт: нужно ~5,75 zł на 15 мин, на кошельке 2,61 zł. Пополни у стойки».

## Клубы

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/catalog` | Витрина: игры/приложения/системные/платформы. Публично — без путей на диске, аргументов, `target_raw` и `games_root` (ссылки и протоколы остаются); `?full=1` с токеном агента — всё, без токена — 401 (К4-и3) |
| GET | `/clubs` | Список клубов с геолокацией, счётчиками ПК и тарифами зон `zones[{name, rate_pln}]` (К2-и5) |
| GET | `/clubs/:id` | Детали клуба |
| GET | `/clubs/:id/computers` | Доступные ПК |
| POST | `/clubs/:id/bookings` | Создать бронь |
| DELETE | `/me/bookings/:id` | Отменить бронь |

## Журнал проблем для стойки (К8, admin)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/admin/incidents?open=1` | Записи журнала проблем: `items[{id, kind, kind_name, auto, computer, nickname, started_at, ended_at, minutes, note, comp_minutes, comp_pln, open, admin}]` + `open` (сколько открытых). Без `open=1` админ видит открытые + сегодняшние, владелец — всё (до 200) |
| POST | `/admin/incidents` | `{computer_id?, user_id?, session_id?, kind: pc_hang\|agent_lost\|power\|other, minutes, note, comp_minutes?, comp_pln?}` → `201`. Гость и сессия подставляются с ПК, если там идёт сессия. С компенсацией — сразу закрыта; без — открытая. Компенсация без гостя — `409 incident_no_guest` |
| POST | `/admin/incidents/:id/compensate` | `{comp_minutes?, comp_pln?, note?}` — минуты в запас гостя (`coin_minutes`), деньги на кошелёк (`wallet_transactions` kind=adjust, ref_type=incident); закрывает запись, оживляет сессию, гостю `incident_compensation`. Повторно — `409 incident_compensated` |
| POST | `/admin/incidents/:id/resolve` | `{note?}` — закрыть без компенсации; уже закрытая — `409 incident_resolved` |

Автозапись (К8-и2): хаб при обрыве агента во время оплачиваемой сессии пишет
`kind=agent_lost, auto=true` без компенсации; возврат агента или конец сессии
проставляет `ended_at`/`minutes`; обрыв короче 20 с записью не считается.
Отчёт `/admin/reports/money` отдаёт блок `incidents {period_count, compensated,
comp_minutes, comp_pln, open}` — компенсации отдельной строкой: рост
обязательства клуба, не выручка. Аудит: `incident_create`,
`incident_compensate`, `incident_resolve`.

Отчёт `/admin/reports/guests` (С4-и1) отдаёт блок `aim {sessions, guests,
prev_sessions, delta}` — прогоны AI-калибратора за период и уникальные гости
(синтетические прогоны стенда `sim` не считаются). Настройка `mousepad_cm`
(0…200, `PUT /admin/settings`) — ширина коврика клуба для замера DPI свайпом.

## WebSocket

`WS /api/v1/ws/shell?computer_id=UUID` — с заголовком `X-Agent-Token: <AGENT_TOKEN>`
(или `Authorization: Bearer`, или `?token=`); при заданном на сервере
`AGENT_TOKEN` без верного токена — `401 agent_token_invalid` (К4-и3).

### События (Сервер → Shell)
- `session_start` — начало сессии: `{session_id, started_at}`; если у гостя сохранён профиль сенсы с играми — ещё `sens {dpi, games {cs2: 4, apex: 2, …}, rescaled_from?}`, пересчитанный под DPI мыши клуба (`mouse_dpi`), агент подставляет её в игры на ПК (К7/С2; то же при пересадке на другой ПК)
- `sens_apply` — `{dpi, games, rescaled_from?}`: гость сохранил профиль во время сессии — подставить сейчас; агент отвечает `sens_result` (С2-и4)
- `session_end` — завершение (закрыть игру, очистка между гостями)
- `lock` — пауза/AFK: запереть экран, игру не трогать (`payload.reason: paused|afk`) — К0-и1
- `unlock` — пауза снята
- `state` — ответ на каждый `session_tick`: `{session_active, paused, session_id, reason}` — агент/киоск сверяют с собой и догоняют пропущенные команды (К4-и1)
- `xp_update` — новый XP
- `force_unlock` — принудительная разблокировка
- `timer_sync` — синхронизация таймера

### События (Shell → Сервер)
- `session_tick` — heartbeat каждые 60 сек
- `admin_call` — вызов администратора
- `sens_result` — `{applied, skipped, dry_run, running}` в ответ на `sens_apply`; сервер кладёт гостю уведомление `sens_applied` (С2-и4)
- `shell_ready` — Shell готов к работе

### Локальный HTTP агента (`127.0.0.1:1448`, для гостевого экрана)

Полный контракт — `docs/АГЕНТ-ЗАПУСК-ИГР.md`. Коротко: `GET /ping` (+ `notify`, `sens`, `sens_games`, `sens_mode`, `sens_current {cs2 {sens, res_w, res_h, aspect}, apex {sens}}` — С2-и2), `POST /launch`, `GET /apps-status`, `POST /notify`, `POST /sens-apply {dpi, games}` → `{applied, skipped {game: cloud|unsupported|no_config|disabled|error}, dry_run, running, supported, cs2_mode}` — curl-проверка на живом ПК (студия с С2 идёт через сервер: PUT `/me/sensitivity` → WS `sens_apply`; 409 `sens_disabled`, если выключено в `agent.json`).

**AI-калибратор `web/aim.html`** (не API сервера, мост со студией шелла через
`postMessage`, С0/С3 SENS.md): открывается iframe'ом
`aim.html?embed=1&dpi=&sens=&lang=ru|en|pl&aspect=` (`sens` — в единицах CS2;
`lang` — язык словарей `AIM_RU/EN/PL`, без него — `1448_lang` из localStorage,
дефолт `ru`; `aspect` — соотношение сторон CS2 с этого ПК из `sens_current`).
Наружу: `{type:'aim_result', sens, dpi, cm360, lo, hi, conf: high|mid|low,
verdict: rec|cur|tie|none, gain_pct, rt_ms, mt_ms, tp, cur, cur_cm, rounds,
ab_rounds, sim, acc1, raw, hz, partial, mode, lang, coach: over|under|none,
warm}`, `{type:'aim_save'}`, `{type:'aim_close'}`, `{type:'aim_dpi', dpi,
dpi_raw, counts, cm, raw}` (замер DPI свайпом, С4-и3: открывается как
`aim.html?embed=1&measure=1&pad_cm=&lang=`). `?warm_cm=N` — тёплый старт от
прошлого оптимума (С4-и1: шелл берёт `warm.cm360` из `GET /me/aim-sessions`;
поиск вокруг него, диапазон ×1.6, раундов ¾ но ≥ 6). `conf` — код, не слово
(С3-и1). Игры студии
(`SENS_GAMES` в shell.html): cs2, valorant, apex, ow2, cod, quake (Quake
Champions), deadlock, cs16, quake3, l4d2, half_life — все с `catalog_ids` для
сопоставления с `GET /catalog`; профиль `PUT /me/sensitivity` несёт их все из
одного cm/360, агент незнакомые ему отдаёт как `skipped: unsupported`.

## Анкета профиля (Г8) — контракт для страницы регистрации

Страница регистрации (Р9) — `web/register.html`, раздаётся на `GET /register`
(same-origin с `/app`: токены в localStorage общие, после реги PWA уже
«вошло»). После `POST /auth/register` она получает `access_token` и добирает
анкету обычным `PATCH /me` — тем же эндпоинтом пользуются шелл и PWA.
E-mail обязателен только на самой странице (быстрая рега «ник+пароль» в
шелле и PWA не меняется, сервер не ужесточён — решение основателя 24.08).

`PATCH /me` — все поля опциональны, отправляй только то, что меняешь.
Пустая строка (или пустой список) **очищает** поле (GDPR).

```json
{
  "nickname": "kotik",             // 3–32 символа, без кавычек/скобок
  "avatar_id": 3,                  // 1–12
  "first_name": "Jan",             // 043: 1–64 буквы (пробел/дефис/апостроф); "" = стереть
  "last_name": "Kowalski",         // как first_name
  "phone": "+48600000000",         // 043: E.164 (+ и 6–15 цифр), уникален; "" = стереть
  "birth_date": "2008-03-15",      // ГГГГ-ММ-ДД; возраст 6–100; "" = стереть
  "discord": "@kotik_1448",        // ведущий @ срезается; 2–64, без пробелов
  "telegram": "kot1448",           // как discord
  "source": "друг привёл",         // «откуда узнал», до 32 символов
  "favorite_games": ["valorant"],  // до 3 id из GET /catalog (category=game)
  "language": "pl"                 // пресет ru|en|pl, едет за гостем (без ачивки)
}
```

Ответ — обновлённый объект пользователя (как в `GET /me`), поля анкеты в нём:
`first_name`, `last_name`, `birth_date`, `discord`, `telegram`, `source`,
`favorite_games` (массив id).

Ошибки: `400 bad_first_name | bad_last_name | bad_phone | bad_birth_date |
bad_discord | bad_telegram | bad_source | bad_favorites | nickname_*`,
`409 nickname_taken | phone_taken | taken`. Телефон здесь — только формат и
уникальность (данные для связи, БЕЗ ачивки): награда за подтверждённый номер
уедет в Г10 вместе с OTP.

Награды начисляет сервер сам (lifetime-ачивки, повторно не выдаются):
+25 XP за каждое анкетное поле; имя и фамилия считаются ОДНИМ полем —
ачивка «Представился» (+25 XP) за оба сразу (043); «Анкета закрыта»
(5 полей сида 040 — ДР, игры, Discord, Telegram, источник; имя/фамилия
в неё НЕ входят) — Light-кейс + 1 SP.
Если у гостя сегодня день рождения (по дате из анкеты), при входе или старте
сессии клуб дарит кейс — тир задаёт владелец настройкой `birthday_case_tier`
(0 = выкл). Список игр для выбора: `GET /catalog` → `games[] {id, name}`.
