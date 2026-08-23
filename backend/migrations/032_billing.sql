-- 032: поминутное списание кошелька за сессию (трек Г, спринт Г1; GUEST.md,
-- решения Р2/Р8) + минутный запас из монет (Г1-и5, стык с В4).
--
-- Заметка основателя становится правдой: «начинает идти время и убывать
-- коинсы». Сервер раз в polтминуты доначисляет стоимость прошедших минут по
-- ставке сессии (цена зоны минус скидки — как считалось и раньше), деньги
-- уходят из users.wallet_grosz строками журнала wallet_transactions
-- (kind=session_spend). Клиент ничего не считает и ничего не решает (Р2).
--
-- Минутный запас: админский обмен монет на время (В4, coin_redemptions)
-- теперь кладёт минуты в users.coin_minutes — биллинг тратит их ПЕРЕЖДЕ
-- денег. Так буква решения «монеты гасятся только временем» сохраняется,
-- а выданное время наконец учитывается автоматически, а не на честном слове.
--
-- Учёт на сессии (все поля меняет только биллинг, транзакционно):
--   billed_minutes    — минут учтено всего (монетные + денежные);
--   coin_minutes_used — из них покрыто минутным запасом;
--   money_minutes     — из них оплачено кошельком;
--   charged_grosz     — сколько денег списано за сессию (сумма session_spend);
--   warn15_at/warn5_at — когда отправлены предупреждения «осталось ~15/5 мин»
--                        (по одному разу на сессию);
--   zero_since        — когда кошелёк упёрся в ноль (старт грейса);
--   ended_reason      — почему сессия закончилась: manual | admin | balance
--                        (дальше добавятся booking Г3 и afk Г2).
-- Депозит и обмен монет во время сессии СБРАСЫВАЮТ warn*/zero_since — гость
-- додепнул, предупреждения начинаются заново.

ALTER TABLE users ADD COLUMN IF NOT EXISTS coin_minutes BIGINT NOT NULL DEFAULT 0;

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS billed_minutes    INT    NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS coin_minutes_used INT    NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS money_minutes     INT    NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS charged_grosz     BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS warn15_at   TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS warn5_at    TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS zero_since  TIMESTAMPTZ;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS ended_reason VARCHAR(16);

-- Настройки биллинга (правило проекта: настройка = миграция + settingBounds +
-- currentSettings в settings.go).
INSERT INTO settings (key, value) VALUES
    ('min_start_minutes', '15'),  -- порог старта: кошелёк+минуты должны покрывать столько минут
    ('zero_grace_min', '2')       -- грейс на нуле до автозавершения, минут (0 = сразу)
ON CONFLICT (key) DO NOTHING;

COMMENT ON COLUMN users.coin_minutes IS 'Минутный запас из обмена монет (В4 redeem): биллинг Г1 тратит его раньше кошелька';
COMMENT ON COLUMN sessions.ended_reason IS 'Причина завершения: manual | admin | balance (Г1); booking (Г3), afk (Г2) — позже';
