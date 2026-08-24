-- 033: пауза сессии и AFK (трек Г, спринт Г2; GUEST.md, этап I).
--
-- Пауза: гость отходит (туалет/перекур) — время, деньги (биллинг Г1) и XP
-- стоят, ПК остаётся за ним. Лимит пауз — НА СЕССИЮ (настройка
-- pause_limit_min, деф. 15 мин, 0 = пауза выключена): по исчерпании биллинг
-- сам снимает паузу и время снова тикает. В плане лимит звался «дневным» —
-- дневной учёт честно появится вместе с user_progress (Г5); лимит на сессию
-- строже к абьюзу «держать ПК бесплатно» и не требует новой таблицы.
--
-- AFK: агент шлёт в session_tick секунды простоя ввода (GetLastInputInfo).
-- Порог afk_stop_min (деф. 10 мин, 0 = выкл): предупреждение afk_warn, затем
-- автопауза от системы (paused_by=afk), если паузный бюджет ещё есть; гость
-- вернулся — пауза снимается сама; бюджета нет — завершение ended_reason=afk.
-- Без агента (нет датчика) AFK не судим.
--
-- active_minutes — минуты, прожитые НЕ в простое (или без датчика): база
-- анти-фарма периодических ачивок (Г5): «просидел 8 часов» нельзя выфармить
-- отойдя от ПК.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS paused_at        TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS paused_total_sec INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS paused_by        VARCHAR(8); -- guest | afk
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS active_minutes   INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS afk_warned_at    TIMESTAMPTZ;

INSERT INTO settings (key, value) VALUES
    ('pause_limit_min', '15'),  -- лимит паузы на сессию, мин; 0 = пауза выключена
    ('afk_stop_min', '10')      -- порог простоя до AFK-реакции, мин; 0 = выкл
ON CONFLICT (key) DO NOTHING;

COMMENT ON COLUMN sessions.paused_total_sec IS 'Суммарная пауза сессии, сек (Г2): биллинг и XP считают время БЕЗ неё';
COMMENT ON COLUMN sessions.active_minutes IS 'Минуты без простоя (Г2): анти-фарм для периодических ачивок Г5';
