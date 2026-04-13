-- 001_create_users.sql
-- Таблица пользователей (игроки)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE user_status AS ENUM ('active', 'banned', 'suspended');

CREATE TABLE users (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nickname              VARCHAR(32) NOT NULL UNIQUE,
    phone                 VARCHAR(20) UNIQUE,
    email                 VARCHAR(255) UNIQUE,
    password_hash         VARCHAR(255) NOT NULL,
    status                user_status NOT NULL DEFAULT 'active',

    -- Прогрессия
    level                 INTEGER NOT NULL DEFAULT 1,
    xp_current            BIGINT NOT NULL DEFAULT 0,
    xp_total              BIGINT NOT NULL DEFAULT 0,
    coins_balance         BIGINT NOT NULL DEFAULT 0,
    skillpoints_available INTEGER NOT NULL DEFAULT 0,
    payment_increase_pct  DECIMAL(5,2) NOT NULL DEFAULT 0.00,

    -- Метаданные
    avatar_id             INTEGER NOT NULL DEFAULT 1,
    registered_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_active_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Индексы
CREATE INDEX idx_users_phone ON users(phone);
CREATE INDEX idx_users_level ON users(level DESC);
CREATE INDEX idx_users_xp_total ON users(xp_total DESC);
CREATE INDEX idx_users_last_active ON users(last_active_at DESC);

COMMENT ON TABLE users IS 'Аккаунты игроков компьютерного клуба';
COMMENT ON COLUMN users.payment_increase_pct IS 'Итоговый % кэшбека с учётом уровня и талантов';
COMMENT ON COLUMN users.skillpoints_available IS 'Нераспределённые очки навыков для прокачки талантов';
