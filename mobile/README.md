# 14:48 — Mobile App (Flutter)

> ⛔ **Заморожен (2026-07-18) до решения о сторах после пилота — не удалять.**
> Активный мобильный трек — гостевое **PWA** `web/app.html` (спринты М0–М5),
> точка входа — `MOBILE.md` в корне репо. Если после пилота пойдём в сторы,
> кандидаты — TWA/Capacitor поверх того же PWA либо этот Flutter-скелет.

iOS + Android приложение для игроков.

## Стек
- **Flutter** + Dart
- **Riverpod** (state management)
- **Dio** (HTTP клиент)
- **WebSocket** (real-time обновления XP)

## Экраны
- `Home` — профиль, XP-шкала, ежедневные квесты
- `Cases` — кейсы Light/Medium/Heavy/Titan/God's
- `Talents` — дерево Strength/Agility/Intellect
- `Profile` — статистика, ачивки
- `Clubs` — карта клубов, бронирование

## Референсные репозитории
- Бойлерплейт: `github.com/danvick/flutter_boilerplate`
- Геймификация: `github.com/nalugala-vc/HabitQuest`
- OTP авторизация: `github.com/Danitilahun/flutter-node-otp-phone-number-verification`
