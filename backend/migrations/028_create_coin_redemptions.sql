-- 028: погашение монет временем за ПК (спринт В4, этап 2; ADMIN.md).
-- До этого монеты были дорогой в один конец: копились из шести источников и
-- не тратились нигде — coins_balance в коде только рос. Значит и «курса
-- погашения» не существовало, и вопрос владельца «сколько денег равно
-- монетам» ответа не имел.
--
-- Решения основателя 2026-08-18: монеты гасятся ВРЕМЕНЕМ за ПК; курс не
-- задаётся руками — владелец ставит цену часа зоны, а сколько это в монетах,
-- считает конвертер из настройки coins_per_pln_spend; баланс один (депозитные
-- и подаренные монеты не разделяем).
--
-- Обратный обмен монет в живые деньги не делаем сознательно (RESEARCH §4):
-- «выигрыш с денежной ценностью» — ровно та формулировка, от которой мы ушли,
-- убрав депозитный кейс.

CREATE TABLE IF NOT EXISTS coin_redemptions (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    zone_id    UUID REFERENCES zones(id) ON DELETE SET NULL,
    zone_name  VARCHAR(32) NOT NULL DEFAULT '',  -- снимок: зону могут переименовать
    minutes    INT NOT NULL CHECK (minutes > 0),
    coins      BIGINT NOT NULL CHECK (coins > 0),
    rate_pln   DECIMAL(8,2) NOT NULL,            -- цена часа зоны на момент погашения
    value_pln  DECIMAL(8,2) NOT NULL,            -- во сколько это обошлось клубу
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_coin_redemptions_created ON coin_redemptions (created_at);
CREATE INDEX IF NOT EXISTS idx_coin_redemptions_user ON coin_redemptions (user_id);

INSERT INTO settings (key, value) VALUES
    ('coins_per_pln_spend', '20'),   -- монет за 1 zł ПРИ СПИСАНИИ (курс погашения)
    ('coin_spend_max_min', '240')    -- потолок минут за одну операцию, 0 = без лимита
ON CONFLICT (key) DO NOTHING;

COMMENT ON TABLE coin_redemptions IS 'Монеты, погашенные временем за ПК (спринт В4-2)';
