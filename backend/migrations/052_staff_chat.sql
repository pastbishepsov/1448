-- 052: канал админ ↔ владелец (спринт Е6-и1, OPERATOR.md; решение Р7).
--
-- «Чат в обе стороны»: гость ↔ админ был с Б2–Б3, а у самого админа спросить
-- некого. Ночью, когда владелец спит, вопрос «гость требует вернуть деньги за
-- пакет, что делать» упирается в тишину — и админ решает наугад, а разбор
-- начинается утром с фразы «а почему ты…».
--
-- ОТДЕЛЬНАЯ таблица, а не флаг в chat_messages. Гостевой чат адресный (гость
-- или машина), этот — общий канал клуба; смешать их значило бы переписать все
-- выборки гостевого чата под «а это не служебное сообщение?» — и однажды
-- показать гостю то, что писали про него.
--
-- Канал ОДИН на клуб, а не «каждый админ со своим владельцем»: вопрос у
-- стойки почти всегда операционный, и ответ полезен всей смене. Заодно из
-- переписки сам собой получается справочник частых случаев.

CREATE TABLE IF NOT EXISTS staff_messages (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    author_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       VARCHAR(8)  NOT NULL,           -- admin | owner: снимок на момент письма
    text       VARCHAR(1000) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_staff_messages_club ON staff_messages(club_id, created_at DESC);

-- Отметка «дочитал до» на пользователе, а не флаги на каждом сообщении:
-- участников в канале много, и таблица флагов росла бы произведением.
ALTER TABLE users ADD COLUMN IF NOT EXISTS staff_chat_read_at TIMESTAMPTZ;

COMMENT ON TABLE  staff_messages          IS 'Е6: рабочий канал персонала — админ спрашивает, владелец отвечает (Р7)';
COMMENT ON COLUMN staff_messages.role     IS 'Е6: роль автора на момент письма — потом он может стать кем угодно';
COMMENT ON COLUMN users.staff_chat_read_at IS 'Е6: докуда человек дочитал служебный канал';
