-- 062 — Ж1-и1 (ADMIN.md, трек Ж, решение Ж-Р4): отмена депозита.
--
-- Депозит был единственной денежной операцией стойки без отката: продажу,
-- выдачу пакета и выдачу минут за монеты админ может отменить, а опечатка
-- «500 вместо 50» жила в выручке до вмешательства владельца прямо в базе
-- (ревью 14.09, №4). Как и у продаж (024), строка депозита не удаляется:
-- voided_at выводит её из выручки, а что именно удалось вернуть —
-- отдельными полями, потому что вернуть можно не всё:
--   refund_grosz — сколько реально снято с кошелька: остаток на момент
--                  отмены, но не больше суммы депозита. Уже отыгранное долгом
--                  не становится (Р8 трека Г — долгов нет): разница
--                  amount − refund уходит в отчёт строкой потерь;
--   coins_taken  — сколько кэшбек-монет за этот депозит забрано обратно
--                  (coins_granted + bonus_coins, но не ниже нуля на балансе).
--
-- Снятие с кошелька идёт через walletApply своим видом операции deposit_void:
-- под refund оно легло бы как «возврат гостю», а это ровно обратное движение.
-- ALTER TYPE … ADD VALUE вне транзакции — entrypoint гонит файлы по одному
-- в autocommit, как 048.

ALTER TABLE deposits ADD COLUMN IF NOT EXISTS voided_at    TIMESTAMPTZ;
ALTER TABLE deposits ADD COLUMN IF NOT EXISTS voided_by    UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE deposits ADD COLUMN IF NOT EXISTS refund_grosz BIGINT NOT NULL DEFAULT 0;
ALTER TABLE deposits ADD COLUMN IF NOT EXISTS coins_taken  BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN deposits.voided_at    IS 'Отменённый депозит: из выручки ушёл, строка осталась';
COMMENT ON COLUMN deposits.refund_grosz IS 'Снято с кошелька при отмене (остаток, не больше суммы); amount − refund — «отыграно по ошибочному депозиту»';
COMMENT ON COLUMN deposits.coins_taken  IS 'Кэшбек-монет забрано при отмене (не ниже нуля на балансе)';

ALTER TYPE wallet_tx_kind ADD VALUE IF NOT EXISTS 'deposit_void';
