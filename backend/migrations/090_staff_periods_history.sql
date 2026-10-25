-- 090 — З15-и5а (OWNER.md, трек З; №30; З-Р20): ранние периоды работы — из журнала.
--
-- 089 завела каждой карточке один период из её дат. Но тем, кого до неё уже возвращали («Назначить» бывшего
-- сотрудника, в журнале — «ник → admin (вернулся)»), прежний код стирал увольнение из карточки, и их период склеил всё
-- подряд: с первого найма по сегодня или по последнее увольнение — без прежних увольнений и причин. Теперь такой период
-- делится по журналу:
--   * увольнение («ник · уволен ДАТА …») закрывает период этой датой и причиной (staff_dismiss_reason из 089);
--   * назначение после увольнения открывает новый — с клубных суток строки назначения, как «Назначить» сейчас: сутки —
--     с report_hour по часам клуба (Europe/Warsaw — CLUB_TZ по умолчанию; миграции идут сессией psql в UTC);
--   * начало периода, в котором «Работает с» в последний раз ставили (наём «с ДАТА», правка карточки «дата найма: ДАТА»,
--     заведение карточки), — из карточки; в периоде, где его ставили раньше, — что тогда поставили (дату заведения
--     карточки журнал не пишет — неизвестно); где не ставили — день возвращения, у первого — неизвестно, а если
--     «Работает с» не ставили ни разу (карточка старше журнала) — из карточки;
--   * два увольнения подряд, без назначения между ними, — между ними период с неизвестным началом;
--   * начало позже конца (уволили задним числом раньше дня возвращения) — начало = конец.
-- Последний из периодов — тот, что завела 089: его конец и причину не трогаем (их мог поставить уже новый код), меняется
-- только начало. Если он у человека последний, «Работает с» в карточке — его начало: карточка хранит последний период, а
-- иначе первая же правка карточки вернула бы ему дату первого найма. Правки карточки после следующего периода
-- (возвращение уже новым кодом) относятся к тому периоду и здесь не учитываются.
--
-- Идемпотентна: восстановленные периоды заведены временем своих строк журнала, возвращений раньше самого раннего периода
-- у человека не остаётся — повторный прогон никого не трогает.

DO $$
DECLARE
    tz       CONSTANT TEXT := 'Europe/Warsaw';
    rh       INT;
    u        RECORD;          -- самый ранний период человека — тот, что завела 089
    r        RECORD;          -- строка журнала
    next_at  TIMESTAMPTZ;     -- когда завели следующий период (NULL — этот последний)
    k        INT;             -- номер текущего периода
    closed   BOOLEAN;         -- текущий период закрыт увольнением
    last_h   INT;             -- в каком периоде «Работает с» ставили последний раз
    s_at     TIMESTAMPTZ[];   -- created_at периода — время строки, которой он начался
    s_def    DATE[];          -- начало по умолчанию: день возвращения (NULL — неизвестно)
    s_set    BOOLEAN[];       -- в периоде ставили «Работает с»
    s_val    DATE[];          -- что поставили
    s_end    DATE[];
    s_reason TEXT[];
    d        DATE;
    st       DATE;
    en       DATE;
    people   INT := 0;
    added    INT := 0;
