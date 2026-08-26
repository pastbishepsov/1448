#!/bin/sh
set -e

echo "14:48 Backend стартует..."
echo "Ожидание PostgreSQL..."

# Ждём по тому же DATABASE_URL, которым потом подключается сервер: раньше
# ожидание шло по POSTGRES_HOST/PORT/USER, и если их не задали (Railway даёт
# DATABASE_URL и PGHOST), pg_isready уходил на локальный сокет и висел вечно —
# сервер не стартовал, healthcheck падал по таймауту (ревью 26.08).
tries=0
until psql "$DATABASE_URL" -c 'SELECT 1' >/dev/null 2>&1; do
  tries=$((tries + 1))
  if [ "$tries" -ge 120 ]; then
    echo "ОШИБКА: PostgreSQL не отвечает по DATABASE_URL за 120 секунд."
    psql "$DATABASE_URL" -c 'SELECT 1' || true
    exit 1
  fi
  sleep 1
done

# ── Журнал миграций (ревью 26.08) ────────────────────────────────────────────
# Раньше КАЖДЫЙ файл выполнялся при КАЖДОМ старте, а ошибки глушились
# («2>/dev/null || true»). Отсюда два разных бедствия:
#   1. Провалившаяся миграция выглядела как успешная: сервер поднимался с
#      недостроенной схемой, и это всплывало 500-кой на живом госте.
#   2. Миграции с данными переигрывались и ТИХО ОТКАТЫВАЛИ правки владельца:
#      014 возвращала удалённые из каталога приложения и снова выключала
#      YouTube, 027 пересоздавала удалённую зону «Общая» и переприсваивала ПК.
# Журнал делает каждый файл ровно-однократным, а ошибки — фатальными.
psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -q -c "
  CREATE TABLE IF NOT EXISTS schema_migrations (
    filename   TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );"

applied_count=$(psql "$DATABASE_URL" -tAc "SELECT count(*) FROM schema_migrations;")
legacy_db=$(psql "$DATABASE_URL" -tAc "SELECT to_regclass('public.users') IS NOT NULL;")

# Первый старт на УЖЕ мигрированной базе (демо-стенд, клубный пилот до
# обновления): журнала нет, а схема есть. Прогоняем терпимо — ровно как делал
# старый цикл — и заполняем журнал. Со следующего старта режим строгий.
if [ "$applied_count" = "0" ] && [ "$legacy_db" = "t" ]; then
  echo "Журнал миграций пуст, а схема уже существует — разовый переходный прогон."
  MIGRATE_MODE="tolerant"
else
  MIGRATE_MODE="strict"
fi

echo "Применяю миграции (режим: $MIGRATE_MODE)..."
for f in /app/migrations/*.sql; do
  name=$(basename "$f")
  done_already=$(psql "$DATABASE_URL" -tAc \
    "SELECT 1 FROM schema_migrations WHERE filename = '$name';")
  if [ "$done_already" = "1" ]; then
    continue
  fi

  echo "  → $name"
  if psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -f "$f"; then
    psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -q -c \
      "INSERT INTO schema_migrations (filename) VALUES ('$name')
       ON CONFLICT (filename) DO NOTHING;"
  elif [ "$MIGRATE_MODE" = "tolerant" ]; then
    echo "    (переходный прогон: уже применена ранее — отмечаю в журнале)"
    psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -q -c \
      "INSERT INTO schema_migrations (filename) VALUES ('$name')
       ON CONFLICT (filename) DO NOTHING;"
  else
    echo "ОШИБКА: миграция $name не применилась — останавливаюсь."
    echo "Схема осталась недостроенной; поднимать сервер в таком виде нельзя."
    exit 1
  fi
done

echo "Миграции применены. Запускаю сервер..."
exec ./server
