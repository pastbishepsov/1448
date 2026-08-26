-- 053_review_invariants.sql
-- Ревью кода 26.08: инварианты, которые до сих пор держались только проверками
-- в Go — а они делались ВНЕ транзакции и потому не держали ничего.
-- Все операторы идемпотентны: entrypoint гоняет миграции при каждом старте.

-- 1. Одна активная сессия на ПК.
--    Проверка статуса ПК шла до транзакции: гость с киоска и посадка админом
--    успевали создать две активные сессии на одной машине, обе тарифицировались.
--    Код теперь занимает ПК условным UPDATE, индекс — вторая линия обороны.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sessions_active_computer
    ON sessions (computer_id) WHERE status = 'active';

-- 2. Одна активная сессия на гостя.
--    Двойной тап «Начать» открывал две сессии на разных ПК, и биллинг честно
--    списывал за обе — двойной тариф с кошелька.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sessions_active_user
    ON sessions (user_id) WHERE status = 'active';

-- 3. Lifetime-ачивки не должны выдаваться дважды.
--    UNIQUE(user_id, achievement_id, period_key) не работает, когда period_key
--    IS NULL: в Postgres NULL <> NULL, строки считаются разными. Из-за этого
--    параллельные проверки выдавали одну ачивку дважды — двойные очки навыков
--    и два кейса.
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_ach_lifetime
    ON user_achievements (user_id, achievement_id) WHERE period_key IS NULL;

-- 4. Горячие выборки: активные сессии по ПК и по гостю дергаются на каждом
--    тике биллинга и на каждом открытии зала.
CREATE INDEX IF NOT EXISTS idx_sessions_status_computer
    ON sessions (status, computer_id);
