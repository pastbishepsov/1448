-- 005_create_achievements.sql
-- Система достижений

CREATE TYPE achievement_category AS ENUM ('lifetime', 'daily', 'weekly', 'monthly');

CREATE TABLE achievements (
    id                   VARCHAR(64) PRIMARY KEY,     -- 'first_hour', 'win_streak_3', etc.
    category             achievement_category NOT NULL,
    title                VARCHAR(128) NOT NULL,
    description          TEXT,
    condition_type       VARCHAR(64) NOT NULL,         -- 'hours_played', 'wins_streak', 'level_reached', ...
    condition_value      JSONB NOT NULL,               -- Гибкая структура условия
    reward_skillpoints   INTEGER NOT NULL DEFAULT 0,
    reward_case_tier     case_tier,                   -- NULL = нет кейса в награду
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Выданные достижения
CREATE TABLE user_achievements (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        UUID NOT NULL REFERENCES users(id),
    achievement_id VARCHAR(64) NOT NULL REFERENCES achievements(id),
    earned_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    period_key     VARCHAR(16),    -- Для daily/weekly/monthly: '2025-01', '2025-W03'
    UNIQUE(user_id, achievement_id, period_key)
);

CREATE INDEX idx_user_achievements_user ON user_achievements(user_id);
CREATE INDEX idx_user_achievements_earned ON user_achievements(earned_at DESC);

-- Базовые достижения
INSERT INTO achievements (id, category, title, description, condition_type, condition_value, reward_skillpoints, reward_case_tier) VALUES
('first_login', 'lifetime', 'Добро пожаловать', 'Первый вход в систему', 'login_count', '{"min": 1}', 5, 'light'),
('hour_1', 'lifetime', '1 час в клубе', 'Провёл 1 час за компьютером', 'hours_played', '{"min": 1}', 10, 'light'),
('hour_10', 'lifetime', '10 часов', 'Провёл 10 часов за компьютером', 'hours_played', '{"min": 10}', 20, 'medium'),
('hour_100', 'lifetime', 'Сотня часов', 'Провёл 100 часов за компьютером', 'hours_played', '{"min": 100}', 50, 'heavy'),
('first_deposit', 'lifetime', 'Первое пополнение', 'Первое пополнение баланса', 'deposit_count', '{"min": 1}', 10, 'light'),
('phone_verified', 'lifetime', 'Верификация', 'Привязал номер телефона', 'phone_verified', '{"verified": true}', 5, 'titan'),
('daily_visit', 'daily', 'Ежедневный визит', 'Посетил клуб сегодня', 'daily_visit', '{"count": 1}', 3, NULL),
('win_streak_3', 'daily', '3 победы подряд', 'Выиграл 3 матча подряд за день', 'win_streak', '{"min": 3}', 10, 'light'),
('week_streak_7', 'weekly', 'Неделя без пропусков', 'Посещал клуб 7 дней подряд', 'visit_streak', '{"days": 7}', 30, 'heavy');
