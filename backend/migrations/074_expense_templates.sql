-- 074 — З9-и5 (OWNER.md, трек З; З-Р30, ревью 23.09 №67, Д10; решение основателя 28.09 — только ежемесячные):
-- регулярные расходы.
--
-- expense_templates — шаблон «аренда 1-го числа 4 500 zł, банк»: название, категория, контрагент, сумма брутто и
-- VAT, способ, число месяца (нет такого числа в месяце — последний день), заметка. from_month — первый месяц, за
-- который шаблон ждёт расход (ближайший срок после заведения или включения); reminded_for — месяц, о котором уже
-- напомнили в канал «только владельцу» (раз на срок). Выключенный шаблон (active = false) не ждёт и не напоминает.
-- Удаления нет — выключить.
--
-- expenses.template_id / template_month — расход внесён по шаблону за этот месяц (первое число месяца). За месяц —
-- один несторнированный расход по шаблону (уникальный индекс); сторно освобождает месяц — шаблон снова ждёт.
--
-- Идемпотентна: таблица, индексы и колонки — IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS expense_templates (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    club_id         UUID NOT NULL REFERENCES clubs(id) ON DELETE CASCADE,
    name            VARCHAR(64) NOT NULL,
    category_id     UUID NOT NULL REFERENCES expense_categories(id),
    counterparty_id UUID REFERENCES counterparties(id),
    gross_grosz     BIGINT NOT NULL CHECK (gross_grosz > 0),
    vat_grosz       BIGINT NOT NULL DEFAULT 0 CHECK (vat_grosz >= 0 AND vat_grosz <= gross_grosz),
    method          VARCHAR(8) NOT NULL CHECK (method IN ('till', 'bank', 'card', 'owner')),
    day             SMALLINT NOT NULL CHECK (day BETWEEN 1 AND 31),
    note            TEXT NOT NULL DEFAULT '',
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    from_month      DATE NOT NULL,
    reminded_for    DATE,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_expense_templates_name ON expense_templates (lower(name));

ALTER TABLE expenses ADD COLUMN IF NOT EXISTS template_id UUID REFERENCES expense_templates(id) ON DELETE SET NULL;
ALTER TABLE expenses ADD COLUMN IF NOT EXISTS template_month DATE;
CREATE UNIQUE INDEX IF NOT EXISTS uq_expenses_template_month ON expenses (template_id, template_month)
    WHERE template_id IS NOT NULL AND storno_at IS NULL;

COMMENT ON TABLE expense_templates IS 'З9-и5: регулярные расходы — шаблон раз в месяц, «Ждут внесения», напоминание владельцу';
COMMENT ON COLUMN expenses.template_month IS 'З9-и5: за какой месяц (первое число) расход внесён по шаблону template_id';
