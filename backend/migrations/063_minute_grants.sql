-- 063 — Ж2-и1 (ADMIN.md, трек Ж, решение Ж-Р3): минуты за монеты привязаны к зоне.
--
-- До этой миграции выданные за монеты минуты (В4-redeem) и минуты компенсации
-- (К8) складывались в один счётчик users.coin_minutes без зоны, а биллинг
-- тратил его на любой машине. Зона при выдаче означала только ЦЕНУ в монетах:
-- 60 минут «в Standard» стоили дешевле, а отыгрывались в VIP — клуб терял
-- треть цены каждой выдачи (ревью 14.09, №3: Standard-минуты отыграны на VIP).
--
-- Теперь зонные минуты живут отдельными строками здесь — ровно как пакеты
-- (047, Р10 трека Е): строка на каждую выдачу, зона и её имя — снимком,
-- остаток minutes_left уменьшает биллинг guarded-UPDATE'ом. Тратятся ТОЛЬКО в
-- своей зоне; zone_id NULL — «без зоны», годны везде (пока такие не пишутся:
-- компенсация без ПК идёт в старый пул). users.coin_minutes остаётся пулом без
-- зоны: уже выданный запас дотрачивается как раньше, ничего не переносим.
--
-- source: redeem (выдача за монеты, ref_id → coin_redemptions.id) |
-- compensation (К8, ref_id → incidents.id). Отмена выдачи (coin_redeem_void)
-- забирает НЕотыгранный остаток гранта и возвращает монеты пропорционально —
-- как refundForVoid у пакетов; отдельного voided_at не нужно: остаток
-- обнуляется, а факт отмены живёт на самой выдаче (coin_redemptions.voided_at).
--
-- ON DELETE у зоны — SET NULL, не RESTRICT: выданные минуты — обязательство
-- клуба перед гостем, и удаление зоны не должно ни блокироваться выдачами
-- (в отличие от каталога пакетов), ни сжигать их — они становятся «без зоны».

CREATE TABLE IF NOT EXISTS minute_grants (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    zone_id        UUID          REFERENCES zones(id) ON DELETE SET NULL,
    zone_name      VARCHAR(32)  NOT NULL DEFAULT '',
    minutes_total  INTEGER      NOT NULL CHECK (minutes_total > 0),
    minutes_left   INTEGER      NOT NULL CHECK (minutes_left >= 0),
    source         VARCHAR(16)  NOT NULL CHECK (source IN ('redeem', 'compensation')),
    ref_id         UUID,
    created_by     UUID          REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Биллинг ищет живые гранты гостя в зоне на каждом тике — индекс только по живым.
CREATE INDEX IF NOT EXISTS idx_minute_grants_live
    ON minute_grants (user_id, zone_id) WHERE minutes_left > 0;
-- Отмена выдачи находит грант по выдаче.
CREATE INDEX IF NOT EXISTS idx_minute_grants_ref ON minute_grants (ref_id);

COMMENT ON TABLE  minute_grants               IS 'Ж2 (Ж-Р3): зонные минуты за монеты и компенсации — тратятся только в своей зоне, как пакеты; users.coin_minutes — старый пул без зоны';
COMMENT ON COLUMN minute_grants.zone_id       IS 'Зона, где минуты годны; NULL — везде';
COMMENT ON COLUMN minute_grants.minutes_left  IS 'Остаток; биллинг уменьшает guarded-UPDATE, отмена выдачи обнуляет';
COMMENT ON COLUMN minute_grants.source        IS 'redeem — выдача за монеты (ref_id → coin_redemptions), compensation — К8 (ref_id → incidents)';
