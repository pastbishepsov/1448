-- 073 — З9-и3б (OWNER.md, трек З; З-Р30, ревью 23.09 №62): долг клуба владельцу.
--
-- Расход способом «личные деньги владельца» (expenses.method = 'owner') — клуб потратил деньги владельца
-- и должен их вернуть. «Клуб должен владельцу» = оплаченные расходы личными (без сторно) − возвраты (без
-- отмены); считается запросом, хранить его не нужно.
--
-- owner_repayments — возврат владельцу: сколько, откуда — из кассы (till: деньги ушли из ящика, «должно быть
-- в ящике» меньше с минуты записи) или с банка (bank: ящик не трогает), заметка, кто и когда. Ошибочный
-- возврат отменяет владелец (voided_at): строка остаётся с пометкой, долг и ящик возвращаются.
--
-- shift_closeouts.owner_grosz — возвраты владельцу из кассы в «ушло» сдачи (инкассации в сдаче — ушло минус
-- возвраты минус расходы минус возвраты владельцу).
--
-- Идемпотентна: таблица, индекс и колонка — IF NOT EXISTS; данных не меняет.

CREATE TABLE IF NOT EXISTS owner_repayments (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id      UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    amount_grosz BIGINT NOT NULL CHECK (amount_grosz > 0),
    method       VARCHAR(8) NOT NULL CHECK (method IN ('till', 'bank')),
    note         TEXT NOT NULL DEFAULT '',
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    voided_at    TIMESTAMPTZ,
    voided_by    UUID REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_owner_repayments_time ON owner_repayments (club_id, created_at);

ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS owner_grosz BIGINT NOT NULL DEFAULT 0;

COMMENT ON TABLE owner_repayments IS 'З9-и3б: возвраты владельцу его личных денег, потраченных на клуб; отмена — voided_at';
COMMENT ON COLUMN owner_repayments.method IS 'till — из кассы (ушло из ящика), bank — с банка (ящик не трогает)';
COMMENT ON COLUMN shift_closeouts.owner_grosz IS 'З9-и3б: возвраты владельцу из кассы в «ушло» сдачи';
