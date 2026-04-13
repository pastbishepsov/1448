-- 007_create_bookings.sql
-- Бронирование ПК

CREATE TYPE booking_status AS ENUM ('pending', 'confirmed', 'cancelled', 'completed', 'no_show');

CREATE TABLE bookings (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    computer_id     UUID NOT NULL REFERENCES computers(id),
    club_id         UUID NOT NULL REFERENCES clubs(id),
    status          booking_status NOT NULL DEFAULT 'pending',
    start_time      TIMESTAMPTZ NOT NULL,
    duration_min    INTEGER NOT NULL DEFAULT 60,
    prepaid         BOOLEAN NOT NULL DEFAULT TRUE,  -- FALSE для привилегированных игроков
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bookings_user ON bookings(user_id);
CREATE INDEX idx_bookings_club_time ON bookings(club_id, start_time);
CREATE INDEX idx_bookings_status ON bookings(status);
