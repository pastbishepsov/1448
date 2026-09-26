-- 070 — З8-и1 (OWNER.md, трек З; З-Р1, ревью 23.09 №1 и №16): модель «ящик клуба».
--
-- Касса считала наличные ЭТОГО админа за его смену (Ж6-и4, З1-и1), а ящик у клуба
-- один: пополнение от владельца в смену админа, наличный возврат по чужой продаже,
-- размен в ящике и вторая сдача за сутки давали ложные излишки и недостачи. Теперь
-- «должно быть в ящике» = остаток прошлой сдачи (пересчитанный, если пересчитывали)
-- + все наличные операции клуба с неё − наличные возвраты − инкассации ± коррекции.
--
-- shift_closeouts: opening_grosz — остаток ящика на начало; NULL у сдач до модели
-- ящика — точкой отсчёта они не служат. in/out/adjust — пришло, ушло (возвраты и
-- инкассации) и коррекции за период сдачи; deposits/goods/packs/refunds — те же
-- «пришло» и возвраты по видам, теперь за весь клуб. baseline — нулевая точка: первая
-- сдача с пересчётом после обновления, пересчитанное становится остатком на начало
-- без расхождения (решение основателя 26.09).
--
-- cash_events — инкассация (collect: вынули из ящика — в банк или сейф, amount_grosz
-- > 0) и коррекция владельца (correct: «в ящике сейчас N zł» — stated_grosz, а
-- amount_grosz — поправка к ожидаемому, ±). Ручки — З8-и2; ящик их уже учитывает.
--
-- user_packages.refund_grosz — сколько вернули при отмене выдачи: на кошелёк или,
-- у пакета за наличные, из ящика (до З11 возврат идёт тем же способом, что и оплата).
-- Раньше сумма наличного возврата нигде не записывалась.
--
-- Идемпотентна: колонки, таблица и индексы — IF NOT EXISTS.

ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS opening_grosz BIGINT;
ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS in_grosz BIGINT NOT NULL DEFAULT 0;
ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS out_grosz BIGINT NOT NULL DEFAULT 0;
ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS adjust_grosz BIGINT NOT NULL DEFAULT 0;
ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS baseline BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_shift_closeouts_drawer ON shift_closeouts (created_at) WHERE opening_grosz IS NOT NULL;

CREATE TABLE IF NOT EXISTS cash_events (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id      UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    kind         VARCHAR(8) NOT NULL CHECK (kind IN ('collect', 'correct')),
    amount_grosz BIGINT NOT NULL,
    stated_grosz BIGINT,
    dest         VARCHAR(8) CHECK (dest IN ('bank', 'safe')),
    note         TEXT NOT NULL DEFAULT '',
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    voided_at    TIMESTAMPTZ,
    voided_by    UUID REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_cash_events_time ON cash_events (club_id, created_at);

ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS refund_grosz BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN shift_closeouts.opening_grosz IS 'З8-и1: остаток ящика на начало; NULL — сдача до модели ящика';
COMMENT ON COLUMN shift_closeouts.baseline IS 'З8-и1: нулевая точка — первая сдача с пересчётом после обновления';
COMMENT ON TABLE cash_events IS 'З8-и1: инкассации (collect) и коррекции владельца (correct) — движение ящика вне продаж';
COMMENT ON COLUMN cash_events.amount_grosz IS 'collect: вынуто из ящика (> 0); correct: поправка к ожидаемому (±)';
COMMENT ON COLUMN user_packages.refund_grosz IS 'З8-и1: возврат при отмене выдачи — на кошелёк или наличными из ящика';
