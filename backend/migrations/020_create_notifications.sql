-- 020: уведомления гостю о действиях админа (спринт Б4 админ-трека, ADMIN.md).
-- Гостевой экран поллит GET /me/notifications (решение №8: канал гостя —
-- поллинг); выдача помечает read_at — тост показывается один раз.
-- Типы: booking_cancel | booking_restore | deposit | grant_xp | grant_case.

CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notifications_unread
    ON notifications (user_id, created_at) WHERE read_at IS NULL;
