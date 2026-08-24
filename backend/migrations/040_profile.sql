-- 040: профиль гостя — данные и награды (трек Г, спринт Г8; Р4/Р9 GUEST.md).
--
-- Все поля ОПЦИОНАЛЬНЫ и стираемы гостем (GDPR): пустое значение в PATCH /me
-- очищает поле; значения полей в аудит не пишутся. Награды за заполнение —
-- lifetime-ачивки (+25 XP за поле, «Анкета закрыта» — Light + 1 sp), выдаются
-- один раз навсегда через обычный have-map user_achievements: стирание и
-- повторное заполнение наград НЕ дублирует.
--
-- ДР-подарок (Г8-и4): birthday_gift_year — год последней выдачи (раз в год,
-- CAS). Дарим КЕЙС по настройке владельца (0 = выкл); вариант «монеты»
-- сознательно отложен: прямое начисление мимо журналов разъехалось бы с
-- отчётом эмиссии В4-2 — ждёт журнала монет (бэклог трека В).

ALTER TABLE users ADD COLUMN IF NOT EXISTS birth_date DATE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS discord VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS source VARCHAR(32);
ALTER TABLE users ADD COLUMN IF NOT EXISTS favorite_games JSONB NOT NULL DEFAULT '[]';
ALTER TABLE users ADD COLUMN IF NOT EXISTS birthday_gift_year INT NOT NULL DEFAULT 0;

-- кейс за день рождения — новый источник в enum (значение нельзя использовать
-- в этой же миграции — и не используем)
ALTER TYPE case_source ADD VALUE IF NOT EXISTS 'birthday';

-- Ачивки за данные (Р4): XP без кейсов, кейс только за полную анкету
INSERT INTO achievements (id, category, title, description, condition_type, condition_value, reward_skillpoints, reward_case_tier, reward_xp) VALUES
('profile_birth',    'lifetime', 'Именинник в базе', 'Указал дату рождения — подарок не промахнётся',  'profile_birth',    '{"min": 1}', 0, NULL,    25),
('profile_games',    'lifetime', 'Мой сет',          'Выбрал любимые игры',                            'profile_games',    '{"min": 1}', 0, NULL,    25),
('profile_discord',  'lifetime', 'На связи в Discord', 'Указал Discord',                               'profile_discord',  '{"min": 1}', 0, NULL,    25),
('profile_telegram', 'lifetime', 'На связи в Telegram', 'Указал Telegram',                             'profile_telegram', '{"min": 1}', 0, NULL,    25),
('profile_source',   'lifetime', 'Сарафан',          'Рассказал, откуда узнал о клубе',                'profile_source',   '{"min": 1}', 0, NULL,    25),
('profile_complete', 'lifetime', 'Анкета закрыта',   'Заполнил профиль целиком',                       'profile_complete', '{"min": 1}', 1, 'light', 0)
ON CONFLICT (id) DO NOTHING;

-- настройка владельца: тир ДР-кейса (0=выкл, 1=light … 5=gods)
INSERT INTO settings (key, value) VALUES ('birthday_case_tier', '1') ON CONFLICT (key) DO NOTHING;

COMMENT ON COLUMN users.birth_date IS 'Г8: опционально, стираемо (GDPR); возраст 6–100 на входе';
COMMENT ON COLUMN users.favorite_games IS 'Г8: до 3 id из catalog_apps (category=game); шелл поднимает их в топ витрины';
COMMENT ON COLUMN users.birthday_gift_year IS 'Г8-и4: год последней выдачи ДР-подарка (раз в год, CAS)';
