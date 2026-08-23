-- 025: карточка сотрудника (спринт В3 трека владельца, этап 2; ADMIN.md).
-- До этого сотрудник в системе был только «ник + роль»: ни ФИО, ни даты найма,
-- ни должности, ни ставки. Карточка живёт ОТДЕЛЬНОЙ таблицей, а не колонками в
-- users, по двум причинам: (1) users — это профиль гостя, он светится на
-- экранах в клубе и в лидербордах, личным данным сотрудника там не место;
-- (2) карточку видит и правит только владелец, и отдельная таблица делает эту
-- границу явной, а не проверкой в каждом ответе.
--
-- Персональные данные (ФИО, телефон) НЕ попадают ни в один гостевой ответ, ни
-- в строки аудита — там остаётся ник. Ставка хранится как тип + сумма, чтобы
-- одинаково лечь и на почасовую, и на посменную, и на оклад.

CREATE TABLE IF NOT EXISTS staff_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    full_name    VARCHAR(128) NOT NULL DEFAULT '',
    phone        VARCHAR(32)  NOT NULL DEFAULT '',
    position     VARCHAR(64)  NOT NULL DEFAULT '',
    hired_at     DATE,
    dismissed_at DATE,                              -- заполняется при увольнении (этап 3)
    rate_type    VARCHAR(16)  NOT NULL DEFAULT 'none', -- none | hour | shift | month
    rate_amount  DECIMAL(10,2) NOT NULL DEFAULT 0,
    note         TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE staff_profiles IS 'Кадровая карточка сотрудника: только для владельца (спринт В3)';
COMMENT ON COLUMN staff_profiles.rate_type IS 'none | hour | shift | month — как считается оплата';
