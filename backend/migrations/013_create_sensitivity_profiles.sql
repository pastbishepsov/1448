-- 013_create_sensitivity_profiles.sql
-- Профиль сенсы игрока: DPI + сенсы по играм. Едет за игроком на любой ПК сети —
-- ключевое отличие 14:48 («твоя настройка прицела всегда с тобой»).

CREATE TABLE sensitivity_profiles (
    user_id    UUID PRIMARY KEY REFERENCES users(id),
    dpi        INTEGER NOT NULL DEFAULT 800,
    games      JSONB NOT NULL DEFAULT '{}',   -- {"cs2": 2.0, "valorant": 0.4, ...}
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
