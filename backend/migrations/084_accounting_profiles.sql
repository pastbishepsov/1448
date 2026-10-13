-- 084 — З13-и6 (OWNER.md, трек З; №76; решения основателя 04.10, З-Р37 — бухгалтерия под любого бухгалтера, только
-- Польша): профили учёта — KPiR, ryczałt, полная бухгалтерия.
--
-- У категории расхода — колонка KPiR по образцу книги с 2026 года (12 — zakup towarów handlowych i materiałów, 13 —
-- koszty uboczne zakupu, 14 — wynagrodzenia, 15 — pozostałe wydatki; 0 — не в книгу) и счёт для полной бухгалтерии
-- (до 32 знаков, пусто — не задан). Значения по умолчанию: закупки — 12, «Зарплата» — 14, остальные — 15; счета
-- стандартных категорий — по типовому плану счетов (401 — материалы и энергия, 402 — услуги, 404 — зарплаты, 409 —
-- прочие, 330 — товары, 310 — материалы), у «ZUS и налоги» и «Оборудование и ремонт» — пусто: в них разные по сути
-- операции, счёт выберет бухгалтер. Всё это владелец меняет со слов своего бухгалтера. Значения ставятся только
-- вместе с колонкой: повторный прогон правки владельца не откатывает.
--
-- У позиции — ставка ryczałtu в десятых долях процента (30 — 3 %, 85 — 8,5 %), по умолчанию 3 %; какие можно
-- выбрать, решает сервер (models.RyczaltRates). Ставка времени за ПК — настройка, не колонка. Реквизиты фирмы для
-- выгрузки — строковые настройки, без схемы.
--
-- Идемпотентна: колонки — проверкой существования, проверки ставятся заново.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                    WHERE table_schema = current_schema() AND table_name = 'expense_categories' AND column_name = 'kpir_col') THEN
        ALTER TABLE expense_categories ADD COLUMN kpir_col SMALLINT NOT NULL DEFAULT 15;
        UPDATE expense_categories SET kpir_col = 12 WHERE purchase;
        UPDATE expense_categories SET kpir_col = 14 WHERE code = 'salary';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                    WHERE table_schema = current_schema() AND table_name = 'expense_categories' AND column_name = 'account') THEN
        ALTER TABLE expense_categories ADD COLUMN account VARCHAR(32) NOT NULL DEFAULT '';
        UPDATE expense_categories SET account = CASE code
                WHEN 'rent'      THEN '402'
                WHEN 'utilities' THEN '401'
                WHEN 'internet'  THEN '402'
                WHEN 'goods'     THEN '330'
                WHEN 'kitchen'   THEN '310'
                WHEN 'salary'    THEN '404'
                WHEN 'software'  THEN '402'
                WHEN 'marketing' THEN '402'
                WHEN 'other'     THEN '409'
                ELSE '' END
         WHERE code IS NOT NULL;
    END IF;
END $$;

ALTER TABLE goods ADD COLUMN IF NOT EXISTS ryczalt_rate SMALLINT NOT NULL DEFAULT 30;

-- колонка и ставка — в разумных пределах; какие именно можно выбрать, решает сервер (models.KpirCols, RyczaltRates)
ALTER TABLE expense_categories DROP CONSTRAINT IF EXISTS expense_categories_kpir_col_check;
ALTER TABLE expense_categories ADD CONSTRAINT expense_categories_kpir_col_check CHECK (kpir_col BETWEEN 0 AND 99);
ALTER TABLE goods DROP CONSTRAINT IF EXISTS goods_ryczalt_rate_check;
ALTER TABLE goods ADD CONSTRAINT goods_ryczalt_rate_check CHECK (ryczalt_rate BETWEEN 1 AND 1000);

COMMENT ON COLUMN expense_categories.kpir_col IS 'З13-и6: колонка KPiR по образцу 2026 г. (12, 13, 14, 15); 0 — не в книгу';
COMMENT ON COLUMN expense_categories.account IS 'З13-и6: счёт для полной бухгалтерии; пусто — не задан';
COMMENT ON COLUMN goods.ryczalt_rate IS 'З13-и6: ставка ryczałtu позиции в десятых долях процента (30 — 3 %, 85 — 8,5 %)';
