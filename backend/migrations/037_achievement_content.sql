-- 037: контент периодических достижений + XP-награды (трек Г, спринт Г6).
--
-- Движок периодов (Г5, резет 14:48) получает полный набор целей из плана
-- GUEST.md (III.4). Решения:
--   - награды за еду и «первые шаги» — XP, НИКОГДА не кейс (Р5: «деньги →
--     случайная награда» — лутбокс-связка, от которой ушли в RESEARCH §4);
--     для этого achievements получают reward_xp (движок ведёт XP через общий
--     applyXP — с левел-апами и кейсами за уровень);
--   - week_streak_7 приводится к решению основателя «кейс МАКС. уровня +
--     2 skillpoints»: Titan + 2 sp (God's еженедельно ломает экономику:
--     средний дроп God's ~12500 монет ≈ 625 zł времени; Titan ~3000 ≈ 150 zł
--     на самых лояльных — финальные тиры подтверждает владелец, вопрос №7
--     GUEST.md; правка тира = одна строка здесь). UPDATE безусловный —
--     ачивки владелец из UI не правит, значение идемпотентно;
--   - «просидел» — по АКТИВНЫМ минутам (Г2): AFK не фармит;
--   - «Ночная смена» — ночной счётчик суток (user_progress.night_sessions).

ALTER TABLE achievements ADD COLUMN IF NOT EXISTS reward_xp INT NOT NULL DEFAULT 0;
ALTER TABLE user_progress ADD COLUMN IF NOT EXISTS night_sessions INT NOT NULL DEFAULT 0;

-- Решение основателя: недельный стрик = макс. кейс + 2 очка (было heavy + 30).
UPDATE achievements SET reward_case_tier = 'titan', reward_skillpoints = 2
WHERE id = 'week_streak_7';

INSERT INTO achievements (id, category, title, description, condition_type, condition_value, reward_skillpoints, reward_case_tier, reward_xp) VALUES
-- Daily: шкала отсиженного (активные минуты — анти-фарм)
('warmup_60',    'daily',   'Разогрев',        'Час активной игры за сутки',            'active_minutes_today', '{"min": 60}',   0, 'light',  0),
('flow_240',     'daily',   'На волне',        '4 активных часа за сутки',              'active_minutes_today', '{"min": 240}',  0, 'medium', 0),
('marathon_480', 'daily',   'Марафон',         '8 активных часов за сутки',             'active_minutes_today', '{"min": 480}',  0, 'heavy',  0),
('snack_daily',  'daily',   'Подкрепился',     'Заказал что-то на кухне сегодня',       'kitchen_today',        '{"min": 1}',    0, NULL,     50),
('night_owl',    'daily',   'Ночная смена',    'Начал сессию ночью (22:00–07:59)',      'night_session_today',  '{"min": 1}',    0, NULL,     50),
-- Weekly
('week_5of7',    'weekly',  'Пять из семи',    'Посетил клуб 5 дней за неделю',         'visits_week',          '{"min": 5}',    1, 'medium', 0),
('week_10h',     'weekly',  'Десятка',         '10 активных часов за неделю',           'active_minutes_week',  '{"min": 600}',  0, 'medium', 0),
-- Monthly
('month_regular','monthly', 'Житель клуба',    '20 визитов за месяц',                   'visits_month',         '{"min": 20}',   3, 'titan',  0),
('month_100h',   'monthly', 'Сотня',           '100 активных часов за месяц',           'active_minutes_month', '{"min": 6000}', 0, 'titan',  0),
-- Lifetime: добивка
('hour_500',     'lifetime','Пятьсот',         'Провёл 500 часов за компьютером',       'hours_played',         '{"min": 500}',  75, 'titan',  0),
('hour_1000',    'lifetime','Тысяча',          'Провёл 1000 часов за компьютером',      'hours_played',         '{"min": 1000}', 100,'gods',   0),
('first_booking','lifetime','Первая бронь',    'Забронировал ПК впервые',               'bookings_count',       '{"min": 1}',    0, NULL,     50),
('first_kitchen','lifetime','Первый заказ',    'Первый заказ на кухне',                 'kitchen_today',        '{"min": 1}',    0, NULL,     50)
ON CONFLICT (id) DO NOTHING;

COMMENT ON COLUMN achievements.reward_xp IS 'Г6: XP-награда (через общий applyXP); для еды и «первых шагов» — вместо кейса (Р5)';
