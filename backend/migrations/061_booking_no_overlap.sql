-- 061 — Ж0-и3 (ADMIN.md, трек Ж, решение Ж-Р5): гонка броней.
--
-- До этой миграции пересечения проверялись только в коде и вне транзакции:
-- два одновременных запроса (две стойки, дабл-клик, телефон) давали две живые
-- брони на один ПК и одно время (ревью 14.09: 4 параллельных POST → 2 брони).
-- База должна держать инвариант сама — как частичный UNIQUE держит «одна
-- активная сессия на ПК» с миграции 053.
--
-- Интервал брони считает IMMUTABLE-обёртка booking_span: сам оператор
-- timestamptz + interval помечен в Postgres как STABLE (интервал в днях зависит
-- от перевода часов), но прибавление МИНУТ к абсолютному моменту от зоны не
-- зависит, поэтому обёртка честная. Касание краями ('[)') пересечением не
-- считается — как bookingOverlaps в коде.
--
-- Уже существующие пересечения (их могло оставить ревью) чинятся здесь же
-- детерминированно: из пересекающихся живых броней остаётся созданная раньше,
-- более поздние снимаются с пометкой в notes — иначе ограничение не встанет,
-- а падение миграции остановило бы сервер (entrypoint строгий с ревью 26.08).
-- Миграция идемпотентна: второй прогон ничего не меняет.

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE OR REPLACE FUNCTION booking_span(start_at TIMESTAMPTZ, minutes INTEGER)
RETURNS TSTZRANGE
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$ SELECT tstzrange(start_at, start_at + make_interval(mins => minutes), '[)') $$;

DO $$
DECLARE
    fixed INTEGER;
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'bookings_no_overlap') THEN
        RETURN;
    END IF;

    -- пересечения, накопленные до ограничения: снимаем более позднюю по created_at
    WITH dup AS (
        SELECT DISTINCT b2.id
        FROM bookings b1
        JOIN bookings b2
          ON b2.computer_id = b1.computer_id
         AND b2.id <> b1.id
         AND b1.status IN ('pending', 'confirmed')
         AND b2.status IN ('pending', 'confirmed')
         AND booking_span(b1.start_time, b1.duration_min) && booking_span(b2.start_time, b2.duration_min)
         AND (b1.created_at < b2.created_at OR (b1.created_at = b2.created_at AND b1.id < b2.id))
    )
    UPDATE bookings
       SET status = 'cancelled',
           notes  = COALESCE(notes || ' ', '') || '[061: пересекалась с более ранней бронью на этот ПК — снята миграцией]'
     WHERE id IN (SELECT id FROM dup);
    GET DIAGNOSTICS fixed = ROW_COUNT;
    IF fixed > 0 THEN
        RAISE NOTICE '061: снято пересекающихся броней: %', fixed;
    END IF;

    ALTER TABLE bookings
        ADD CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (
            computer_id WITH =,
            booking_span(start_time, duration_min) WITH &&
        ) WHERE (status IN ('pending', 'confirmed'));
END $$;
