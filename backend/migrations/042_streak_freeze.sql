-- 042: заморозка стрика за монеты (трек Г, спринт Г6-и4).
--
-- Механика (решение Р11 GUEST.md): гость покупает «заморозку» ЗАРАНЕЕ, и она
-- автоматически прикрывает пропущенный день, чтобы стрик визитов не начинался
-- с нуля. Сознательно НЕ делаем «восстановить стрик задним числом за монеты»:
-- это ровно та связка «потерял → доплати», от которой ушли в RESEARCH §4.
--
-- Замороженный день НЕ засчитывается как визит: он только не рвёт цепочку.
-- «Неделя без пропусков» по-прежнему требует семи реальных приходов —
-- заморозка не покупает ачивку, она бережёт накопленное.
--
-- Деньги/монеты: трата монет уменьшает обязательство клуба, поэтому каждая
-- покупка пишется строкой в coin_spends — иначе монеты «худели» бы в отчёте
-- эмиссии (В4-2) молча, как это было бы со сгоранием без coin_burns.

ALTER TABLE users ADD COLUMN IF NOT EXISTS streak_freezes INT NOT NULL DEFAULT 0;
ALTER TABLE user_progress ADD COLUMN IF NOT EXISTS frozen BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS coin_spends (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          VARCHAR(24) NOT NULL,          -- streak_freeze (дальше — другие траты)
    coins         BIGINT NOT NULL CHECK (coins > 0),
    balance_after BIGINT NOT NULL,
    note          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_coin_spends_user ON coin_spends (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_coin_spends_created ON coin_spends (created_at);

-- Цена ~1,5 часа игры (120 монет/час) и примерно чашка кофе по spend-курсу
-- (20 монет = 1 zł). 0 = механика выключена целиком.
INSERT INTO settings (key, value) VALUES
    ('streak_freeze_cost',    '150'),
    ('streak_freeze_max',     '3'),
    ('streak_freeze_max_row', '2')
ON CONFLICT (key) DO NOTHING;

COMMENT ON TABLE coin_spends IS 'Траты монет на клубные плюшки (Г6-и4): обязательство гасится, отчёт эмиссии сходится';
COMMENT ON COLUMN user_progress.frozen IS 'Г6-и4: день прикрыт заморозкой — цепочку не рвёт, визитом не считается';
