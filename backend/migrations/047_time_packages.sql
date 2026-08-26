-- 047: каталог пакетов времени (спринт Е2-и1, OPERATOR.md).
--
-- Лист основателя: «положить на аккаунт пакет времени (3 часа, 5 часов)».
-- Это НЕ пополнение кошелька (Р2): кошелёк — предоплаченные злотые, которые
-- тратятся по цене часа, а пакет продаётся номиналом («3 часа за 45 zł») и
-- живёт минутами. Гость, купивший пакет, платит за час меньше — в этом весь
-- смысл покупки; кошелёк такой скидки дать не может, не сломав Г-Р1.
--
-- Зона обязательна (Р10). Час VIP дороже часа STANDARD, и один номинал на обе
-- зоны означал бы, что гость покупает дешёвое, а отыгрывает дорогое. Поэтому
-- «3 часа STANDARD» и «3 часа VIP» — два разных пакета с разной ценой, а
-- универсального «куда хочешь» нет сознательно.
--
-- Срок годности — у каждого пакета свой (Р11): days_valid = 0 значит
-- бессрочно. Одной настройкой клуба это не сделать: акция «3 часа на неделю»
-- должна жить рядом с обычным бессрочным пакетом.
--
-- ON DELETE RESTRICT у зоны намеренно: зона, на которую продан пакет, не
-- должна исчезнуть под ним. Владелец сначала выключает пакет, потом зону.

CREATE TABLE IF NOT EXISTS time_packages (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id     UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    zone_id     UUID NOT NULL REFERENCES zones(id) ON DELETE RESTRICT,
    name        VARCHAR(64)   NOT NULL,
    minutes     INTEGER       NOT NULL CHECK (minutes > 0),
    price_pln   DECIMAL(8,2)  NOT NULL CHECK (price_pln > 0),
    days_valid  INTEGER       NOT NULL DEFAULT 0 CHECK (days_valid >= 0),
    sort        INTEGER       NOT NULL DEFAULT 0,
    active      BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- Ценник показывается в порядке sort → минуты: у стойки его читают глазами.
CREATE INDEX IF NOT EXISTS idx_time_packages_club ON time_packages(club_id, sort, minutes);

COMMENT ON TABLE  time_packages           IS 'Е2-и1: каталог пакетов времени — купленный запас минут в зоне (Р2, Р10)';
COMMENT ON COLUMN time_packages.zone_id   IS 'Е2: зона обязательна (Р10) — час VIP дороже часа STANDARD';
COMMENT ON COLUMN time_packages.days_valid IS 'Е2: срок годности выданных минут в днях; 0 — бессрочно (Р11)';

-- Выданные пакеты. Отдельная строка на каждую покупку — не счётчик на госте:
-- у каждой покупки свой срок (Р11), своя зона (Р10) и своя цена, а «сколько
-- минут осталось» без этого не разложить ни в отчёте, ни в споре с гостем.
--
-- Снимок имени, зоны и цены на момент покупки — по образцу coin_redemptions
-- (028): владелец переименует зону или поднимет цену, а проданное останется
-- тем, чем его продали.
--
-- Отменённая выдача не удаляется: voided_at выводит её из выручки и забирает
-- неотыгранный остаток, но след в журнале остаётся (образец В4-4).
CREATE TABLE IF NOT EXISTS user_packages (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id        UUID NOT NULL REFERENCES clubs(id)         ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id)         ON DELETE CASCADE,
    package_id     UUID          REFERENCES time_packages(id) ON DELETE RESTRICT,
    zone_id        UUID NOT NULL REFERENCES zones(id)         ON DELETE RESTRICT,
    name           VARCHAR(64)  NOT NULL,
    zone_name      VARCHAR(32)  NOT NULL DEFAULT '',
    minutes_total  INTEGER      NOT NULL CHECK (minutes_total > 0),
    minutes_left   INTEGER      NOT NULL CHECK (minutes_left >= 0),
    price_pln      DECIMAL(8,2) NOT NULL,
    method         VARCHAR(16)  NOT NULL DEFAULT 'cash',
    expires_at     TIMESTAMPTZ,
    created_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    voided_at      TIMESTAMPTZ,
    voided_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Биллинг спрашивает одно и то же: живые пакеты этого гостя в этой зоне,
-- ближайший к сгоранию первым.
CREATE INDEX IF NOT EXISTS idx_user_packages_live
    ON user_packages(user_id, zone_id, expires_at)
    WHERE voided_at IS NULL AND minutes_left > 0;

COMMENT ON TABLE  user_packages              IS 'Е2: выданный/купленный пакет минут — строка на покупку, со своим сроком и зоной';
COMMENT ON COLUMN user_packages.minutes_left IS 'Е2: неотыгранный остаток; по нему считается обязательство клуба';
COMMENT ON COLUMN user_packages.method       IS 'Е2: cash|card|blik — выручка; wallet — погашение обязательства (Г-Р7)';
COMMENT ON COLUMN user_packages.expires_at   IS 'Е2: NULL — бессрочно (days_valid=0 у пакета, Р11)';
