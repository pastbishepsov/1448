-- 066 — Ж6-и4 (ADMIN.md, трек Ж, №32): закрытие смены с кассой.
--
-- «Отметить уход» закрывал только табель (026): часы записаны, а сколько
-- наличных должно лежать в ящике и сколько лежит — никто не спрашивал.
-- Расхождение всплывало через день, когда уже не восстановить, кто и что.
--
-- Теперь уход может идти через пересчёт: сервер считает ожидаемую наличность
-- за клубные сутки ЭТОГО админа (нал по paid_at: депозиты + товары + пакеты +
-- расчёты кухни, минус наличные возвраты за сегодня по вчерашним операциям),
-- админ вбивает пересчитанное, разница ложится сюда и уходит владельцу в
-- канал персонала, в сводку смены и в табель. Уход без пересчёта остаётся
-- возможным — counted_grosz NULL и пометка skipped: владелец видит, что смену
-- сдали не считая.
--
-- Составляющие ожидаемой суммы хранятся отдельными колонками: через месяц
-- «должно было быть 480 zł» без разбивки не проверить.

CREATE TABLE IF NOT EXISTS shift_closeouts (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id        UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    work_entry_id  UUID REFERENCES work_entries(id) ON DELETE SET NULL,
    date           DATE NOT NULL,                   -- клубные сутки смены
    expected_grosz BIGINT NOT NULL DEFAULT 0,       -- должно быть в ящике
    deposits_grosz BIGINT NOT NULL DEFAULT 0,       -- из них: пополнения налом
    goods_grosz    BIGINT NOT NULL DEFAULT 0,       -- товары и расчёты кухни налом
    packs_grosz    BIGINT NOT NULL DEFAULT 0,       -- пакеты времени налом
    refunds_grosz  BIGINT NOT NULL DEFAULT 0,       -- наличные возвраты по операциям прошлых суток
    counted_grosz  BIGINT,                          -- пересчитано; NULL = ушёл без пересчёта
    diff_grosz     BIGINT,                          -- counted − expected
    skipped        BOOLEAN NOT NULL DEFAULT FALSE,  -- «без пересчёта»
    note           TEXT NOT NULL DEFAULT '',        -- «передал смену»
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_shift_closeouts_date ON shift_closeouts (date, created_at);
CREATE INDEX IF NOT EXISTS idx_shift_closeouts_entry ON shift_closeouts (work_entry_id);

COMMENT ON TABLE shift_closeouts IS 'Ж6-и4: сдача смены с пересчётом наличных (ожидалось / пересчитано / расхождение)';
