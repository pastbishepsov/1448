-- 024: товары, продажи и остатки (спринт В2 трека владельца, ADMIN.md).
-- Решение основателя 2026-08-18: выручка клуба = пополнения + продажи еды и
-- товаров. Учёт — ценник и остатки, без себестоимости; платят ТОЛЬКО злотыми
-- (монеты за товар не принимаем, у монет своя история — см. бэклог трека В).
--
-- Продажа — строка на позицию, а не чек с корзиной: у стойки клуба продают
-- «колу» и «сникерс» двумя тапами, корзина добавила бы шаг на ровном месте.
-- Корзина/чек — в бэклоге, таблица к этому готова (общий created_at и смена).

CREATE TABLE IF NOT EXISTS goods (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    name       VARCHAR(64) NOT NULL,
    category   VARCHAR(32) NOT NULL DEFAULT '',
    price_pln  DECIMAL(8,2) NOT NULL CHECK (price_pln > 0),
    stock      INT NOT NULL DEFAULT 0,       -- остаток, может уйти в минус только через корректировку
    low_stock  INT NOT NULL DEFAULT 0,       -- порог «заканчивается»; 0 = не следим
    sort       INT NOT NULL DEFAULT 0,
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (club_id, name)
);

CREATE TABLE IF NOT EXISTS sales (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    good_id    UUID REFERENCES goods(id) ON DELETE SET NULL,
    name       VARCHAR(64) NOT NULL,          -- снимок имени: позицию могут переименовать
    qty        INT NOT NULL CHECK (qty > 0),
    price_pln  DECIMAL(8,2) NOT NULL,         -- цена за единицу на момент продажи
    total_pln  DECIMAL(8,2) NOT NULL,
    method     VARCHAR(16) NOT NULL DEFAULT 'cash',  -- cash | card | blik
    user_id    UUID REFERENCES users(id),     -- гость, если продали «на ник»
    created_by UUID NOT NULL REFERENCES users(id),
    voided_at  TIMESTAMPTZ,                   -- отмена продажи: строка остаётся, из выручки уходит
    voided_by  UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sales_created ON sales (created_at);
CREATE INDEX IF NOT EXISTS idx_sales_good ON sales (good_id);
CREATE INDEX IF NOT EXISTS idx_sales_user ON sales (user_id);

-- Движения склада: приход поставки, списание продажей, возврат при отмене,
-- ручная корректировка (бой/порча/пересчёт). Владелец видит по ним потери —
-- всё, что ушло со склада не через кассу.
CREATE TABLE IF NOT EXISTS stock_moves (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    good_id    UUID NOT NULL REFERENCES goods(id) ON DELETE CASCADE,
    delta      INT NOT NULL,                  -- + приход, − списание
    reason     VARCHAR(16) NOT NULL,          -- supply | sale | void | adjust
    note       TEXT NOT NULL DEFAULT '',
    sale_id    UUID REFERENCES sales(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stock_moves_good ON stock_moves (good_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_stock_moves_created ON stock_moves (created_at);

COMMENT ON TABLE goods IS 'Товары клуба: ценник и остатки (спринт В2)';
COMMENT ON TABLE sales IS 'Продажи товаров за злотые; voided_at — отменённая продажа';
COMMENT ON TABLE stock_moves IS 'Движения склада: приход, продажа, возврат, корректировка';
