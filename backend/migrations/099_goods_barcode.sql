-- 099 — З23-и5а (OWNER.md, трек З; №54 «в», З-Р28): штрихкод позиции.
--
-- goods.barcode — EAN-13 или EAN-8 цифрами (контрольную цифру проверяет сервер, barcodeNorm в barcode.go; UPC-A из 12
-- цифр хранится как EAN-13 с ведущим нулём). Пусто — штрихкода нет. Уникален в клубе: один код — одна позиция, иначе
-- сканер не знал бы, что продать; у выключенной позиции код тоже занят — включат обратно, и он снова её.
--
-- Идемпотентна: IF NOT EXISTS, проверка формата — через pg_constraint.

ALTER TABLE goods ADD COLUMN IF NOT EXISTS barcode VARCHAR(13);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'goods_barcode_format') THEN
        ALTER TABLE goods ADD CONSTRAINT goods_barcode_format CHECK (barcode IS NULL OR barcode ~ '^([0-9]{8}|[0-9]{13})$');
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_goods_barcode ON goods (club_id, barcode) WHERE barcode IS NOT NULL;

COMMENT ON COLUMN goods.barcode IS 'З23-и5а: штрихкод позиции — EAN-13 или EAN-8 цифрами, уникален в клубе; пусто — нет';
