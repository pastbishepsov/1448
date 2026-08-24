-- 043: страница регистрации (Р9, отдельный чат основателя) — имя и фамилия.
--
-- Поля ОПЦИОНАЛЬНЫ и стираемы гостем (GDPR, как вся анкета Г8): пустое
-- значение в PATCH /me очищает поле, значения в аудит не пишутся.
-- Награда — ОДНА lifetime-ачивка «Представился» (+25 XP), когда заполнены
-- ОБА поля (решение основателя 24.08): ачивку «Анкета закрыта» НЕ расширяем,
-- она остаётся про 5 полей сида 040. Have-map user_achievements гарантирует:
-- стирание и повторное заполнение наград не дублирует.

ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name  VARCHAR(64);

INSERT INTO achievements (id, category, title, description, condition_type, condition_value, reward_skillpoints, reward_case_tier, reward_xp) VALUES
('profile_name', 'lifetime', 'Представился', 'Указал имя и фамилию', 'profile_name', '{"min": 1}', 0, NULL, 25)
ON CONFLICT (id) DO NOTHING;

COMMENT ON COLUMN users.first_name IS 'Анкета/страница регистрации: опционально, стираемо (GDPR)';
COMMENT ON COLUMN users.last_name  IS 'Анкета/страница регистрации: опционально, стираемо (GDPR)';
