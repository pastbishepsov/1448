-- 081 — З11-и5 (OWNER.md, трек З; №18; З-Р12, Д6; решение основателя 30.09): способ «Подарено».
--
-- Продажа у стойки, выдача пакета времени и заказ кухни от стойки могут быть подарком: method = 'comp' (у заказа
-- кухни — pay_method = 'comp' и его продажа с тем же способом). Подарок — не выручка и не ящик; товар уходит со
-- склада как обычно, пакет даёт минуты как обычно. Причина обязательна (решение основателя 30.09): дарит обычно
-- админ, и владелец должен видеть, за что, — она в comp_reason, проверка ниже не даёт записать подарок без неё.
-- Админ дарит в пределах дневного потолка компенсаций (admin_day_comp_cap_pln — один на компенсации и подарки,
-- Д6), владелец — без лимита; это правило сервера.
--
-- Идемпотентна: колонки и индексы — IF NOT EXISTS, проверки ставятся заново.

ALTER TABLE sales ADD COLUMN IF NOT EXISTS comp_reason VARCHAR(200);
ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS comp_reason VARCHAR(200);

ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_comp_reason_check;
ALTER TABLE sales ADD CONSTRAINT sales_comp_reason_check
    CHECK (method <> 'comp' OR btrim(COALESCE(comp_reason, '')) <> '');
ALTER TABLE user_packages DROP CONSTRAINT IF EXISTS user_packages_comp_reason_check;
ALTER TABLE user_packages ADD CONSTRAINT user_packages_comp_reason_check
    CHECK (method <> 'comp' OR btrim(COALESCE(comp_reason, '')) <> '');

CREATE INDEX IF NOT EXISTS idx_sales_comp ON sales (created_at) WHERE method = 'comp';
CREATE INDEX IF NOT EXISTS idx_user_packages_comp ON user_packages (created_at) WHERE method = 'comp';

COMMENT ON COLUMN sales.comp_reason IS 'З11-и5: за что подарили (method = comp) — обязательно; подарок не выручка и не ящик';
COMMENT ON COLUMN user_packages.comp_reason IS 'З11-и5: за что подарили пакет (method = comp) — обязательно; подарок не выручка и не ящик';
