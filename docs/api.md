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

## Профиль

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/me` | Профиль + XP + coins + таланты |
| PATCH | `/me` | Обновить аватарку / email |
| GET | `/me/cases` | Список кейсов |
| POST | `/me/cases/:id/open` | Открыть кейс |
| GET | `/me/talents` | Дерево талантов |
| POST | `/me/talents/invest` | Вложить SP в талант |
| GET | `/me/achievements` | Достижения |
| GET | `/me/sessions` | История сессий |

## Клубы

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/clubs` | Список клубов с геолокацией |
| GET | `/clubs/:id` | Детали клуба |
| GET | `/clubs/:id/computers` | Доступные ПК |
| POST | `/clubs/:id/bookings` | Создать бронь |
| DELETE | `/me/bookings/:id` | Отменить бронь |

## WebSocket

`WS /api/v1/ws/shell?computer_id=UUID&token=JWT`

### События (Сервер → Shell)
- `session_start` — начало сессии
- `session_end` — завершение
- `xp_update` — новый XP
- `force_unlock` — принудительная разблокировка
- `timer_sync` — синхронизация таймера

### События (Shell → Сервер)
- `session_tick` — heartbeat каждые 60 сек
- `admin_call` — вызов администратора
- `shell_ready` — Shell готов к работе

## Анкета профиля (Г8) — контракт для страницы регистрации

Страница регистрации (Р9, делается отдельно) после `POST /auth/register`
получает `access_token` и добирает анкету обычным `PATCH /me` — тем же
эндпоинтом пользуются шелл и PWA.

`PATCH /me` — все поля опциональны, отправляй только то, что меняешь.
Пустая строка (или пустой список) **очищает** поле (GDPR).

```json
{
  "nickname": "kotik",             // 3–32 символа, без кавычек/скобок
  "avatar_id": 3,                  // 1–12
  "birth_date": "2008-03-15",      // ГГГГ-ММ-ДД; возраст 6–100; "" = стереть
  "discord": "@kotik_1448",        // ведущий @ срезается; 2–64, без пробелов
  "telegram": "kot1448",           // как discord
  "source": "друг привёл",         // «откуда узнал», до 32 символов
  "favorite_games": ["valorant"]   // до 3 id из GET /catalog (category=game)
}
```

Ответ — обновлённый объект пользователя (как в `GET /me`), поля анкеты в нём:
`birth_date`, `discord`, `telegram`, `source`, `favorite_games` (массив id).

Ошибки: `400 bad_birth_date | bad_discord | bad_telegram | bad_source |
bad_favorites | nickname_*`, `409 nickname_taken`.

Награды начисляет сервер сам (lifetime-ачивки, повторно не выдаются):
+25 XP за каждое поле, «Анкета закрыта» (все 5) — Light-кейс + 1 SP.
Если у гостя сегодня день рождения (по дате из анкеты), при входе или старте
сессии клуб дарит кейс — тир задаёт владелец настройкой `birthday_case_tier`
(0 = выкл). Список игр для выбора: `GET /catalog` → `games[] {id, name}`.
