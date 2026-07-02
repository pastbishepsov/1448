-- 009_create_deposits.sql
-- Пополнения баланса. MVP: оформляет администратор (наличные/карта на кассе).
-- Stripe/BLIK подключатся позже и будут писать в эту же таблицу.

CREATE TABLE deposits (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        UUID NOT NULL REFERENCES users(id),
    amount_pln     DECIMAL(8,2) NOT NULL CHECK (amount_pln > 0),
    coins_granted  BIGINT NOT NULL,
    bonus_coins    BIGINT NOT NULL DEFAULT 0,          -- бонус таланта coin_mint
    method         VARCHAR(16) NOT NULL DEFAULT 'cash', -- cash | card | blik
    created_by     UUID REFERENCES users(id),           -- администратор
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deposits_user ON deposits(user_id);
CREATE INDEX idx_deposits_created ON deposits(created_at);
