-- 098 — З23-и3 (OWNER.md, трек З; №54 «в», З-Р28): инвентаризация — акт пересчёта склада.
--
-- inventories — акт: номер по клубу (no: 1, 2, 3 … — следующий после самого большого), кто провёл (created_by) и когда
-- (created_at), заметка. Акт не правится и не отменяется, как бумажный: ошибку исправляет корректировка или новый акт.
--
-- inventory_lines — строки акта в порядке пересчёта (sort): позиция (good_id; позицию удалят — строка останется с её
-- именем), имя и категория снимком, сколько было в системе, когда начали считать (expected), и сколько насчитали
-- (counted); разница — counted − expected. Цена единицы — в момент акта (price_pln, как у движений склада, З6-и2).
-- stock_move_id — движение склада разницы; у сошедшейся строки его нет.
--
-- stock_moves.inventory_id — движение из акта: разница — корректировка (reason adjust) с номером акта; недостача — со
-- старых партий первыми или из выбранной владельцем, излишек — партия «нашлось» без цены (Д5). В «Потерях» такие
-- движения — как любые корректировки, и отдельно — «по инвентаризациям». Акт с движениями не удалить.
--
-- Идемпотентна: IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS inventories (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    no         INT NOT NULL CHECK (no > 0),
    note       TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT inventories_club_no UNIQUE (club_id, no)
);
CREATE INDEX IF NOT EXISTS idx_inventories_at ON inventories (created_at);

CREATE TABLE IF NOT EXISTS inventory_lines (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    inventory_id  UUID NOT NULL REFERENCES inventories(id) ON DELETE CASCADE,
    good_id       UUID REFERENCES goods(id) ON DELETE SET NULL,
    name          VARCHAR(64) NOT NULL,
    category      VARCHAR(32) NOT NULL DEFAULT '',
    expected      INT NOT NULL CHECK (expected >= 0),
    counted       INT NOT NULL CHECK (counted >= 0),
    price_pln     DECIMAL(8,2) NOT NULL DEFAULT 0,
    stock_move_id UUID REFERENCES stock_moves(id) ON DELETE SET NULL,
    sort          INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_inventory_lines_act ON inventory_lines (inventory_id, sort);

ALTER TABLE stock_moves ADD COLUMN IF NOT EXISTS inventory_id UUID REFERENCES inventories(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS idx_stock_moves_inventory ON stock_moves (inventory_id) WHERE inventory_id IS NOT NULL;

COMMENT ON TABLE inventories IS 'З23-и3: акт инвентаризации — номер по клубу, кто и когда провёл, заметка';
COMMENT ON TABLE inventory_lines IS 'З23-и3: строка акта — позиция, в системе и насчитали, цена в момент акта, движение разницы';
COMMENT ON COLUMN stock_moves.inventory_id IS 'З23-и3: движение — разница акта инвентаризации (корректировка с номером акта)';
