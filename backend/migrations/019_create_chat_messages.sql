-- 019: чат гость ↔ админ (спринты Б2–Б3 админ-трека, ADMIN.md).
-- Вызов админа — сообщение kind='call' (решение №7: вызов = частный случай
-- чата). user_id NULL-able: вызов с ПК через агента может прийти без
-- известного гостя (нет активной сессии) — тогда хранится только computer_id.

CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    computer_id UUID REFERENCES computers(id) ON DELETE SET NULL,
    admin_id UUID REFERENCES users(id),          -- кто из персонала ответил/принял
    sender VARCHAR(8) NOT NULL,                  -- guest | staff
    kind VARCHAR(8) NOT NULL DEFAULT 'text',     -- text | call
    text VARCHAR(500) NOT NULL DEFAULT '',
    read_staff BOOLEAN NOT NULL DEFAULT FALSE,
    read_guest BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chat_user_created ON chat_messages (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_unread_staff ON chat_messages (created_at DESC) WHERE NOT read_staff;
