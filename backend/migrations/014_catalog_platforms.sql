-- 014_catalog_platforms.sql
-- Каталог: игр и приложений в клубе много — игры группируются по платформам.
-- Новая категория 'platform' — сам клиент платформы (Steam, Riot, Epic, Battle.net),
-- запускаемая карточка в шапке своей группы. Связь: game.subtitle = platform.name
-- (редактируется в админке, поле «Подпись»).

ALTER TABLE catalog_apps DROP CONSTRAINT catalog_apps_category_check;
ALTER TABLE catalog_apps ADD CONSTRAINT catalog_apps_category_check
    CHECK (category IN ('game', 'app', 'system', 'platform'));

-- Steam из «приложений» становится платформой.
UPDATE catalog_apps SET category = 'platform' WHERE id = 'steam';

-- GTA V запускается через Steam — группа Steam.
UPDATE catalog_apps SET subtitle = 'Steam' WHERE id = 'gta5' AND subtitle = 'Rockstar';

-- Клиенты платформ (name — ключ группировки игр по subtitle).
INSERT INTO catalog_apps (id, name, subtitle, category, tag, emoji, color_a, color_b, target, args, sort) VALUES
('riot',      'Riot Games', NULL, 'platform', NULL, '⚡', NULL, NULL, 'C:\Riot Games\Riot Client\RiotClientServices.exe',          NULL, 20),
('epic',      'Epic Games', NULL, 'platform', NULL, '🎪', NULL, NULL, 'com.epicgames.launcher://',                                 NULL, 30),
('battlenet', 'Battle.net', NULL, 'platform', NULL, '🌀', NULL, NULL, 'C:\Program Files (x86)\Battle.net\Battle.net Launcher.exe', NULL, 40)
ON CONFLICT (id) DO NOTHING;

-- Больше игр по платформам.
INSERT INTO catalog_apps (id, name, subtitle, category, tag, emoji, color_a, color_b, target, args, sort) VALUES
('apex',   'Apex Legends',  'Steam',      'game', 'APEX', NULL, '#f87171', '#7f1d1d', 'steam://rungameid/1172470', NULL, 70),
('pubg',   'PUBG',          'Steam',      'game', 'PUBG', NULL, '#fbbf24', '#92400e', 'steam://rungameid/578080',  NULL, 80),
('rivals', 'Marvel Rivals', 'Steam',      'game', 'MR',   NULL, '#a78bfa', '#4c1d95', 'steam://rungameid/2767030', NULL, 90),
('ow2',    'Overwatch 2',   'Battle.net', 'game', 'OW2',  NULL, '#fb923c', '#9a3412', 'battlenet://Pro',           NULL, 100)
ON CONFLICT (id) DO NOTHING;

-- Приложения. Для 'app' subtitle = папка на экране («Коммуникация»;
-- пусто = «Другие»; системные — своя папка из category='system').
INSERT INTO catalog_apps (id, name, subtitle, category, tag, emoji, color_a, color_b, target, args, sort) VALUES
('teamspeak', 'TeamSpeak',  'Коммуникация', 'app', NULL, '🎙', NULL, NULL, 'C:\Program Files\TeamSpeak 3 Client\ts3client_win64.exe', NULL, 25),
('faceit',    'FACEIT',     NULL,           'app', NULL, '🎯', NULL, NULL, 'https://www.faceit.com',                                  NULL, 35),
('obs',       'OBS Studio', NULL,           'app', NULL, '📹', NULL, NULL, 'C:\Program Files\obs-studio\bin\64bit\obs64.exe',         NULL, 45)
ON CONFLICT (id) DO NOTHING;

UPDATE catalog_apps SET subtitle = 'Коммуникация' WHERE id = 'discord';
UPDATE catalog_apps SET name = 'Google Chrome' WHERE id = 'browser' AND name = 'Браузер';
-- Убраны с экрана ещё в июле (осталось в БД с сида 011) — гасим и в базе.
UPDATE catalog_apps SET enabled = FALSE WHERE id IN ('youtube', 'telegram');

-- Система: настройки мыши (скорость, кнопки; ms-settings работает и без агента).
INSERT INTO catalog_apps (id, name, subtitle, category, tag, emoji, color_a, color_b, target, args, sort) VALUES
('mouse', 'Мышь', NULL, 'system', NULL, '🖱', NULL, NULL, 'ms-settings:mousetouchpad', NULL, 15)
ON CONFLICT (id) DO NOTHING;
