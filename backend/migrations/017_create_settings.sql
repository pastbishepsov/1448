-- 017: настройки экономики клуба (спринт А5 админ-трека, ADMIN.md).
-- key-value; движок читает через settingInt64 с дефолтом из кода, поэтому
-- пустая таблица ничего не ломает. Сидим дефолты для наглядности в админке.

CREATE TABLE IF NOT EXISTS settings (
    key VARCHAR(64) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO settings (key, value) VALUES
    ('xp_per_min', '10'),
    ('coins_per_min', '2'),
    ('coins_per_pln', '10')
ON CONFLICT (key) DO NOTHING;
