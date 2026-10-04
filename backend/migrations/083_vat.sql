-- 083 — З13-и1 (OWNER.md, трек З; №72 «а»; решения основателя 04.10 — бухгалтерия под любого бухгалтера, только
-- Польша, VAT у пополнения — настройкой): ставки VAT и их снимки.
--
-- Ставка VAT — у зоны (время за ПК), пакета времени и позиции (товар и кухня): 23, 8, 5, 0 или −1 — «zw»
-- (освобождено). У новых — 23, владелец меняет. В каждой операции — снимок ставки на её момент, чтобы смена ставки
-- не переписывала прошлые месяцы: пополнение (ставка пополнения из настроек — для режима «VAT при пополнении»),
-- продажа и расчёт кухни (ставка позиции), выданный пакет (ставка пакета), сессия (ставка зоны ПК — для времени с
-- кошелька). Снимок пишет сервер при создании строки. NULL в снимке — операция до этой миграции: выгрузка берёт
-- нынешнюю ставку позиции, пакета, зоны или пополнения.
--
-- Идемпотентна: колонки — IF NOT EXISTS, проверки ставятся заново.

ALTER TABLE zones         ADD COLUMN IF NOT EXISTS vat_pct SMALLINT NOT NULL DEFAULT 23;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS vat_pct SMALLINT NOT NULL DEFAULT 23;
ALTER TABLE goods         ADD COLUMN IF NOT EXISTS vat_pct SMALLINT NOT NULL DEFAULT 23;
ALTER TABLE deposits      ADD COLUMN IF NOT EXISTS vat_pct SMALLINT;
ALTER TABLE sales         ADD COLUMN IF NOT EXISTS vat_pct SMALLINT;
ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS vat_pct SMALLINT;
ALTER TABLE sessions      ADD COLUMN IF NOT EXISTS vat_pct SMALLINT;

-- ставка — процент 0…100 или −1 («zw»); какие именно можно выбрать, решает сервер (models.VatRates)
ALTER TABLE zones DROP CONSTRAINT IF EXISTS zones_vat_pct_check;
ALTER TABLE zones ADD CONSTRAINT zones_vat_pct_check CHECK (vat_pct BETWEEN -1 AND 100);
ALTER TABLE time_packages DROP CONSTRAINT IF EXISTS time_packages_vat_pct_check;
ALTER TABLE time_packages ADD CONSTRAINT time_packages_vat_pct_check CHECK (vat_pct BETWEEN -1 AND 100);
ALTER TABLE goods DROP CONSTRAINT IF EXISTS goods_vat_pct_check;
ALTER TABLE goods ADD CONSTRAINT goods_vat_pct_check CHECK (vat_pct BETWEEN -1 AND 100);
ALTER TABLE deposits DROP CONSTRAINT IF EXISTS deposits_vat_pct_check;
ALTER TABLE deposits ADD CONSTRAINT deposits_vat_pct_check CHECK (vat_pct IS NULL OR vat_pct BETWEEN -1 AND 100);
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_vat_pct_check;
ALTER TABLE sales ADD CONSTRAINT sales_vat_pct_check CHECK (vat_pct IS NULL OR vat_pct BETWEEN -1 AND 100);
ALTER TABLE user_packages DROP CONSTRAINT IF EXISTS user_packages_vat_pct_check;
ALTER TABLE user_packages ADD CONSTRAINT user_packages_vat_pct_check CHECK (vat_pct IS NULL OR vat_pct BETWEEN -1 AND 100);
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_vat_pct_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_vat_pct_check CHECK (vat_pct IS NULL OR vat_pct BETWEEN -1 AND 100);

COMMENT ON COLUMN zones.vat_pct IS 'З13-и1: ставка VAT времени за ПК этой зоны, % (−1 — zw)';
COMMENT ON COLUMN time_packages.vat_pct IS 'З13-и1: ставка VAT пакета времени, % (−1 — zw)';
COMMENT ON COLUMN goods.vat_pct IS 'З13-и1: ставка VAT позиции (товар и кухня), % (−1 — zw)';
COMMENT ON COLUMN deposits.vat_pct IS 'З13-и1: снимок ставки пополнения на момент пополнения; NULL — пополнение до ставок';
COMMENT ON COLUMN sales.vat_pct IS 'З13-и1: снимок ставки позиции на момент продажи; NULL — продажа до ставок';
COMMENT ON COLUMN user_packages.vat_pct IS 'З13-и1: снимок ставки пакета на момент выдачи; NULL — выдача до ставок';
COMMENT ON COLUMN sessions.vat_pct IS 'З13-и1: снимок ставки зоны ПК на начало сессии (время с кошелька); NULL — сессия до ставок';
