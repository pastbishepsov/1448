-- 004_create_cases.sql
-- Кейсы (система гача-дропа)

CREATE TYPE case_tier AS ENUM ('light', 'medium', 'heavy', 'titan', 'gods');
CREATE TYPE case_source AS ENUM ('achievement', 'daily_visit', 'deposit', 'level_up', 'admin_grant');
CREATE TYPE drop_type AS ENUM ('coins', 'buster');

CREATE TABLE cases (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id),
    club_id     UUID REFERENCES clubs(id),           -- В каком клубе выдан
    tier        case_tier NOT NULL,
    source      case_source NOT NULL,

    -- Статус
    opened_at   TIMESTAMPTZ,                          -- NULL = не открыт
    expires_at  TIMESTAMPTZ NOT NULL,                 -- Дата сгорания (1 месяц бездействия)

    -- Результат открытия
    drop_type   drop_type,                            -- NULL пока не открыт
    drop_amount BIGINT,                               -- Количество coins или % бустера

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cases_user ON cases(user_id);
CREATE INDEX idx_cases_user_unopened ON cases(user_id) WHERE opened_at IS NULL;
CREATE INDEX idx_cases_expires ON cases(expires_at) WHERE opened_at IS NULL;

COMMENT ON TABLE cases IS 'Кейсы нельзя купить за реальные деньги — только за лояльность. Это соответствует EU gambling regulations.';