BEGIN
    SELECT value::int INTO rh FROM settings WHERE key = 'report_hour' AND value ~ '^[0-9]{1,2}$';
    rh := coalesce(rh, 8);

    FOR u IN
        SELECT f.*
          FROM (SELECT DISTINCT ON (user_id) id, user_id, hired_on, dismissed_on, reason, created_at
                  FROM staff_periods
                 ORDER BY user_id, created_at, id) f
         WHERE EXISTS (SELECT 1 FROM admin_actions a
                        WHERE a.target_user_id = f.user_id
                          AND a.action = 'staff_promote'
                          AND a.details LIKE '% → admin (вернулся)'
                          AND a.created_at < f.created_at)
    LOOP
        SELECT min(created_at) INTO next_at FROM staff_periods WHERE user_id = u.user_id AND created_at > u.created_at;
        k := 1; closed := false; last_h := NULL;
        s_at := ARRAY[NULL]::timestamptz[]; s_def := ARRAY[NULL]::date[]; s_set := ARRAY[false];
        s_val := ARRAY[NULL]::date[]; s_end := ARRAY[NULL]::date[]; s_reason := ARRAY['']::text[];

        FOR r IN
            SELECT a.action, a.details, a.created_at
              FROM admin_actions a
             WHERE a.target_user_id = u.user_id
               AND ((a.action IN ('staff_dismiss', 'staff_promote') AND a.created_at < u.created_at)
                 OR (a.action IN ('staff_hire', 'staff_card') AND (next_at IS NULL OR a.created_at < next_at)))
             ORDER BY a.created_at, a.id
        LOOP
            s_at[1] := coalesce(s_at[1], r.created_at);
            IF r.action = 'staff_dismiss' THEN
                d := substring(r.details FROM ' · уволен (\d{4}-\d{2}-\d{2})')::date;
                CONTINUE WHEN d IS NULL;
                IF closed THEN   -- уволен снова без назначения между: работал, а с какого дня — журнал не знает
                    k := k + 1;
                    s_at[k] := r.created_at; s_def[k] := NULL; s_set[k] := false; s_val[k] := NULL;
                END IF;
                s_end[k] := d;
                s_reason[k] := coalesce(staff_dismiss_reason(r.details, d), '');
                closed := true;
            ELSIF r.action = 'staff_promote' THEN
                IF closed THEN   -- назначен после увольнения — вернулся (снятый с прав и назначенный снова — тот же период)
                    k := k + 1;
                    s_at[k] := r.created_at; s_set[k] := false; s_val[k] := NULL;
                    s_def[k] := ((r.created_at AT TIME ZONE tz) - make_interval(hours => rh))::date;
                    s_end[k] := NULL; s_reason[k] := '';
                    closed := false;
                END IF;
            ELSIF r.action = 'staff_hire' THEN
                s_set[k] := true; last_h := k;
                s_val[k] := substring(r.details FROM ' · с (\d{4}-\d{2}-\d{2})$')::date;
            ELSIF r.details ~ '(: |, )дата найма: (\d{4}-\d{2}-\d{2})?(,|$)' THEN
                s_set[k] := true; last_h := k;   -- пустая дата — «Работает с» стёрли
                s_val[k] := (regexp_match(r.details, '(?:: |, )дата найма: (\d{4}-\d{2}-\d{2})?(?:,|$)'))[1]::date;
            ELSIF r.details LIKE '%: карточка заведена' THEN
                s_set[k] := true; last_h := k; s_val[k] := NULL;
            END IF;
        END LOOP;
        CONTINUE WHEN k = 1;   -- увольнения перед возвращением в журнале нет — делить нечем

        people := people + 1;
        FOR i IN 1..k LOOP
            st := CASE WHEN i = last_h THEN u.hired_on
                       WHEN s_set[i] THEN s_val[i]
                       WHEN i = 1 THEN CASE WHEN last_h IS NULL THEN u.hired_on END
                       ELSE s_def[i] END;
            en := CASE WHEN i = k THEN u.dismissed_on ELSE s_end[i] END;
            IF st > en THEN
                st := en;
            END IF;
            IF i < k THEN
                INSERT INTO staff_periods (user_id, hired_on, dismissed_on, reason, created_at, updated_at)
                VALUES (u.user_id, st, en, s_reason[i], s_at[i], now());
                added := added + 1;
            ELSE
                UPDATE staff_periods SET hired_on = st, updated_at = now()
                 WHERE id = u.id AND hired_on IS DISTINCT FROM st;
                IF next_at IS NULL THEN
                    UPDATE staff_profiles SET hired_at = st, updated_at = now()
                     WHERE user_id = u.user_id AND hired_at IS DISTINCT FROM st;
                END IF;
            END IF;
        END LOOP;
    END LOOP;

    RAISE NOTICE '090: ранние периоды работы восстановлены у % чел., добавлено периодов: %', people, added;
END $$;
