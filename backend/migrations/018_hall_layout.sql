-- 018: конструктор зала (спринт А8 админ-трека, ADMIN.md).
-- У ПК появляются координаты на схеме клуба (NULL = вне схемы),
-- у клуба — размер схемы-прямоугольника. Владелец расставляет ПК сам.

ALTER TABLE computers
    ADD COLUMN IF NOT EXISTS pos_x INT,
    ADD COLUMN IF NOT EXISTS pos_y INT;

ALTER TABLE clubs
    ADD COLUMN IF NOT EXISTS layout_w INT NOT NULL DEFAULT 12,
    ADD COLUMN IF NOT EXISTS layout_h INT NOT NULL DEFAULT 8;
