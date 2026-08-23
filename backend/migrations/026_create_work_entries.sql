-- 026: табель — фактически отработанное время (спринт В3, этап 4; ADMIN.md).
-- До этого график был только ПЛАНОМ: кто на какой смене стоит. Факта не было
-- вовсе, поэтому зарплату по ставке из карточки посчитать было нечем.
--
-- Отмечается сотрудник сам: «пришёл» открывает запись, «ушёл» закрывает.
-- Владелец может исправить, добавить или удалить запись — всё в аудит.
-- Дата — КЛУБНЫЕ СУТКИ (день начала смены), чтобы ночная смена не делилась
-- пополам между двумя календарными днями, как и везде в отчётах.
--
-- auto_closed = сотрудник забыл отметить уход, и запись закрыл сервер при
-- следующем приходе. Часы в такой записи — по расписанию смены (а если смена
-- неизвестна, ноль): выдумывать отработанное время нельзя, владелец увидит
-- пометку и поправит руками.

CREATE TABLE IF NOT EXISTS work_entries (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id     UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shift_id    UUID REFERENCES shifts(id) ON DELETE SET NULL,
    date        DATE NOT NULL,                  -- клубные сутки, день начала смены
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ,                    -- NULL = человек на смене прямо сейчас
    minutes     INT NOT NULL DEFAULT 0,         -- считается при закрытии
    auto_closed BOOLEAN NOT NULL DEFAULT FALSE, -- забыл отметить уход
    note        TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES users(id),      -- кто создал запись (сам или владелец)
    edited_by   UUID REFERENCES users(id),      -- кто правил последним
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_work_entries_user_date ON work_entries (user_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_work_entries_date ON work_entries (date);
-- у одного человека не может быть двух открытых записей одновременно
CREATE UNIQUE INDEX IF NOT EXISTS idx_work_entries_open
    ON work_entries (user_id) WHERE ended_at IS NULL;

COMMENT ON TABLE work_entries IS 'Табель: фактические приходы и уходы сотрудников (спринт В3-4)';
