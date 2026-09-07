-- 057_incidents.sql
-- К8 (KIOSK.md, решение основателя Р4): журнал проблем для стойки.
--
-- Потерю агента, зависший ПК, свет — автоматикой НЕ компенсируем: админ видит
-- запись («агент пропал в 21:14 на 6 мин») и решает сам — минутами
-- (coin_minutes, тот же запас, что за монеты) или деньгами на кошелёк
-- (wallet_transactions kind=adjust, ref=incident). Компенсация — рост
-- обязательства клуба, не выручка и не выручка со знаком минус (Р7 трека Г).
--
-- auto = true — запись сделал хаб при обрыве агента во время активной сессии;
-- ended_at и minutes у неё проставляются при реконнекте или конце сессии.
-- resolved_at IS NULL — «открытая»: админ ещё не решил (компенсация или
-- закрытие без неё).

CREATE TABLE IF NOT EXISTS incidents (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id         UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    computer_id     UUID REFERENCES computers(id) ON DELETE SET NULL,
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    session_id      UUID REFERENCES sessions(id) ON DELETE SET NULL,
    kind            VARCHAR(16) NOT NULL CHECK (kind IN ('pc_hang', 'agent_lost', 'power', 'other')),
    auto            BOOLEAN NOT NULL DEFAULT FALSE,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ,
    minutes         INTEGER NOT NULL DEFAULT 0 CHECK (minutes >= 0),
    note            TEXT NOT NULL DEFAULT '',
    comp_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (comp_minutes >= 0),
    comp_grosz      BIGINT NOT NULL DEFAULT 0 CHECK (comp_grosz >= 0),
    compensated_at  TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    resolved_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_incidents_open    ON incidents(club_id, created_at DESC) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_incidents_created ON incidents(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_user    ON incidents(user_id);

COMMENT ON TABLE  incidents              IS 'К8 (Р4): журнал проблем с ПК — факт, длительность, компенсация по решению админа';
COMMENT ON COLUMN incidents.auto         IS 'К8-и2: автозапись хаба при обрыве агента во время сессии';
COMMENT ON COLUMN incidents.comp_minutes IS 'компенсация минутами → users.coin_minutes';
COMMENT ON COLUMN incidents.comp_grosz   IS 'компенсация деньгами → кошелёк (wallet_transactions adjust, ref=incident)';
COMMENT ON COLUMN incidents.resolved_at  IS 'NULL — открытая: админ ещё не решил';
