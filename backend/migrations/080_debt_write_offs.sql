-- 080 — З11-и4 (OWNER.md, трек З; №15; З-Р12; решения основателя 30.09): списание долга гостя.
--
-- Долг гостя — выданное и не рассчитанное, только гостю за ПК: продажи «в долг» (бар: method = 'later' без
-- paid_at) и выданные заказы кухни «при выходе» (status = 'done' без paid_at). Гость ушёл и не заплатил —
-- владелец списывает весь его долг одной кнопкой, без причины (решение основателя 30.09): строка
-- debt_write_offs со счётом и суммами, у каждой списанной продажи и заказа — ссылка write_off_id. Склад не
-- трогаем — товар съеден; выручки нет; сумма — «Потери → долги» по дню списания. Гость потом заплатил —
-- владелец отменяет списание (решение основателя 30.09): cancelled_at, ссылки снимаются, долг снова открыт и
-- рассчитывается как обычно; отменённое списание — не потеря.
--
-- Идемпотентна: таблица, колонки и индексы — IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS debt_write_offs (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id),
    kitchen_count INT NOT NULL DEFAULT 0,
    kitchen_grosz BIGINT NOT NULL DEFAULT 0,
    bar_count     INT NOT NULL DEFAULT 0,
    bar_grosz     BIGINT NOT NULL DEFAULT 0,
    total_grosz   BIGINT NOT NULL DEFAULT 0,
    created_by    UUID REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at  TIMESTAMPTZ,
    cancelled_by  UUID REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_debt_write_offs_created ON debt_write_offs (created_at);
CREATE INDEX IF NOT EXISTS idx_debt_write_offs_user ON debt_write_offs (user_id, created_at);

ALTER TABLE sales ADD COLUMN IF NOT EXISTS write_off_id UUID REFERENCES debt_write_offs(id);
ALTER TABLE kitchen_orders ADD COLUMN IF NOT EXISTS write_off_id UUID REFERENCES debt_write_offs(id);
CREATE INDEX IF NOT EXISTS idx_sales_write_off ON sales (write_off_id) WHERE write_off_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kitchen_orders_write_off ON kitchen_orders (write_off_id) WHERE write_off_id IS NOT NULL;

COMMENT ON TABLE debt_write_offs IS 'З11-и4: списание долга гостя владельцем — весь открытый долг (бар в долг и выданная кухня при выходе) одной строкой; склад не трогается, сумма — «Потери → долги» по дню списания; cancelled_at — списание отменено, долг снова открыт';
COMMENT ON COLUMN sales.write_off_id IS 'З11-и4: продажа в долг списана этим списанием — не висит в долге, не рассчитывается и не отменяется, пока списание стоит';
COMMENT ON COLUMN kitchen_orders.write_off_id IS 'З11-и4: выданный заказ кухни при выходе списан этим списанием — не ждёт оплаты, пока списание стоит';
