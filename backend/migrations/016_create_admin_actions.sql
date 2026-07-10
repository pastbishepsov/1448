-- 016: журнал действий администраторов (спринт А4 админ-трека, ADMIN.md).
-- Ручные начисления и депозиты уже журналируются своими таблицами
-- (admin_grants, deposits) — здесь остальное: баны/разбаны, форс-завершения
-- сессий, ремонт ПК, брони за гостя, правки каталога.

CREATE TABLE IF NOT EXISTS admin_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES users(id),
    action VARCHAR(32) NOT NULL,
    target_user_id UUID REFERENCES users(id),
    details TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_admin_actions_created_at ON admin_actions (created_at DESC);
