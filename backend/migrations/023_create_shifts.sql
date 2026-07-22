-- 023: смены и график персонала (спринт Б11 админ-трека, ADMIN.md).
-- Шаблоны смен задаёт owner: время может пересекать полночь (end < start),
-- дни недели — битовая маска (бит 0 = понедельник, 127 = все дни).
-- График — назначение сотрудников (admin/owner) на шаблон × дату.
-- Правит только owner; персонал читает («на смене», сетка недели).

CREATE TABLE IF NOT EXISTS shifts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    name VARCHAR(32) NOT NULL,
    start_min INT NOT NULL,             -- минуты от полуночи, 0..1439
    end_min INT NOT NULL,               -- end < start = смена через полночь
    days_mask INT NOT NULL DEFAULT 127, -- биты пн..вс (бит 0 = понедельник)
    sort INT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shift_assignments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    date DATE NOT NULL,                 -- день НАЧАЛА смены (ночная — по вечеру)
    shift_id UUID NOT NULL REFERENCES shifts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (date, shift_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_shift_assignments_date
    ON shift_assignments (club_id, date);
