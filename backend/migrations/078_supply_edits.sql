-- 078 — З10-и4 (OWNER.md, трек З; №78; решение основателя 28.09): приёмку можно исправить — с пометкой.
--
-- Пока поставка ждёт цен, админ или владелец может поправить штуки в строке (не меньше того, что из
-- строки уже ушло) или отменить поставку целиком, если из неё ничего не ушло. У исправленной поставки
-- остаётся пометка — когда и кто исправил (edited_at, edited_by); что именно — в журнале и в строках
-- («было 30 → стало 20» — из первого движения прихода партии). Отменённая — status = 'cancelled': в
-- «ждут цен» её нет, запись и движения остаются.
--
-- Идемпотентна: колонки — IF NOT EXISTS, тип и проверка статуса ставятся заново.

ALTER TABLE supplies ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;
ALTER TABLE supplies ADD COLUMN IF NOT EXISTS edited_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE supplies ALTER COLUMN status TYPE VARCHAR(16); -- 'cancelled' — девять букв
ALTER TABLE supplies DROP CONSTRAINT IF EXISTS supplies_status_check;
ALTER TABLE supplies ADD CONSTRAINT supplies_status_check CHECK (status IN ('waiting', 'priced', 'cancelled'));

COMMENT ON COLUMN supplies.edited_at IS 'З10-и4: когда приёмку исправили (штуки или отмена) — пометка «исправлено»';
