-- 041: язык гостя (трек Г, спринт Г9-и1, этап VI.1).
--
-- Язык — пресет профиля (ru/en/pl): гость выбирает в PWA, значение едет за
-- ним на любой ПК (шелл заберёт его из /me — вынос строк шелла в STR идёт
-- отдельной итерацией Г9-и2). Валидация значений — в коде (PATCH /me).

ALTER TABLE users ADD COLUMN IF NOT EXISTS language VARCHAR(2) NOT NULL DEFAULT 'ru';

COMMENT ON COLUMN users.language IS 'Г9: ru|en|pl — язык гостевых экранов, пресет профиля';
