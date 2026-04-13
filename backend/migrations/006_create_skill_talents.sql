-- 006_create_skill_talents.sql
-- Дерево талантов (Strength / Agility / Intellect)

CREATE TYPE talent_branch AS ENUM ('strength', 'agility', 'intellect');

CREATE TABLE skill_talents (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    branch          talent_branch NOT NULL,
    talent_id       VARCHAR(64) NOT NULL,   -- 'case_hunter', 'xp_boost', 'cashback_master', ...
    current_level   INTEGER NOT NULL DEFAULT 0,
    max_level       INTEGER NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, talent_id)
);

CREATE INDEX idx_skill_talents_user ON skill_talents(user_id);

-- Таблица определений талантов (конфигурация)
CREATE TABLE talent_definitions (
    id              VARCHAR(64) PRIMARY KEY,
    branch          talent_branch NOT NULL,
    name            VARCHAR(128) NOT NULL,
    description     TEXT NOT NULL,
    max_level       INTEGER NOT NULL DEFAULT 5,
    effect_type     VARCHAR(64) NOT NULL,   -- 'case_chance', 'xp_multiplier', 'cashback_pct', ...
    effect_per_level DECIMAL(8,4) NOT NULL, -- Значение эффекта за 1 уровень
    min_user_level  INTEGER NOT NULL DEFAULT 1,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE
);

INSERT INTO talent_definitions (id, branch, name, description, max_level, effect_type, effect_per_level, min_user_level) VALUES
-- Strength
('case_hunter',     'strength', 'Охотник за кейсами', '+15% шанс выпадения кейса за уровень', 5, 'case_drop_chance', 0.15, 1),
('luck_grade',      'strength', 'Градация везения', 'Шанс Heavy+ кейса ×1.5 за уровень', 5, 'case_tier_boost', 0.10, 5),
('double_drop',     'strength', 'Двойной дроп', 'Шанс 2 кейсов одновременно', 7, 'double_case_chance', 0.03, 15),
-- Agility
('xp_boost',        'agility', 'Разгон опыта', '+10% XP за каждый час игры', 5, 'xp_multiplier', 0.10, 1),
('night_mode',      'agility', 'Ночной режим', '-5% к ночному тарифу за уровень', 5, 'night_discount', 0.05, 3),
('priority_booking','agility', 'Приоритет брони', 'Бронь без предоплаты', 1, 'free_booking', 1.00, 20),
-- Intellect
('coin_mint',       'intellect', 'Монетный двор', '+5% coins при пополнении баланса', 5, 'deposit_bonus', 0.05, 1),
('cashback_master', 'intellect', 'Кэшбек-мастер', '+2% к базовому кэшбеку за уровень', 5, 'cashback_pct', 0.02, 1),
('investor',        'intellect', 'Инвестор', 'Coins не сгорают 2 месяца вместо 1', 1, 'coins_ttl_bonus', 30.00, 25);
