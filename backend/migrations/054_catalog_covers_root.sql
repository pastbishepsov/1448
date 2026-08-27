-- 054_catalog_covers_root.sql
-- Каталог под наполнение владельцем (пакет П1, docs/КАТАЛОГ-ИГР.md).
--
-- 1) Обложка плитки. Была файлом web/covers/<id>.jpg, а список id — константой
--    COVERS в shell.html: владелец не мог добавить свою игру с картинкой.
--    Теперь обложка — поле записи каталога, два способа задать:
--      • cover_url — ссылка (внешняя или свой путь /covers/cs2.jpg из репо);
--      • cover/cover_type/cover_at — загруженный файл прямо в БД, как фото
--        позиции кухни (миграция 038). Папки для загрузок нет нигде: в клубе
--        ./web примонтирован read-only, на Railway файловая система эфемерна.
--    Загруженная обложка приоритетнее ссылки (см. catalogCoverURL в catalog.go).
--
-- 2) games_root — корень сетевого диска с играми. target вида
--    {GAMES}\CS2\cs2.exe разворачивается СЕРВЕРОМ при отдаче GET /catalog,
--    поэтому смена буквы диска или адреса NAS — правка одной настройки,
--    а не 50 записей руками. Пустое значение = подстановка выключена.
--
-- Идемпотентна: ADD COLUMN IF NOT EXISTS, UPDATE по условию, ON CONFLICT.

ALTER TABLE catalog_apps ADD COLUMN IF NOT EXISTS cover_url  TEXT;
ALTER TABLE catalog_apps ADD COLUMN IF NOT EXISTS cover      BYTEA;
ALTER TABLE catalog_apps ADD COLUMN IF NOT EXISTS cover_type VARCHAR(32);
ALTER TABLE catalog_apps ADD COLUMN IF NOT EXISTS cover_at   TIMESTAMPTZ;

COMMENT ON COLUMN catalog_apps.cover_url IS
    'П1: ссылка на обложку — внешняя http(s) или свой путь (/covers/cs2.jpg)';
COMMENT ON COLUMN catalog_apps.cover IS
    'П1: загруженная обложка (jpeg/png/webp ≤500КБ, жмёт клиент); BYTEA — деплой без папок';

-- Семь обложек, уже лежащих в репо (web/covers/<id>.jpg) — ровно тот список,
-- что был зашит константой COVERS в shell.html. Прописываем ссылками, чтобы
-- витрина брала обложку из каталога и ничего не пропало при переходе.
UPDATE catalog_apps SET cover_url = '/covers/' || id || '.jpg'
 WHERE cover_url IS NULL
   AND id IN ('cs2', 'dota2', 'gta5', 'apex', 'pubg', 'rivals', 'ow2');

-- Корень игр. Пусто = подстановки нет: {GAMES} в target остаётся видимым
-- шаблоном (админка предупредит), а не превращается в тихо битый путь.
INSERT INTO settings (key, value) VALUES ('games_root', '')
    ON CONFLICT (key) DO NOTHING;
