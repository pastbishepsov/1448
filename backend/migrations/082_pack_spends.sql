-- 082 — З12-и2 (OWNER.md, трек З; №49; З-Р9; Д7; решение основателя 03.10): расход пакетов по сессиям.
--
-- Биллинг снимает минуты с пакета тиками (billing.go → takePackageMinutes). Раньше оставался только итог на
-- сессии (sessions.pack_minutes_used) — когда и с какого пакета ушли минуты, было не восстановить, и
-- «Отработанную выручку» за период нечем было посчитать. Теперь каждое списание — строка: пакет, сессия, минуты,
-- когда и сколько это стоит — доля цены пакета за эти минуты (подаренный — 0 zł, денег за него не было, З11-и5).
-- Доля считается от накопленного расхода (цена × отыграно после − цена × отыграно до), поэтому строки пакета в
-- сумме дают ровно цену отыгранного, как бы тики его ни резали. До этой миграции строк нет — за прошлое
-- «Отработанная выручка» даёт оценку по средней цене минуты пакета (Д7).
--
-- Идемпотентна: таблица и индексы — IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS pack_spends (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_package_id UUID NOT NULL REFERENCES user_packages(id),
    session_id      UUID REFERENCES sessions(id) ON DELETE SET NULL,
    minutes         INT NOT NULL CHECK (minutes > 0),
    grosz           BIGINT NOT NULL CHECK (grosz >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_pack_spends_created ON pack_spends (created_at);
CREATE INDEX IF NOT EXISTS idx_pack_spends_session ON pack_spends (session_id);
CREATE INDEX IF NOT EXISTS idx_pack_spends_package ON pack_spends (user_package_id);

COMMENT ON TABLE pack_spends IS 'З12-и2: расход пакетов — каждое списание минут пакета биллингом: пакет, сессия, минуты, когда; grosz — доля цены пакета за эти минуты (подаренный — 0)';
COMMENT ON COLUMN pack_spends.grosz IS 'З12-и2: цена × отыграно после − цена × отыграно до, в грошах — строки пакета в сумме дают цену отыгранного без дрейфа округления';
