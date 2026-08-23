-- 027: зоны с ценой (спринт В4, этап 5; ADMIN.md).
-- До этого зона была просто ПОДПИСЬЮ на компьютере, а тариф — один на весь
-- клуб (clubs.base_rate_pln, 23 zł/ч) и нигде не редактировался. То есть VIP
-- стоил ровно столько же, сколько обычное место. Теперь зона — сущность со
-- своей ценой часа, имя владелец придумывает сам в конструкторе зала.
--
-- computers.zone остаётся как КЭШ имени: по нему уже строятся карта зала,
-- разрезы отчётов и фильтры. Источник правды — zone_id; кэш пишет только
-- сервер, одним путём (при привязке ПК и при переименовании зоны), поэтому
-- разъехаться им негде, зато не пришлось переписывать половину запросов.

CREATE TABLE IF NOT EXISTS zones (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id    UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    name       VARCHAR(32) NOT NULL,
    rate_pln   DECIMAL(8,2) NOT NULL CHECK (rate_pln > 0),
    sort       INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (club_id, name)
);

ALTER TABLE computers ADD COLUMN IF NOT EXISTS zone_id UUID REFERENCES zones(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_computers_zone ON computers (zone_id);

-- Перенос того, что уже есть: каждая существующая подпись становится зоной с
-- текущим клубным тарифом, компьютеры без подписи уезжают в зону «Общая».
-- После этого у каждого ПК есть зона, и владельцу остаётся только проставить
-- разные цены.
INSERT INTO zones (club_id, name, rate_pln, sort)
SELECT c.club_id, COALESCE(NULLIF(TRIM(c.zone), ''), 'Общая'), cl.base_rate_pln, 0
FROM computers c
JOIN clubs cl ON cl.id = c.club_id
GROUP BY c.club_id, COALESCE(NULLIF(TRIM(c.zone), ''), 'Общая'), cl.base_rate_pln
ON CONFLICT (club_id, name) DO NOTHING;

UPDATE computers c
SET zone_id = z.id, zone = z.name
FROM zones z
WHERE z.club_id = c.club_id
  AND z.name = COALESCE(NULLIF(TRIM(c.zone), ''), 'Общая')
  AND c.zone_id IS NULL;

COMMENT ON TABLE zones IS 'Зоны зала со своей ценой часа (спринт В4-5)';
COMMENT ON COLUMN computers.zone IS 'Кэш имени зоны; источник правды — zone_id';
