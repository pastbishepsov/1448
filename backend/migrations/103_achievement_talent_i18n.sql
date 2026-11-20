-- 103 — К12-и1 (KIOSK.md, этап VII, трек К; №10 осмотра 09.11 — «да, надо сделать»; В1 19.11 — «Да, давай так»):
-- переводы ачивок и талантов для гостя — польский и английский рядом с русским.
--
-- Готового способа переводить тексты из базы в проекте не было (гостевые экраны переводят словари киоска и PWA),
-- поэтому поля рядом с русскими. Сервер отдаёт гостю текст на его языке (users.language, 041): GET /me/achievements,
-- GET /me/talents и уведомление «ачивка получена» (язык — на момент выдачи). Пустой перевод или одни пробелы —
-- русский. Русские title, name и description не меняются: их по-прежнему видят админка и владелец.
-- Сами тексты — К12-и2, отдельной миграцией.
--
-- Откат (down-файлов в репо нет — миграции однократны по журналу schema_migrations):
--   ALTER TABLE achievements DROP COLUMN title_pl, DROP COLUMN title_en, DROP COLUMN description_pl, DROP COLUMN description_en;
--   ALTER TABLE talent_definitions DROP COLUMN name_pl, DROP COLUMN name_en, DROP COLUMN description_pl, DROP COLUMN description_en;
--
-- Идемпотентна: повтор ничего не меняет.

ALTER TABLE achievements
    ADD COLUMN IF NOT EXISTS title_pl       VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS title_en       VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description_pl TEXT         NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description_en TEXT         NOT NULL DEFAULT '';

ALTER TABLE talent_definitions
    ADD COLUMN IF NOT EXISTS name_pl        VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS name_en        VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description_pl TEXT         NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description_en TEXT         NOT NULL DEFAULT '';

COMMENT ON COLUMN achievements.title_pl IS 'К12-и1: название для гостя по-польски; пусто — русское title';
COMMENT ON COLUMN achievements.title_en IS 'К12-и1: название для гостя по-английски; пусто — русское title';
COMMENT ON COLUMN achievements.description_pl IS 'К12-и1: описание для гостя по-польски; пусто — русское description';
COMMENT ON COLUMN achievements.description_en IS 'К12-и1: описание для гостя по-английски; пусто — русское description';
COMMENT ON COLUMN talent_definitions.name_pl IS 'К12-и1: название для гостя по-польски; пусто — русское name';
COMMENT ON COLUMN talent_definitions.name_en IS 'К12-и1: название для гостя по-английски; пусто — русское name';
COMMENT ON COLUMN talent_definitions.description_pl IS 'К12-и1: описание для гостя по-польски; пусто — русское description';
COMMENT ON COLUMN talent_definitions.description_en IS 'К12-и1: описание для гостя по-английски; пусто — русское description';
