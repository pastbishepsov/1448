-- 089 — З15-и5 (OWNER.md, трек З; №30; З-Р20): периоды работы сотрудника.
--
-- Карточка хранила одну пару «нанят — уволен», и повторный наём стирал увольнение: dismissed_at обнулялся, человек
-- уходил из «Уволенных», а то, что он уже работал и уходил (когда и почему), оставалось только в журнале. Теперь у
-- каждого периода работы своя строка: с какого дня, по какой и причина увольнения. Открытый период (dismissed_on IS
-- NULL) у человека один; карточка (staff_profiles.hired_at / dismissed_at) показывает последний.
--
-- Заполнение: каждой карточке — один период из её дат; причина закрытого — из последней строки журнала об увольнении с
-- той же датой («ник · уволен ДАТА[ (последний рабочий день прошёл)] · причина[, смена с … закрыта (…), касса …][,
-- снят с N будущих смен]»), стёртая («—») — пусто. Причину из строки достаёт staff_dismiss_reason — она же нужна
-- проверке. Ранние периоды тех, кого уже нанимали повторно, не восстанавливаются: прежнее увольнение у них стёрто из
-- карточки.
--
-- Идемпотентна: таблица и индексы — IF NOT EXISTS, функция — CREATE OR REPLACE, заполнение — только тем, у кого
-- периодов ещё нет.

CREATE TABLE IF NOT EXISTS staff_periods (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hired_on     DATE,                       -- NULL — дата найма неизвестна
    dismissed_on DATE,                       -- NULL — работает сейчас
    reason       TEXT NOT NULL DEFAULT '',   -- причина увольнения
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_staff_periods_user ON staff_periods (user_id);
-- у человека не больше одного открытого периода
CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_periods_open ON staff_periods (user_id) WHERE dismissed_on IS NULL;

-- staff_dismiss_reason — причина из строки журнала об увольнении в день day; причины нет или она стёрта — NULL.
CREATE OR REPLACE FUNCTION staff_dismiss_reason(details TEXT, day DATE)
RETURNS TEXT
LANGUAGE sql STABLE PARALLEL SAFE
AS $$
    SELECT nullif(btrim(regexp_replace(regexp_replace(
               (regexp_match(details, ' · уволен ' || to_char(day, 'YYYY-MM-DD')
                                      || '(?: \(последний рабочий день прошёл\))? · (.*)$'))[1],
               ', снят с \d+ будущих смен$', ''),
               ', смена с [^,]+ закрыта \([^)]*\), касса (?:не сдана|без пересчёта)$', '')), '—')
$$;

INSERT INTO staff_periods (user_id, hired_on, dismissed_on, reason)
SELECT p.user_id, p.hired_at, p.dismissed_at,
       COALESCE((SELECT staff_dismiss_reason(a.details, p.dismissed_at)
                   FROM admin_actions a
                  WHERE p.dismissed_at IS NOT NULL
                    AND a.action = 'staff_dismiss'
                    AND a.target_user_id = p.user_id
                    AND position(' · уволен ' || to_char(p.dismissed_at, 'YYYY-MM-DD') IN a.details) > 0
                  ORDER BY a.created_at DESC
                  LIMIT 1), '')
  FROM staff_profiles p
 WHERE NOT EXISTS (SELECT 1 FROM staff_periods s WHERE s.user_id = p.user_id);
