-- 101 — З24-и5 (OWNER.md, трек З; №55, З-Р28): сезонные пакеты.
--
-- time_packages — условия продажи и списания пакета каталога:
--   sell_from, sell_to — когда пакет продаётся: даты по календарю клуба включительно (пусто — без границы); вне окна
--     его не продают ни у стойки, ни в приложении, в каталоге гостя его нет;
--   per_guest — сколько раз один гость может купить этот пакет (0 — без ограничения; отменённые выдачи не в счёт);
--   new_only — только новым гостям: первое появление не раньше seg_new_days суток назад (как «Новые» в отчётах, З5-и1);
--   spend_days_mask, spend_start_min, spend_end_min — когда тратятся минуты пакета: дни недели (пн = 1 … вс = 64) и
--     окно по стенным часам клуба (1440 — до полуночи; конец не позже начала — через полночь, окно — дню начала, как у
--     смен и расписания цен); по умолчанию — всегда (127, 0, 1440). Вне окна сессия платит другим запасом и кошельком,
--     а пакет ждёт своего часа.
-- user_packages — снимок окна списания на момент выдачи: правка каталога не меняет условий проданного (как снимок зоны
-- и ставок у сессии, З16). Старые выдачи — «всегда».
--
-- Идемпотентна: IF NOT EXISTS, проверки — через pg_constraint.

ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS sell_from DATE;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS sell_to DATE;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS per_guest SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS new_only BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS spend_days_mask SMALLINT NOT NULL DEFAULT 127;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS spend_start_min SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE time_packages ADD COLUMN IF NOT EXISTS spend_end_min SMALLINT NOT NULL DEFAULT 1440;

ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS spend_days_mask SMALLINT NOT NULL DEFAULT 127;
ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS spend_start_min SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE user_packages ADD COLUMN IF NOT EXISTS spend_end_min SMALLINT NOT NULL DEFAULT 1440;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'time_packages_season') THEN
        ALTER TABLE time_packages ADD CONSTRAINT time_packages_season CHECK (
            (sell_from IS NULL OR sell_to IS NULL OR sell_from <= sell_to)
            AND per_guest BETWEEN 0 AND 100
            AND spend_days_mask BETWEEN 1 AND 127
            AND spend_start_min BETWEEN 0 AND 1439 AND spend_end_min BETWEEN 1 AND 1440
            AND spend_start_min <> spend_end_min);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'user_packages_spend') THEN
        ALTER TABLE user_packages ADD CONSTRAINT user_packages_spend CHECK (
            spend_days_mask BETWEEN 1 AND 127
            AND spend_start_min BETWEEN 0 AND 1439 AND spend_end_min BETWEEN 1 AND 1440
            AND spend_start_min <> spend_end_min);
    END IF;
END $$;

-- лимит на гостя считает покупки этого пакета этим гостем
CREATE INDEX IF NOT EXISTS idx_user_packages_user_pack ON user_packages (user_id, package_id) WHERE voided_at IS NULL;

COMMENT ON COLUMN time_packages.sell_from IS 'З24-и5: продаётся с этой даты (календарь клуба); пусто — без границы';
COMMENT ON COLUMN time_packages.sell_to IS 'З24-и5: продаётся по эту дату включительно; пусто — без границы';
COMMENT ON COLUMN time_packages.per_guest IS 'З24-и5: не больше N покупок на гостя (отменённые не в счёт); 0 — без ограничения';
COMMENT ON COLUMN time_packages.new_only IS 'З24-и5: только новым гостям (первое появление не раньше seg_new_days суток назад)';
COMMENT ON COLUMN time_packages.spend_days_mask IS 'З24-и5: дни списания минут (пн = 1 … вс = 64), окно — spend_start_min…spend_end_min';
COMMENT ON COLUMN user_packages.spend_days_mask IS 'З24-и5: снимок окна списания пакета на момент выдачи';
