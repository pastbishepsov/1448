-- 010_create_revoked_tokens.sql
-- Отозванные refresh-токены (logout + ротация при /auth/refresh).
-- Храним только jti и срок: после expires_at запись бессмысленна и подчищается.

CREATE TABLE revoked_tokens (
    jti        UUID PRIMARY KEY,
    user_id    UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_revoked_expires ON revoked_tokens(expires_at);
