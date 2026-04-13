# Архитектура 14:48

## Компоненты

```
Mobile App (Flutter)
       │
       │ HTTPS REST + WS
       ▼
   Go Backend ◄──── WebSocket ────► PC Shell (C#)
       │
   PostgreSQL
       │
     Redis
       │
Admin Panel (React)
```

## Принципы безопасности

1. **Все расчёты только на сервере** — XP, coins, drop rate никогда не вычисляются на клиенте
2. **PC Shell — тонкий клиент** — только отображает данные, не вычисляет их
3. **JWT + Refresh tokens** — access token живёт 15 минут, refresh — 30 дней
4. **Криптографически стойкий random** — для кейсов используется crypto/rand

## Поток сессии

1. Администратор → Admin Panel: запускает сессию для ПК
2. Backend → PC Shell: WebSocket `session_start` с user_id, duration
3. PC Shell: блокирует explorer, запускает наш Shell
4. PC Shell → Backend: WebSocket `session_tick` каждые 60 сек
5. Backend: начисляет XP (100 XP/час базово) и coins
6. По истечении времени: Backend → PC Shell: `session_end`
7. PC Shell: блокирует экран, Backend: завершает сессию
