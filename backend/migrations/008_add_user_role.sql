-- 008_add_user_role.sql
-- Роли аккаунтов: player (по умолчанию), admin (сотрудник клуба), owner (владелец).

CREATE TYPE user_role AS ENUM ('player', 'admin', 'owner');

ALTER TABLE users
    ADD COLUMN role user_role NOT NULL DEFAULT 'player';

CREATE INDEX idx_users_role ON users(role) WHERE role <> 'player';
