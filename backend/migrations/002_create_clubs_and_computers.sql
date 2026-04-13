-- 002_create_clubs_and_computers.sql
-- Клубы и компьютеры

CREATE TABLE clubs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(128) NOT NULL,
    address         TEXT NOT NULL,
    latitude        DECIMAL(9,6),
    longitude       DECIMAL(9,6),
    phone           VARCHAR(20),
    telegram        VARCHAR(64),
    instagram       VARCHAR(64),
    working_hours   JSONB,                -- {"mon":{"open":"10:00","close":"23:00"}, ...}
    base_rate_pln   DECIMAL(8,2) NOT NULL DEFAULT 23.00,
    rtp_modifier    DECIMAL(4,2) NOT NULL DEFAULT 1.00, -- Множитель RTP кейсов (0.8 - 1.2)
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE computer_status AS ENUM ('available', 'in_session', 'maintenance', 'reserved');

CREATE TABLE computers (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id     UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    name        VARCHAR(32) NOT NULL,   -- "ПК-01", "VIP-03"
    zone        VARCHAR(32),             -- "VIP", "Standard"
    status      computer_status NOT NULL DEFAULT 'available',
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_computers_club ON computers(club_id);
CREATE INDEX idx_computers_status ON computers(status);
