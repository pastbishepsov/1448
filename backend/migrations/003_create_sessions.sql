-- 003_create_sessions.sql
-- Игровые сессии

CREATE TYPE session_status AS ENUM ('active', 'completed', 'cancelled');

CREATE TABLE sessions (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL REFERENCES users(id),
    computer_id      UUID NOT NULL REFERENCES computers(id),
    club_id          UUID NOT NULL REFERENCES clubs(id),
    status           session_status NOT NULL DEFAULT 'active',

    started_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at         TIMESTAMPTZ,
    minutes_total    INTEGER NOT NULL DEFAULT 0,  -- Оплаченное время
    minutes_used     INTEGER NOT NULL DEFAULT 0,  -- Фактически использованное

    -- Финансы
    base_rate_pln    DECIMAL(8,2) NOT NULL,
    effective_rate_pln DECIMAL(8,2) NOT NULL,     -- С учётом бонусов

    -- Начисления за сессию
    xp_earned        BIGINT NOT NULL DEFAULT 0,
    coins_earned     BIGINT NOT NULL DEFAULT 0,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_computer ON sessions(computer_id);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_started ON sessions(started_at DESC);

COMMENT ON TABLE sessions IS 'Игровые сессии. Все расчёты XP и coins происходят только на сервере.';
