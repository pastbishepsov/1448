-- 059_aim_sessions.sql
-- С4-и1 (SENS.md, №14): история калибровок AI-калибратора (web/aim.html).
-- До этого результат жил только в памяти шелла и терялся с выходом гостя.
-- Теперь каждый прогон — строка: рекомендация (sens при dpi + cm/360 —
-- инвариант, сервер пересчитывает его сам из sens×dpi), вердикт слепого A/B,
-- моторика (реакция, движение, пропускная способность по Фиттсу, точность
-- первого выстрела), подсказка коуча (С4-и2) и условия прогона (режим, raw
-- input, Гц, частичный ли результат, тёплый ли старт).
-- sim = прогон синтетического стрелка (стенд, ?sim=1): хранится для
-- проверки конвейера, но в тёплый старт, аим-рейтинг и отчёт владельца не идёт.
-- Хранится не больше 200 последних строк на гостя (aim.go подрезает).

CREATE TABLE IF NOT EXISTS aim_sessions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    sens        NUMERIC(8,3) NOT NULL,                 -- рекомендованный sens (единицы CS2) при dpi
    dpi         INTEGER NOT NULL,
    cm360       NUMERIC(7,2) NOT NULL,                 -- см на оборот (инвариант)
    cur         NUMERIC(8,3) NOT NULL DEFAULT 0,       -- текущая сенса гостя на момент прогона
    cur_cm      NUMERIC(7,2) NOT NULL DEFAULT 0,
    lo          NUMERIC(8,3) NOT NULL DEFAULT 0,       -- диапазон оптимума
    hi          NUMERIC(8,3) NOT NULL DEFAULT 0,
    conf        VARCHAR(8)  NOT NULL DEFAULT 'low'  CHECK (conf IN ('high', 'mid', 'low')),
    verdict     VARCHAR(8)  NOT NULL DEFAULT 'none' CHECK (verdict IN ('rec', 'cur', 'tie', 'none')),
    gain_pct    NUMERIC(6,1) NOT NULL DEFAULT 0,
    acc1        SMALLINT,                              -- точность первого выстрела, %; NULL — не мерили
    rt_ms       INTEGER NOT NULL DEFAULT 0,
    mt_ms       INTEGER NOT NULL DEFAULT 0,
    tp          NUMERIC(6,2) NOT NULL DEFAULT 0,       -- пропускная способность, бит/с
    mode        VARCHAR(8)  NOT NULL DEFAULT 'custom' CHECK (mode IN ('quick', 'precise', 'custom')),
    rounds      SMALLINT NOT NULL DEFAULT 0,
    ab_rounds   SMALLINT NOT NULL DEFAULT 0,
    partial     SMALLINT NOT NULL DEFAULT 0,           -- > 0: прервано на N раундах из rounds
    raw         BOOLEAN,                               -- raw input; NULL — без захвата
    hz          INTEGER NOT NULL DEFAULT 0,
    coach       VARCHAR(8)  NOT NULL DEFAULT 'none' CHECK (coach IN ('over', 'under', 'none')),
    warm        BOOLEAN NOT NULL DEFAULT FALSE,
    lang        VARCHAR(2)  NOT NULL DEFAULT 'ru',
    sim         BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_aim_sessions_user_at ON aim_sessions (user_id, created_at DESC);
-- аим-рейтинг: лучший полный (не частичный) настоящий прогон каждого гостя
CREATE INDEX IF NOT EXISTS idx_aim_sessions_rating ON aim_sessions (user_id, tp DESC) WHERE sim = FALSE AND partial = 0;
