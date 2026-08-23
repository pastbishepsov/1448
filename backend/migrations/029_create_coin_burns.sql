-- 029: сгорание монет у неактивных гостей (спринт В4, этап 3; ADMIN.md).
-- Решение основателя 2026-08-18: три месяца аккаунт не трогаем вообще, после
-- этого баланс тает на 10% в неделю. Активность — СЕССИЯ ЗА ПК: «зашёл в
-- приложение» и «пополнил, не приходя» счётчик не сбрасывают.
--
-- Зачем: до этого обязательство клуба было вечным и только росло. Мягкое
-- таяние вместо обнуления выбрано осознанно — вернувшийся на четвёртом месяце
-- застаёт почти весь баланс, и это повод прийти, а не повод обидеться.
-- За две недели до старта таяния гость получает уведомление: пропавшему
-- гостю нужен честный повод вернуться, и это он.
--
-- Каждое сгорание пишется строкой: иначе обязательства в отчёте падали бы
-- сами по себе, и владелец не понял бы, куда делись монеты.

CREATE TABLE IF NOT EXISTS coin_burns (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    coins         BIGINT NOT NULL CHECK (coins > 0),
    balance_after BIGINT NOT NULL,
    idle_days     INT NOT NULL,   -- сколько дней не играл на момент сгорания
    pct           INT NOT NULL,   -- какой процент применили
    manual        BOOLEAN NOT NULL DEFAULT FALSE, -- прогон руками, а не по расписанию
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_coin_burns_user ON coin_burns (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_coin_burns_created ON coin_burns (created_at);

INSERT INTO settings (key, value) VALUES
    ('coin_idle_days',     '90'),  -- дней без сессии до первого сгорания
    ('coin_burn_pct_week', '10'),  -- процент баланса в неделю, 0 = не жечь
    ('coin_burn_warn_days','14')   -- за сколько дней предупредить гостя
ON CONFLICT (key) DO NOTHING;

COMMENT ON TABLE coin_burns IS 'Сгорание монет у неактивных гостей (спринт В4-3)';
