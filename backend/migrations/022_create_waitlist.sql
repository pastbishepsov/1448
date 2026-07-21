-- 022: очередь-вейтлист, когда все ПК заняты (спринт Б9 админ-трека, ADMIN.md).
-- Встать может только зарегистрированный гость: админ по нику у стойки
-- (POST /admin/waitlist) или сам гость (POST /me/waitlist — задел под PWA).
-- Один активный (waiting) на гостя; notified_at — разовое уведомление
-- «ПК свободен» голове очереди через шину Б4 (notifications).

CREATE TABLE IF NOT EXISTS waitlist (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'waiting', -- waiting | seated | removed
    added_by UUID REFERENCES users(id),            -- NULL = гость встал сам (PWA)
    notified_at TIMESTAMPTZ,                       -- «ПК свободен» уже отправлено
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ                        -- когда seated/removed
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlist_active_user
    ON waitlist (user_id) WHERE status = 'waiting';
CREATE INDEX IF NOT EXISTS idx_waitlist_waiting
    ON waitlist (club_id, created_at) WHERE status = 'waiting';
