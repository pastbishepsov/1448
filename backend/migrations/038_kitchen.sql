-- 038: кухня для гостя (трек Г, спринт Г7).
--
-- Конструктор товаров уже в main (В2, миграция 024): владелец ведёт ценник и
-- остатки, админ продаёт у стойки. Г7 достраивает гостевую сторону:
--   - goods получают описание и фото (сверка Г7-и0: в В2 их не было, а гостевой
--     кухне без картинки нечего показать на плитке; фото — BYTEA в БД, чтобы
--     деплой оставался «бинарь + Postgres», без папок и прав на запись);
--   - kitchen_orders — заказ гостя: одна позиция × количество (то же решение,
--     что у продаж В2 — «без корзины», заказать две вещи = два заказа);
--     оплата: wallet (списание с кошелька сразу, Р7 — это НЕ выручка) или
--     counter («оплачу у стойки» — выручка в момент выдачи, как продажа В2);
--   - склад резервируется в момент заказа (stock_move reason='order'),
--     отмена возвращает (reason='void') — владелец видит каждый шаг;
--   - при выдаче (done) создаётся строка sales: wallet-заказ — method='wallet'
--     (отчёты исключают его из выручки, Р7), counter-заказ — cash/card/blik.

ALTER TABLE goods ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE goods ADD COLUMN IF NOT EXISTS photo BYTEA;
ALTER TABLE goods ADD COLUMN IF NOT EXISTS photo_type VARCHAR(32);
ALTER TABLE goods ADD COLUMN IF NOT EXISTS photo_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS kitchen_orders (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id     UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    good_id     UUID REFERENCES goods(id) ON DELETE SET NULL,
    name        VARCHAR(64) NOT NULL,          -- снимок имени (позицию могут переименовать)
    qty         INT NOT NULL CHECK (qty > 0),
    price_pln   DECIMAL(8,2) NOT NULL,         -- цена за единицу на момент заказа
    total_pln   DECIMAL(8,2) NOT NULL,
    paid        VARCHAR(16) NOT NULL,          -- wallet | counter
    status      VARCHAR(16) NOT NULL DEFAULT 'new', -- new|accepted|preparing|delivering|done|cancelled
    computer_id UUID REFERENCES computers(id) ON DELETE SET NULL, -- «принесём к ПК»; NULL = у стойки
    sale_id     UUID REFERENCES sales(id) ON DELETE SET NULL,     -- продажа, созданная выдачей
    status_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    status_by   UUID REFERENCES users(id),     -- кто перевёл статус (NULL = гость)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_kitchen_orders_status ON kitchen_orders (status, created_at);
CREATE INDEX IF NOT EXISTS idx_kitchen_orders_user ON kitchen_orders (user_id, created_at DESC);

COMMENT ON TABLE kitchen_orders IS 'Заказы кухни гостями (Г7): оплата кошельком (не выручка, Р7) или у стойки';
COMMENT ON COLUMN goods.photo IS 'Г7: фото позиции (jpeg/png/webp ≤500КБ, жмёт клиент); BYTEA — деплой без папок';
