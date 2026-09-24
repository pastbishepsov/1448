-- 068 — З5-и2 (OWNER.md, трек З; №10, решения З-Р9 и Д3): монеты и минуты сходятся.
--
-- coin_redemptions.coins_returned — сколько монет вернулось гостю при отмене
-- выдачи времени. Отмена возвращает монеты пропорционально НЕотыгранному
-- остатку (refundCoinsForVoid), а отчёт «Монеты» выбрасывал из «Погашено» всю
-- выдачу целиком: «на руках = выпущено − погашено − сгорело − потрачено»
-- расходилось на отыгранную часть каждой отменённой выдачи. Теперь погашено =
-- coins − coins_returned. Отмены до этой миграции считаются «вернулось всё»
-- (Д3): сколько вернулось на самом деле, из базы не восстановить.
--
-- minute_grants.rate_pln — тариф, по которому выданы минуты (zł за час):
-- «должны временем» теперь и в злотых. Выдачам за монеты — тариф из самой
-- выдачи (coin_redemptions.rate_pln), компенсациям — нынешний тариф зоны,
-- гранту без зоны — клубный тариф. Новые гранты пишут тариф сами.
--
-- Идемпотентна: колонки добавляются с IF NOT EXISTS, заполняются только пустые
-- (NULL) и лишь потом получают умолчание и NOT NULL — повторный прогон ничего
-- не меняет.

ALTER TABLE coin_redemptions ADD COLUMN IF NOT EXISTS coins_returned BIGINT;
UPDATE coin_redemptions SET coins_returned = CASE WHEN voided_at IS NOT NULL THEN coins ELSE 0 END
 WHERE coins_returned IS NULL;
ALTER TABLE coin_redemptions ALTER COLUMN coins_returned SET DEFAULT 0;
ALTER TABLE coin_redemptions ALTER COLUMN coins_returned SET NOT NULL;

ALTER TABLE minute_grants ADD COLUMN IF NOT EXISTS rate_pln DECIMAL(8,2);
UPDATE minute_grants g SET rate_pln = r.rate_pln
  FROM coin_redemptions r
 WHERE g.rate_pln IS NULL AND g.source = 'redeem' AND g.ref_id = r.id;
UPDATE minute_grants g SET rate_pln = z.rate_pln
  FROM zones z
 WHERE g.rate_pln IS NULL AND g.zone_id = z.id;
UPDATE minute_grants SET rate_pln = COALESCE((SELECT base_rate_pln FROM clubs ORDER BY created_at LIMIT 1), 0)
 WHERE rate_pln IS NULL;
ALTER TABLE minute_grants ALTER COLUMN rate_pln SET DEFAULT 0;
ALTER TABLE minute_grants ALTER COLUMN rate_pln SET NOT NULL;

COMMENT ON COLUMN coin_redemptions.coins_returned IS 'З5-и2: вернулось гостю при отмене выдачи (пропорционально неотыгранному); погашено = coins − coins_returned; отмены до 068 — все монеты (Д3)';
COMMENT ON COLUMN minute_grants.rate_pln IS 'З5-и2: тариф выданных минут, zł/ч — «должны временем» в злотых';
