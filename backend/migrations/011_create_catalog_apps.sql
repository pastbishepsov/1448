-- 011_create_catalog_apps.sql
-- Каталог приложений гостевого экрана. Управляется из Admin Panel (ТЗ 6.2).
-- target/args выполняет shell-agent на гостевом ПК (только по этому allowlist).

CREATE TABLE catalog_apps (
    id         VARCHAR(32) PRIMARY KEY,          -- slug: cs2, steam, sound...
    name       VARCHAR(64) NOT NULL,
    subtitle   VARCHAR(64),
    category   VARCHAR(16) NOT NULL CHECK (category IN ('game', 'app', 'system')),
    tag        VARCHAR(12),                      -- крупная надпись на тайле игры
    emoji      VARCHAR(8),                       -- иконка для app/system
    color_a    VARCHAR(9),                       -- градиент тайла игры
    color_b    VARCHAR(9),
    target     TEXT,                             -- exe-путь / протокол / ms-settings:
    args       JSONB,                            -- аргументы запуска
    sort       INTEGER NOT NULL DEFAULT 100,     -- меньше = выше
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Сид: стандарт клуба (совпадает с прежним встроенным каталогом).
INSERT INTO catalog_apps (id, name, subtitle, category, tag, emoji, color_a, color_b, target, args, sort) VALUES
('cs2',      'Counter-Strike 2',  'Steam',      'game', 'CS2',  NULL, '#f59e0b', '#ef4444', 'steam://rungameid/730',    NULL, 10),
('dota2',    'Dota 2',            'Steam',      'game', 'DOTA', NULL, '#ef4444', '#7c5cff', 'steam://rungameid/570',    NULL, 20),
('valorant', 'Valorant',          'Riot Games', 'game', 'VAL',  NULL, '#fb7185', '#be123c', 'C:\Riot Games\Riot Client\RiotClientServices.exe', '["--launch-product=valorant","--launch-patchline=live"]', 30),
('fortnite', 'Fortnite',          'Epic Games', 'game', 'FN',   NULL, '#22d3ee', '#6366f1', 'com.epicgames.launcher://apps/Fortnite?action=launch&silent=true', NULL, 40),
('lol',      'League of Legends', 'Riot Games', 'game', 'LoL',  NULL, '#fbbf24', '#0ea5e9', 'C:\Riot Games\Riot Client\RiotClientServices.exe', '["--launch-product=league_of_legends","--launch-patchline=live"]', 50),
('gta5',     'GTA V',             'Rockstar',   'game', 'GTA',  NULL, '#34d399', '#0f766e', 'steam://rungameid/271590', NULL, 60),
('steam',    'Steam',    NULL, 'app', NULL, '🎮',  NULL, NULL, 'steam://open/main',       NULL, 10),
('discord',  'Discord',  NULL, 'app', NULL, '💬',  NULL, NULL, 'discord://',              NULL, 20),
('browser',  'Браузер',  NULL, 'app', NULL, '🌐',  NULL, NULL, 'https://www.google.com',  NULL, 30),
('spotify',  'Spotify',  NULL, 'app', NULL, '🎵',  NULL, NULL, 'spotify:',                NULL, 40),
('youtube',  'YouTube',  NULL, 'app', NULL, '▶️',  NULL, NULL, 'https://www.youtube.com', NULL, 50),
('telegram', 'Telegram', NULL, 'app', NULL, '✈️',  NULL, NULL, 'https://web.telegram.org', NULL, 60),
('sound',    'Звук',     NULL, 'system', NULL, '🔊', NULL, NULL, 'sndvol.exe',          NULL, 10),
('display',  'Дисплей',  NULL, 'system', NULL, '🖥️', NULL, NULL, 'ms-settings:display', NULL, 20),
('nvidia',   'NVIDIA',   NULL, 'system', NULL, '🟢', NULL, NULL, 'C:\Program Files\NVIDIA Corporation\NVIDIA app\CEF\NVIDIA app.exe', NULL, 30);
