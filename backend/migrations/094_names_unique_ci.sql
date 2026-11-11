-- 094 — З19-и2 (OWNER.md, трек З; №40, решение основателя 23.09): имена ПК, зон и товаров — без пробелов по краям и
-- уникальны без учёта регистра.
--
-- Было: «PC-01 » и «pc-01» при живом PC-01 заводились как разные машины (у ПК уникальность проверял только сервер, с
-- регистром и пробелами), зона «standard» рядом со «Standard» и товар «Cola» рядом с «cola» — тоже (уникальность в базе —
-- с учётом регистра). У стойки и в отчётах такие имена не отличить.
--
-- Сначала пробелы по краям снимаются, кэш имени зоны у ПК — следом. Потом запрос-отчёт о дублях без учёта регистра: если
-- они есть, миграция останавливается с понятным текстом — какие имена и сколько раз — и ничего не меняет (весь блок
-- откатывается); чинить руками (переименовать в «Экономике», «Зале» или «Товарах») и запустить снова. Дублей нет —
-- уникальные индексы по lower(name) в пределах клуба.
--
-- Идемпотентна: пробелов после первого прогона нет, индексы — IF NOT EXISTS.

DO $$
DECLARE
  dups text;
BEGIN
  UPDATE computers SET name = btrim(name) WHERE name <> btrim(name);
  UPDATE zones SET name = btrim(name) WHERE name <> btrim(name);
  UPDATE computers c SET zone = z.name FROM zones z WHERE c.zone_id = z.id AND c.zone IS DISTINCT FROM z.name;
  UPDATE goods SET name = btrim(name) WHERE name <> btrim(name);

  SELECT string_agg(what || ' «' || names || '»', '; ' ORDER BY what, names) INTO dups FROM (
    SELECT 'ПК' AS what, string_agg(name, '», «' ORDER BY name) AS names FROM computers GROUP BY club_id, lower(name) HAVING count(*) > 1
    UNION ALL
    SELECT 'зона', string_agg(name, '», «' ORDER BY name) FROM zones GROUP BY club_id, lower(name) HAVING count(*) > 1
    UNION ALL
    SELECT 'товар', string_agg(name, '», «' ORDER BY name) FROM goods GROUP BY club_id, lower(name) HAVING count(*) > 1
  ) d;
  IF dups IS NOT NULL THEN
    RAISE EXCEPTION 'Миграция 094 остановлена: имена совпадают без учёта регистра — %. Переименуй их (ПК — в «Зале», зоны — в «Экономике», товары — в «Товарах») и запусти снова.', dups;
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS computers_club_name_ci ON computers (club_id, lower(name));
CREATE UNIQUE INDEX IF NOT EXISTS zones_club_name_ci ON zones (club_id, lower(name));
CREATE UNIQUE INDEX IF NOT EXISTS goods_club_name_ci ON goods (club_id, lower(name));
