-- 072 — З9-и3а (OWNER.md, трек З; З-Р30, ревью 23.09 №62, №63 «а»; решение основателя 27.09): расход «из кассы»
-- в модели ящика.
--
-- Расход способом «из кассы» (till) — деньги ушли из ящика. Внесён в ту же смену (день операции ещё не кончился)
-- — «должно быть в ящике» уменьшается с момента записи. Внесён задним числом за день, который уже закрыт сдачей, —
-- текущий ящик не трогает, а объясняет расхождение той сдачи: в кассовой книге у неё — «объяснено расходом,
-- внесённым позже», расхождение дня уменьшается на эту сумму.
--
-- expenses.drawer_at — когда расход ушёл из ящика для модели ящика: момент записи или, задним числом, миг перед
-- объясняемой сдачей; NULL — ящик не трогает (не из кассы или не оплачен). expenses.explains_closeout — какая сдача
-- объяснена. shift_closeouts.expenses_grosz — расходы из кассы в «ушло» сдачи (инкассации в сдаче — ушло минус
-- возвраты минус расходы).
--
-- Бэкфилл — только пустого: оплаченные расходы из кассы до этой миграции ложатся в ящик в момент записи.
-- Идемпотентна: колонки и индекс — IF NOT EXISTS, бэкфилл — WHERE drawer_at IS NULL.

ALTER TABLE expenses ADD COLUMN IF NOT EXISTS drawer_at TIMESTAMPTZ;
ALTER TABLE expenses ADD COLUMN IF NOT EXISTS explains_closeout UUID REFERENCES shift_closeouts(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_expenses_drawer ON expenses (drawer_at) WHERE drawer_at IS NOT NULL;
ALTER TABLE shift_closeouts ADD COLUMN IF NOT EXISTS expenses_grosz BIGINT NOT NULL DEFAULT 0;

UPDATE expenses SET drawer_at = created_at
 WHERE method = 'till' AND paid AND storno_at IS NULL AND drawer_at IS NULL;

COMMENT ON COLUMN expenses.drawer_at IS 'З9-и3а: когда расход из кассы ушёл из ящика (момент записи или миг перед объясняемой сдачей); NULL — ящик не трогает';
COMMENT ON COLUMN expenses.explains_closeout IS 'З9-и3а: сдача, расхождение которой объясняет расход из кассы, внесённый задним числом';
COMMENT ON COLUMN shift_closeouts.expenses_grosz IS 'З9-и3а: расходы из кассы в «ушло» сдачи';
