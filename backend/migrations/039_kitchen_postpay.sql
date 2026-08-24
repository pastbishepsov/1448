-- 039: кухня — постоплата (решение основателя 2026-08-24, Р10 GUEST.md).
--
-- Один вариант заказа вместо выбора «кошелёк/у стойки»: гость заказал из-за
-- компа, ему принесли, а рассчитывается он у стойки, когда закончил играть.
-- Следствия:
--   - заказ создаётся НЕоплаченным (paid='postpay'); никакого списания при
--     заказе; продажа (и выручка) появляется в момент РАСЧЁТА;
--   - оплата — отдельный шаг админа: нал/карта/BLIK (выручка, как В2) или
--     кошелёк гостя (погашение обязательства, НЕ выручка — Р7);
--   - выданный и неоплаченный заказ висит в очереди «ждут оплаты» и в
--     карточке гостя, при завершении сессии гостю напоминание;
--   - заказ доступен только с активной сессией (за компом): постоплата из
--     дома смысла не имеет — у стойки продадут обычной продажей В2.

ALTER TABLE kitchen_orders ADD COLUMN IF NOT EXISTS pay_method VARCHAR(16); -- cash|card|blik|wallet
ALTER TABLE kitchen_orders ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;
ALTER TABLE kitchen_orders ADD COLUMN IF NOT EXISTS paid_by UUID REFERENCES users(id);

-- «ждут оплаты» и долг в карточке гостя ищутся этим частичным индексом
CREATE INDEX IF NOT EXISTS idx_kitchen_orders_unpaid
    ON kitchen_orders (user_id) WHERE status = 'done' AND paid_at IS NULL;

COMMENT ON COLUMN kitchen_orders.paid_at IS 'Р10: расчёт после игры; NULL у выданного = «ждёт оплаты»';
