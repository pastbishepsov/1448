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
