-- 065 — Ж6-и1/и2 (ADMIN.md, трек Ж, решение Ж-Р8): постоплата товаров и чек.
--
-- Товары у стойки платились только сразу (024: cash | card | blik). Гостю с
-- идущей сессией продать «в долг до выхода» было нельзя, хотя кухня так живёт
-- с 039 (Р10): заказал из-за компа — рассчитался у стойки, когда доиграл.
-- Основатель 15.09 (№16–17): бар — тем же механизмом, что кухня; кошельком
-- товары не платятся. Следствия:
--   - продажа с method='later' создаётся НЕоплаченной: paid_at NULL, склад
--     списан сразу (товар выдан), выручки ещё нет;
--   - расчёт у стойки ставит method в реальный способ (cash | card | blik |
--     wallet), paid_at и paid_by; в выручку продажа попадает в момент
--     РАСЧЁТА — все выручечные запросы теперь смотрят на paid_at, а не на
--     created_at (для обычной продажи это одно и то же время);
--   - postpaid = true остаётся навсегда: журнал и чек честно говорят
--     «продано в долг, рассчитано картой», даже когда method уже card.
--
-- receipt_id — чек (В2-бэклог «корзина»): несколько позиций одной продажей
-- получают общий receipt_id; журнал и «Продажи смены» показывают чек одной
-- строкой, «Вернуть» умеет и чек целиком, и одну позицию. У продаж кухни
-- (Р10) receipt_id NULL — каждый заказ рассчитывается сам по себе.
--
-- Бэкфилл paid_at = created_at защищён по method: у настоящих неоплаченных
-- строк method='later', и повторный прогон файла их не «оплатит».

ALTER TABLE sales ADD COLUMN IF NOT EXISTS paid_at    TIMESTAMPTZ;
ALTER TABLE sales ADD COLUMN IF NOT EXISTS paid_by    UUID REFERENCES users(id);
ALTER TABLE sales ADD COLUMN IF NOT EXISTS postpaid   BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE sales ADD COLUMN IF NOT EXISTS receipt_id UUID;

UPDATE sales SET paid_at = created_at WHERE paid_at IS NULL AND method <> 'later';

-- выручка за окно клубных суток — по paid_at
CREATE INDEX IF NOT EXISTS idx_sales_paid_at ON sales (paid_at);
-- чек одной строкой
CREATE INDEX IF NOT EXISTS idx_sales_receipt ON sales (receipt_id) WHERE receipt_id IS NOT NULL;
-- долг гостя: неоплаченные живые продажи
CREATE INDEX IF NOT EXISTS idx_sales_unpaid ON sales (user_id) WHERE paid_at IS NULL AND voided_at IS NULL;

COMMENT ON COLUMN sales.paid_at    IS 'Ж6 (Ж-Р8): момент денег; NULL = продано в долг, ждёт расчёта у стойки';
COMMENT ON COLUMN sales.postpaid   IS 'Ж6: продажа была в долг (method=later до расчёта)';
COMMENT ON COLUMN sales.receipt_id IS 'Ж6: чек — общий id позиций одной продажи';
