-- 031: денежный кошелёк гостя (трек Г, спринт Г0-и1; GUEST.md, решения Р1/Р2/Р8).
-- До этого депозит сразу превращался в монеты, и «выйти, не потеряв деньги»
-- было невозможно: у гостя не существовало денежного остатка. Теперь депозит
-- кладёт деньги в кошелёк; сессия будет списывать их поминутно (Г1), а
-- остаток живёт на аккаунте до следующего визита.
--
-- Решения основателя 2026-08-23 (Р1 GUEST.md): кошелёк и монеты НЕ смешиваем.
-- Кошелёк — предоплаченные деньги: не тает, не сгорает, в монеты и обратно не
-- конвертируется. Монеты — кэшбек: живут по правилам В4 (таяние, погашение
-- только временем). Единый баланс означал бы, что таяние жжёт реальные
-- предоплаченные деньги — юридический и репутационный риск.
--
-- Деньги храним в ГРОШАХ (BIGINT): никакой плавающей запятой в деньгах.
-- Кошелёк меняется ТОЛЬКО через walletApply (cmd/server/wallet.go): каждая
-- операция — строка в wallet_transactions с balance_after. Прямой UPDATE
-- wallet_grosz в коде запрещён — это и есть настоящая защита от «взлома
-- баланса» из Rules гостевого экрана: сервер не принимает баланс извне.

ALTER TABLE users ADD COLUMN IF NOT EXISTS wallet_grosz BIGINT NOT NULL DEFAULT 0;

DO $$ BEGIN
    CREATE TYPE wallet_tx_kind AS ENUM
        ('deposit', 'session_spend', 'kitchen_spend', 'refund', 'adjust');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS wallet_transactions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          wallet_tx_kind NOT NULL,
    amount_grosz  BIGINT NOT NULL,          -- со знаком: + пополнение, − списание
    balance_after BIGINT NOT NULL,          -- кошелёк сразу после операции
    ref_type      VARCHAR(16),              -- deposit | session | sale | manual
    ref_id        UUID,                     -- id связанной сущности (депозит, сессия…)
    note          TEXT,
    created_by    UUID REFERENCES users(id),-- кто провёл (админ); NULL = система
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wallet_tx_user ON wallet_transactions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_created ON wallet_transactions (created_at);

COMMENT ON TABLE wallet_transactions IS 'Журнал денежного кошелька гостя (трек Г, Г0-и1): сумма всех строк гостя = users.wallet_grosz';
