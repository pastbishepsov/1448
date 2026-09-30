-- 079 — З11-и1 (OWNER.md, трек З; №17; решения основателя 30.09): способ возврата у отмен.
--
-- Отмена депозита, продажи (и чека), выданного пакета и оплаченного заказа кухни записывает, как деньги
-- вернули гостю: cash — наличными из ящика, card — на карту, blik — по BLIK, wallet — на кошелёк (только то,
-- что платили кошельком), none — не возвращали (у долга — всегда: денег не брали). Ящик уменьшают только
-- наличные возвраты. Пусто — строка не отменена.
--
-- Отменённые до З11 возвращались тем же способом, каким платили (так их считал и ящик): им ставится способ
-- оплаты, неоплаченному долгу — none. Строки со способом оплаты не из списка остаются пустыми — ящик для них
-- по-прежнему смотрит на способ оплаты.
--
-- Идемпотентна: колонки — IF NOT EXISTS, проверки ставятся заново, дозаполняются только пустые.

ALTER TABLE deposits ADD COLUMN IF NOT EXISTS refund_method VARCHAR(8);
ALTER TABLE sales ADD COLUMN IF NOT EXISTS refund_method VARCHAR(8);
ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS refund_method VARCHAR(8);

ALTER TABLE deposits DROP CONSTRAINT IF EXISTS deposits_refund_method_check;
ALTER TABLE deposits ADD CONSTRAINT deposits_refund_method_check
    CHECK (refund_method IN ('cash', 'card', 'blik', 'wallet', 'none'));
ALTER TABLE sales DROP CONSTRAINT IF EXISTS sales_refund_method_check;
ALTER TABLE sales ADD CONSTRAINT sales_refund_method_check
    CHECK (refund_method IN ('cash', 'card', 'blik', 'wallet', 'none'));
ALTER TABLE user_packages DROP CONSTRAINT IF EXISTS user_packages_refund_method_check;
ALTER TABLE user_packages ADD CONSTRAINT user_packages_refund_method_check
    CHECK (refund_method IN ('cash', 'card', 'blik', 'wallet', 'none'));

UPDATE deposits SET refund_method = method
    WHERE voided_at IS NOT NULL AND refund_method IS NULL AND method IN ('cash', 'card', 'blik');
UPDATE sales SET refund_method = CASE WHEN paid_at IS NULL THEN 'none' ELSE method END
    WHERE voided_at IS NOT NULL AND refund_method IS NULL
      AND (paid_at IS NULL OR method IN ('cash', 'card', 'blik', 'wallet'));
UPDATE user_packages SET refund_method = method
    WHERE voided_at IS NOT NULL AND refund_method IS NULL AND method IN ('cash', 'card', 'blik', 'wallet');

COMMENT ON COLUMN deposits.refund_method IS 'З11-и1: как вернули деньги отменённого депозита — cash | card | blik | none; ящик уменьшает только cash';
COMMENT ON COLUMN sales.refund_method IS 'З11-и1: как вернули деньги отменённой продажи — cash | card | blik | wallet | none; ящик уменьшает только cash';
COMMENT ON COLUMN user_packages.refund_method IS 'З11-и1: как вернули деньги отменённой выдачи пакета — cash | card | blik | wallet | none; ящик уменьшает только cash';
