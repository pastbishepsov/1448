#!/bin/sh
set -e

echo "14:48 Backend стартует..."
echo "Ожидание PostgreSQL..."

until pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER"; do
  sleep 1
done

echo "PostgreSQL готов. Применяю миграции..."
for f in /app/migrations/*.sql; do
  echo "  → $f"
  psql "$DATABASE_URL" -f "$f" 2>/dev/null || true
done

echo "Миграции применены. Запускаю сервер..."
exec ./server
