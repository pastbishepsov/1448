-- 012_create_admin_grants.sql
-- Журнал ручных начислений администратора (ТЗ 7.1: логируется, видно владельцу).

CREATE TABLE admin_grants (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id),
    admin_id   UUID NOT NULL REFERENCES users(id),
    grant_type VARCHAR(8) NOT NULL CHECK (grant_type IN ('xp', 'case')),
    amount     BIGINT,                -- для xp
    case_tier  case_tier,             -- для case
    reason     TEXT NOT NULL,         -- причина обязательна
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_grants_user ON admin_grants(user_id);
CREATE INDEX idx_admin_grants_created ON admin_grants(created_at);
