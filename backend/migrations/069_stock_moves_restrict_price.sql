-- 069 — З6-и2 (OWNER.md, трек З; №31): позиция с историей не удаляется, движение склада помнит цену.
--
-- stock_moves.good_id был ON DELETE CASCADE: удаление позиции (сервер проверял
-- только продажи) уносило остаток и все движения склада, и «Потери» за прошлые
-- дни менялись задним числом (стенд: «бой −3» → потери 3 шт → удалили позицию →
-- потери 0, остаток 7 исчез без движения). Теперь RESTRICT: база не даст удалить
-- позицию, у которой есть движения, — сервер отвечает 409 has_history
-- «выключи позицию». Удаляется только позиция без движений и с нулевым остатком.
--
-- stock_moves.price_pln — цена единицы в момент движения: «Потери» считаются по
-- ней, а не по нынешней цене позиции, и перецена не переписывает прошлые
-- потери. Старым движениям: у продажи и её отмены — цена из продажи (она и была
-- ценой в тот момент), остальным — нынешняя цена позиции: другой не сохранилось.
--
-- Идемпотентна: ограничение пересоздаётся с IF EXISTS, цена заполняется только
-- у пустых (NULL) и лишь потом получает умолчание и NOT NULL.

ALTER TABLE stock_moves DROP CONSTRAINT IF EXISTS stock_moves_good_id_fkey;
ALTER TABLE stock_moves ADD CONSTRAINT stock_moves_good_id_fkey
    FOREIGN KEY (good_id) REFERENCES goods(id) ON DELETE RESTRICT;

ALTER TABLE stock_moves ADD COLUMN IF NOT EXISTS price_pln DECIMAL(8,2);
UPDATE stock_moves m SET price_pln = s.price_pln
  FROM sales s
 WHERE m.price_pln IS NULL AND m.sale_id = s.id;
UPDATE stock_moves m SET price_pln = g.price_pln
  FROM goods g
 WHERE m.price_pln IS NULL AND m.good_id = g.id;
UPDATE stock_moves SET price_pln = 0 WHERE price_pln IS NULL;
ALTER TABLE stock_moves ALTER COLUMN price_pln SET DEFAULT 0;
ALTER TABLE stock_moves ALTER COLUMN price_pln SET NOT NULL;

COMMENT ON COLUMN stock_moves.good_id   IS 'З6-и2: ON DELETE RESTRICT — позиция с движениями не удаляется, её выключают';
COMMENT ON COLUMN stock_moves.price_pln IS 'З6-и2: цена единицы в момент движения, zł — «Потери» считаются по ней';
