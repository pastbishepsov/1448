# 14:48

**Геймифицированная экосистема управления компьютерными клубами**

> Превращаем «прокат железа» в живую RPG-игру с бесконечным циклом удержания клиента.

---

## Что это

**14:48** — SaaS-платформа для компьютерных клубов, которая строится вокруг удержания клиента через игровые механики: XP, уровни, таланты, кейсы, достижения.

Каждый конкурент (SmartShell, SENET, LANGAME) делает инструмент контроля. Мы делаем продолжение игры.

---

## Архитектура — 4 компонента

```
14:48/
├── backend/     # Go — REST API, WebSocket, бизнес-логика, PostgreSQL + Redis
├── mobile/      # Flutter — iOS + Android приложение для игроков
├── shell/       # C# / WinUI 3 — кастомный PC Shell для Windows
└── admin/       # React — Admin Panel + Owner Stats (веб)
```

---

## Быстрый старт (Docker)

### Требования
- Docker Desktop
- Git

### 1. Клонировать
```bash
git clone https://github.com/YOUR_USERNAME/1448.git
cd 1448
```

### 2. Настроить окружение
```bash
cp .env.example .env
```

### 3. Инициализировать зависимости Go
```bash
cd backend && go mod tidy && cd ..
```

### 4. Запустить
```bash
docker compose up -d
```

| Сервис | URL |
|--------|-----|
| Go API | http://localhost:8080 |
| pgAdmin | http://localhost:5050 |
| Redis Commander | http://localhost:8081 |

### 5. Применить миграции
```bash
docker compose exec backend ./migrate up
```

### 6. Проверка
```bash
curl http://localhost:8080/health
# {"status":"ok","version":"0.1.0"}
```

---

## Разработка

### Backend (Go)
```bash
cd backend && go mod download && go run cmd/server/main.go
```

### Mobile (Flutter)
```bash
cd mobile && flutter pub get && flutter run
```

### Admin Panel (React)
```bash
cd admin && npm install && npm run dev
```

### PC Shell (C#)
Открыть `shell/1448Shell.sln` в Visual Studio 2022.

---

## Стек

| Компонент | Технологии |
|-----------|------------|
| Backend | Go 1.22, Gin, GORM, PostgreSQL, Redis, WebSocket |
| Mobile | Flutter 3.x, Dart, Dio, Riverpod |
| PC Shell | C# .NET 8, WinUI 3, Windows App SDK |
| Admin Panel | React 19, Vite, TypeScript, shadcn/ui |
| Инфраструктура | Docker, GitHub Actions, PostgreSQL 16, Redis 7 |

---

## Документация

- `docs/ТЗ_14-48_v1.1.docx` — полное ТЗ на 32 страницы
- `docs/api.md` — API Reference
- `docs/architecture.md` — архитектура и схемы

---

## Открытые позиции

Проект ищет разработчиков на equity-основе:
- Go Backend Developer
- C# / WinUI 3 Developer
- Flutter Developer
- React Developer

---

© 2025 Кочергин Е. А. Все права защищены.
