-- 100 — З24-и1 (OWNER.md, трек З; №53 «в», З-Р28): расписание цен зоны — «с 18:00 дороже», «выходные дешевле».
--
-- zone_price_rules — правило цены часа зоны: дни недели (days_mask — пн = 1, вт = 2 … вс = 64, как у шаблонов смен),
-- окно по стенным часам клуба (start_min — начало, end_min — конец, 1440 — до полуночи; конец не позже начала — окно
-- через полночь и принадлежит дню начала), цена часа (rate_pln), срок действия — даты дня начала окна (valid_from и
-- valid_to включительно, пусто — без границы), заметка («happy hours»). Вне правил зона стоит своей ценой
-- (zones.rate_pln). Правила одной зоны не накладываются — это проверяет сервер (409 rule_overlap), в один момент у
-- зоны одна цена.
--
-- Цена фиксируется на посадке: сессия хранит её снимком (sessions.base_rate_pln и effective_rate_pln, как и было),
-- граница правила посреди игры её не меняет. Пакеты, монеты и минуты зоны — по цене зоны, правила их не трогают.
-- Зону удалят — её правила уйдут с ней.
--
-- Идемпотентна: IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS zone_price_rules (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    zone_id    UUID NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    days_mask  SMALLINT NOT NULL CHECK (days_mask BETWEEN 1 AND 127),
    start_min  SMALLINT NOT NULL CHECK (start_min BETWEEN 0 AND 1439),
    end_min    SMALLINT NOT NULL CHECK (end_min BETWEEN 1 AND 1440),
    rate_pln   DECIMAL(8,2) NOT NULL CHECK (rate_pln > 0),
    valid_from DATE,
    valid_to   DATE,
    note       VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT zone_price_rules_window CHECK (start_min <> end_min),
    CONSTRAINT zone_price_rules_dates CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from <= valid_to)
);
CREATE INDEX IF NOT EXISTS idx_zone_price_rules_zone ON zone_price_rules (zone_id);

COMMENT ON TABLE zone_price_rules IS 'З24-и1: расписание цены часа зоны — дни, окно по стенным часам, цена, срок действия; не накладываются';
