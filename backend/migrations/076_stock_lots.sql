-- 076 — З10-и2 (OWNER.md, трек З; №68, №69, №70, №78): партии склада — таблицы. В работу не подключены:
-- партии пишет З10-и3 (stockMove, начальные партии), поставки — З10-и4 (приёмка) и З10-и5 (цены).
--
-- supplies — поставка: кто принял (received_by) и когда (created_at), заметка; status: waiting — ждёт
-- цен, priced — оценена. При оценке (З10-и5): контрагент, номер и дата документа, способ оплаты (как у
-- расхода), кто и когда оценил, расход «Закупка …» (expense_id) и сумма, которую поставка в него
-- записала (expense_grosz): расход поправили вручную — сумма в нём другая, поставка его больше не
-- перезаписывает, а расхождение видно.
--
-- stock_lots — партия позиции: откуда (kind: supply — поставка, initial — остаток на момент включения
-- партий, found — «нашлось» при корректировке, Д5), сколько пришло (qty_in) и осталось (qty_left), цена
-- за штуку в грошах (price_grosz; пусто — не оценена). Порядок списания (FIFO) — received_at, затем
-- part: строка поставки, разбитая на несколько цен («20 × 2,50 + 10 × 2,80»), — куски одной строки с
-- одним received_at, первая цена — первой. Остаток позиции = сумма qty_left её партий (сверка —
-- З10-и3). qty_left сверху не ограничен qty_in: отмена продажи, сделанной до партий, возвращает товар в
-- самую новую партию позиции.
--
-- lot_moves — какое движение склада из какой партии сколько взяло или положило (знак — как у движения:
-- списание −, приход и возврат +); одно движение — не больше одной строки на партию. Движение удалят —
-- его строки уйдут с ним; партию, у которой есть движения, не удалить.
--
-- Идемпотентна: таблицы и индексы — IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS supplies (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id         UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    status          VARCHAR(8) NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'priced')),
    note            TEXT NOT NULL DEFAULT '',
    received_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    counterparty_id UUID REFERENCES counterparties(id),
    doc_no          VARCHAR(64) NOT NULL DEFAULT '',
    doc_date        DATE,
    method          VARCHAR(8) CHECK (method IN ('till', 'bank', 'card', 'owner')),
    priced_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    priced_at       TIMESTAMPTZ,
    expense_id      UUID REFERENCES expenses(id) ON DELETE SET NULL,
    expense_grosz   BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_supplies_waiting ON supplies (created_at) WHERE status = 'waiting';

CREATE TABLE IF NOT EXISTS stock_lots (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id     UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    good_id     UUID NOT NULL REFERENCES goods(id) ON DELETE RESTRICT,
    kind        VARCHAR(8) NOT NULL CHECK (kind IN ('supply', 'initial', 'found')),
    supply_id   UUID REFERENCES supplies(id) ON DELETE RESTRICT,
    qty_in      INT NOT NULL CHECK (qty_in >= 0),
    qty_left    INT NOT NULL CHECK (qty_left >= 0),
    price_grosz BIGINT CHECK (price_grosz >= 0),
    received_at TIMESTAMPTZ NOT NULL,
    part        SMALLINT NOT NULL DEFAULT 1 CHECK (part >= 1),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stock_lots_supply_kind CHECK ((kind = 'supply') = (supply_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_stock_lots_fifo ON stock_lots (good_id, received_at, part);
CREATE INDEX IF NOT EXISTS idx_stock_lots_supply ON stock_lots (supply_id) WHERE supply_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS lot_moves (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    stock_move_id UUID NOT NULL REFERENCES stock_moves(id) ON DELETE CASCADE,
    lot_id        UUID NOT NULL REFERENCES stock_lots(id) ON DELETE RESTRICT,
    qty           INT NOT NULL CHECK (qty <> 0),
    CONSTRAINT lot_moves_move_lot UNIQUE (stock_move_id, lot_id)
);
CREATE INDEX IF NOT EXISTS idx_lot_moves_lot ON lot_moves (lot_id);

COMMENT ON TABLE supplies IS 'З10-и2: поставка — принята (ждёт цен) или оценена; при оценке — документ, способ, расход';
COMMENT ON TABLE stock_lots IS 'З10-и2: партия позиции — пришло, осталось, цена за штуку (пусто — не оценена); FIFO по received_at, part';
COMMENT ON TABLE lot_moves IS 'З10-и2: какое движение склада из какой партии сколько (знак — как у движения)';
