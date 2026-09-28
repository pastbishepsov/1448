-- 075 — З10-и1 (OWNER.md, трек З; подготовка партий, №68): движение склада знает свой заказ кухни.
--
-- Остаток позиции меняется только функцией stockMove (goods.go): изменение и строка stock_moves —
-- вместе, через неё все семь мест. У продажи и её отмены связь с движением уже есть (sale_id); у
-- заказа кухни и его отмены — теперь kitchen_order_id: по нему З10-и3 вернёт отменённый заказ в те
-- же партии. Старые движения заказов остаются без ссылки (NULL): партий у них всё равно нет — их
-- отмена после партий вернёт товар в самую новую партию позиции (грабли З10-и3). Заказ удалят —
-- движение останется, ссылка обнулится.
--
-- Идемпотентна: колонка и индекс — IF NOT EXISTS.

ALTER TABLE stock_moves ADD COLUMN IF NOT EXISTS kitchen_order_id UUID REFERENCES kitchen_orders(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_stock_moves_kitchen_order ON stock_moves (kitchen_order_id) WHERE kitchen_order_id IS NOT NULL;

COMMENT ON COLUMN stock_moves.kitchen_order_id IS 'З10-и1: заказ кухни, чьё это движение (заказ и его отмена)';
