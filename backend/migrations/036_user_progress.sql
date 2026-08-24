-- 036: прогресс-счётчики по ачивочным суткам (трек Г, спринт Г5; GUEST.md).
--
-- Одна строка = гость × ачивочные сутки (сутки идут 14:48→14:48 клубного
-- времени, Р6; ключ = дата их НАЧАЛА). Пишет finishSession при завершении
-- сессии (минуты попадают в сутки завершения — простое честное правило,
-- граничные сессии редки); заказы кухни дошлёт Г7. Из этих строк движок
-- достижений (Г5) считает daily/weekly/monthly-условия и стрики визитов —
-- по active_minutes (Г2), чтобы «просидел N часов» нельзя было выфармить
-- простоем.

CREATE TABLE IF NOT EXISTS user_progress (
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day_key          VARCHAR(10) NOT NULL,          -- '2026-08-24' (начало суток 14:48)
    minutes          INT NOT NULL DEFAULT 0,         -- минуты сессий (как для XP)
    active_minutes   INT NOT NULL DEFAULT 0,         -- из них активных (Г2, анти-фарм)
    sessions         INT NOT NULL DEFAULT 0,
    kitchen_orders   INT NOT NULL DEFAULT 0,         -- Г7 дошлёт
    first_session_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, day_key)
);

CREATE INDEX IF NOT EXISTS idx_user_progress_day ON user_progress (day_key);

COMMENT ON TABLE user_progress IS 'Г5: счётчики по ачивочным суткам 14:48→14:48 — топливо периодических достижений';
