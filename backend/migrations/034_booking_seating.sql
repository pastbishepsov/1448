-- 034: правило посадки перед бронью (трек Г, спринт Г3; GUEST.md, Р3).
--
-- Контрольный пример основателя (закреплён тестом в booking_rules_test.go):
-- бронь на 19:00, буфер 15 мин → пришедшему в 18:00 «на час» — отказ
-- (окно 60 мин − буфер 15 = 45 < 60), пришедшему в 17:45 — посадка
-- (75 − 15 = 60 ≥ 60). За booking_lock_min (деф. 10) до брони ПК придержан:
-- сесть может только хозяин брони; его посадка гасит бронь статусом seated.
-- Сессия на ПК с чужой броней живёт с жёстким дедлайном (start − lock):
-- предупреждения за ~15 и ~5 минут, затем штатное завершение
-- ended_reason=booking (billing.go).

ALTER TYPE booking_status ADD VALUE IF NOT EXISTS 'seated';

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS bkwarn15_at TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS bkwarn5_at  TIMESTAMPTZ;

INSERT INTO settings (key, value) VALUES
    ('booking_buffer_min', '15'),  -- сессия должна закончиться за столько до чужой брони
    ('booking_lock_min', '10')     -- за столько минут ПК придержан под пришедшего по брони
ON CONFLICT (key) DO NOTHING;

COMMENT ON COLUMN sessions.bkwarn15_at IS 'Г3: «до брони ~15 мин» отправлено';
