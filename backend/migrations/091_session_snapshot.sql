-- 091 — З16-и1 (OWNER.md, трек З; №37, №38 «б»; З-Р25): сессия помнит условия — снимок зоны и ставок XP и монет.
--
-- Тариф сессии снимается на старте (base_rate_pln, effective_rate_pln; ставка VAT — 083), а зона и ставки начисления —
-- нет: XP и монеты считались при завершении по ставкам этой минуты (ревью, сценарий M: coins_per_min 2 → 100 за минуту
-- до конца — 1000 монет за 10 минут), а пакеты, гранты и отчёт «Загрузка» брали нынешнюю зону ПК. Теперь у сессии —
-- снимок на начало: зона ПК (id и имя; имя остаётся, если зону потом переименуют или удалят; ПК без зоны — NULL и
-- пустое имя) и ставки xp_per_min и coins_per_min из настроек. Пишет сервер при посадке и старте; пересадка снимок не
-- меняет — как и тариф. По снимку начисляет finishSession; биллинг, прогноз и отчёты переходят на него в З16-и2 и З17.
--
-- Старым сессиям — нынешняя зона их ПК и нынешние ставки (10 и 2 — значения кода, если в settings их нет): истории зон
-- ПК нет (в журнале смены зоны нет ни прежней зоны, ни имени ПК), а XP и монеты завершённых уже начислены — у них
-- снимок справочный; идущие досчитаются по ставкам на момент миграции. Ставки NULL после неё — сессия, которую успела
-- начать прежняя версия сервера, пока поднималась новая: начисление возьмёт нынешние.
--
-- Идемпотентна: колонки — IF NOT EXISTS, заполняются только сессии без снимка ставок — повторный прогон трогает лишь
-- начатые прежней версией сервера.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS zone_id       UUID REFERENCES zones(id) ON DELETE SET NULL;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS zone_name     VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS xp_per_min    INTEGER;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS coins_per_min INTEGER;

-- отчёты по зонам (З17) и удаление зоны (ON DELETE SET NULL) ищут сессии по зоне
CREATE INDEX IF NOT EXISTS idx_sessions_zone ON sessions (zone_id);

-- зона — первой: «без снимка» узнаём по ставкам, которые заполняются следом
UPDATE sessions s SET zone_id = c.zone_id, zone_name = COALESCE(z.name, '')
  FROM computers c LEFT JOIN zones z ON z.id = c.zone_id
 WHERE c.id = s.computer_id AND s.xp_per_min IS NULL;

UPDATE sessions SET
    xp_per_min    = COALESCE(xp_per_min,
                             (SELECT value::int FROM settings WHERE key = 'xp_per_min' AND value ~ '^[0-9]{1,9}$'), 10),
    coins_per_min = COALESCE(coins_per_min,
                             (SELECT value::int FROM settings WHERE key = 'coins_per_min' AND value ~ '^[0-9]{1,9}$'), 2)
 WHERE xp_per_min IS NULL OR coins_per_min IS NULL;

COMMENT ON COLUMN sessions.zone_id IS 'З16-и1: снимок зоны ПК на начало сессии; NULL — ПК без зоны или зону потом удалили (имя — в zone_name)';
COMMENT ON COLUMN sessions.zone_name IS 'З16-и1: снимок имени зоны ПК на начало сессии; пусто — ПК без зоны';
COMMENT ON COLUMN sessions.xp_per_min IS 'З16-и1: снимок ставки XP за минуту на начало сессии — по ней начисление; NULL — сессия прежней версии сервера';
COMMENT ON COLUMN sessions.coins_per_min IS 'З16-и1: снимок ставки монет за минуту на начало сессии — по ней начисление; NULL — сессия прежней версии сервера';
