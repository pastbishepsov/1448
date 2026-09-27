-- 071 — З9-и1 (OWNER.md, трек З; З-Р29, З-Р30, ревью 23.09 №61, №62, №65, №66): расходы клуба.
--
-- expense_categories — справочник категорий: 11 стандартных (code — стабильный ключ для
-- выгрузки и «Прибыли»), свои владелец заводит в З9-и4 (code NULL). purchase — закупка
-- товаров или кухни: в «Прибыли» это не расход, а себестоимость проданного (Д9). hidden —
-- скрытая не предлагается (З9-и4).
--
-- counterparties — кому платим: имя, NIP (10 цифр; контрольная сумма — З9-и4), контакты.
--
-- expenses — расход: день операции (календарь клуба), оплачен ли, категория, контрагент,
-- сумма брутто и VAT в грошах (VAT ≤ брутто), способ — из кассы (till), банк, карта, личные
-- деньги владельца (owner), номер документа, заметка, фото документа (как у товара — байты
-- читаются только отдельной ручкой), поставка (З10). Удаления нет — сторно: storno_at,
-- кто и почему; сторнированный расход из сумм уходит, строка остаётся.
--
-- Идемпотентна: таблицы и индексы — IF NOT EXISTS, стандартные категории — ON CONFLICT
-- DO NOTHING (повторный прогон и правка названия владельцем их не дублируют).

CREATE TABLE IF NOT EXISTS expense_categories (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code       VARCHAR(24),
    name       VARCHAR(64) NOT NULL,
    purchase   BOOLEAN NOT NULL DEFAULT FALSE,
    hidden     BOOLEAN NOT NULL DEFAULT FALSE,
    sort       INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_expense_categories_code ON expense_categories (code);
CREATE UNIQUE INDEX IF NOT EXISTS uq_expense_categories_name ON expense_categories (lower(name));

INSERT INTO expense_categories (code, name, purchase, sort) VALUES
    ('rent',      'Аренда',                FALSE,  10),
    ('utilities', 'Коммуналка',            FALSE,  20),
    ('internet',  'Интернет и связь',      FALSE,  30),
    ('goods',     'Закупка товаров',       TRUE,   40),
    ('kitchen',   'Закупка кухни',         TRUE,   50),
    ('salary',    'Зарплата',              FALSE,  60),
    ('zus_tax',   'ZUS и налоги',          FALSE,  70),
    ('equipment', 'Оборудование и ремонт', FALSE,  80),
    ('software',  'ПО и подписки',         FALSE,  90),
    ('marketing', 'Маркетинг',             FALSE, 100),
    ('other',     'Прочее',                FALSE, 110)
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS counterparties (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(128) NOT NULL,
    nip        VARCHAR(10),
    contact    VARCHAR(200) NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    hidden     BOOLEAN NOT NULL DEFAULT FALSE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_counterparties_name ON counterparties (lower(name));

CREATE TABLE IF NOT EXISTS expenses (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id         UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    op_date         DATE NOT NULL,
    paid            BOOLEAN NOT NULL DEFAULT TRUE,
    category_id     UUID NOT NULL REFERENCES expense_categories(id),
    counterparty_id UUID REFERENCES counterparties(id),
    gross_grosz     BIGINT NOT NULL CHECK (gross_grosz > 0),
    vat_grosz       BIGINT NOT NULL DEFAULT 0 CHECK (vat_grosz >= 0 AND vat_grosz <= gross_grosz),
    method          VARCHAR(8) NOT NULL CHECK (method IN ('till', 'bank', 'card', 'owner')),
    doc_no          VARCHAR(64) NOT NULL DEFAULT '',
    note            TEXT NOT NULL DEFAULT '',
    photo           BYTEA,
    photo_type      VARCHAR(16),
    photo_at        TIMESTAMPTZ,
    supply_id       UUID,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    storno_at       TIMESTAMPTZ,
    storno_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    storno_reason   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_expenses_date ON expenses (op_date);
CREATE INDEX IF NOT EXISTS idx_expenses_category ON expenses (category_id, op_date);

COMMENT ON TABLE expense_categories IS 'З9-и1: категории расходов — 11 стандартных (code) и свои (З9-и4)';
COMMENT ON COLUMN expense_categories.purchase IS 'закупка: в «Прибыли» — себестоимость, а не расход (Д9)';
COMMENT ON TABLE counterparties IS 'З9-и1: контрагенты расходов';
COMMENT ON TABLE expenses IS 'З9-и1: расходы клуба; удаления нет — сторно (storno_at)';
COMMENT ON COLUMN expenses.method IS 'till — из кассы (ящик), bank, card — карта клуба, owner — личные деньги владельца';
COMMENT ON COLUMN expenses.supply_id IS 'З10: поставка, из которой расход (ссылка появится с таблицей поставок)';
