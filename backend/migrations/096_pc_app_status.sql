-- 096 — З20-и1 (OWNER.md, трек З; №57 «Где не установлено», обязательно): сервер принимает статус приложений.
--
-- Агент ПК знает, какие приложения каталога у него запустятся: GET /apps-status отдаёт киоску доступность по каждому
-- (файла нет — target_missing, шара или том не отвечают — path_unreachable, в каталоге пусто — no_target, ещё не
-- проверял — checking). Сервер этого не знал: клуб видел «игра не запустилась» только от гостя. Теперь агент шлёт тот же
-- снимок по WS (тип apps_status, с З20-и2), сервер хранит последний по каждому ПК: строка — ПК × приложение каталога,
-- доступно ли, почему нет и когда пришло. Новый снимок заменяет прежний целиком. ПК или запись каталога удалили —
-- их строки уходят вместе с ними. Показывать — З20-и3 (каталог и шторка ПК).
--
-- Идемпотентна: IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS pc_app_status (
    computer_id UUID        NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    app_id      VARCHAR(32) NOT NULL REFERENCES catalog_apps (id) ON DELETE CASCADE,
    available   BOOLEAN     NOT NULL,
    reason      VARCHAR(32) NOT NULL DEFAULT '',
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (computer_id, app_id)
);

-- «нет на PC-03, PC-07» (З20-и3) — по приложению
CREATE INDEX IF NOT EXISTS idx_pc_app_status_app ON pc_app_status (app_id);

COMMENT ON TABLE pc_app_status IS 'З20-и1: последний снимок доступности приложений каталога по ПК (agent → apps_status по WS)';
COMMENT ON COLUMN pc_app_status.reason IS 'почему недоступно: target_missing, path_unreachable, no_target; checking — агент ещё не проверял';
