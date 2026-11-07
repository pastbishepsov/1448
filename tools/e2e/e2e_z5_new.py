#!/usr/bin/env python3
"""З5-и1 «„Новый гость“ одним определением» (№11 «а», З-Р10, Д2): новый на момент T — гость, чьё первое появление
(первая засчитанная сессия, а если сессий нет — регистрация) не раньше seg_new_days суток до T. T — конец периода
в отчёте «Гости», конец смены в сводке, «сейчас» в фильтре «новые» у стойки. Раньше определений было три
(первая сессия в окне отчёта, регистрация за смену, «аккаунт моложе недели»), а seg_new_days ни на что не
влиял. Guest36: первая сессия за 5 суток до дня D и сессия в D; Guest37: первая сессия в D; D — пустые сутки
прошлого. Идемпотентен: сессии удаляются, регистрация Guest36 и настройки — как были."""
from _z import *

g36, g37 = uid("Guest36"), uid("Guest37")
listed = lambda nick: any(u.get("nickname") == nick for u in
                          api("GET", f"/admin/users?filter=new&q={nick}", None, "admin1")[1].get("users", []))
new_today = lambda: (api("GET", "/admin/stats/shift", None, "boss")[1].get("new_guests"),
                     api("GET", "/admin/reports/guests?preset=today", None, "boss")[1].get("totals", {}).get("new"))

section("без визитов первое появление — регистрация (Д2): в фильтре у стойки, но не в отчётах")
set_settings(seg_new_days=7)
check("у Guest36 и Guest37 нет сессий", sql(f"SELECT count(*) FROM sessions WHERE user_id IN ('{g36}','{g37}')") == "0")
reg36 = sql(f"SELECT registered_at FROM users WHERE id='{g36}'")
at_end(lambda: sql(f"UPDATE users SET registered_at='{reg36}' WHERE id='{g36}'"))
b = new_today()
sql(f"UPDATE users SET registered_at = now() - interval '1 minute' WHERE id='{g36}'")
check("Guest36 зарегистрировался минуту назад → в фильтре «новые»", listed("Guest36"))
check("Guest37 зарегистрирован больше 7 суток назад → не в фильтре «новые»", not listed("Guest37"))
check("регистрация без визита не меняет «Новых» ни в сводке, ни в отчёте за сегодня", new_today() == b, f"{b} → {new_today()}")
sql(f"UPDATE users SET registered_at='{reg36}' WHERE id='{g36}'")

section("в отчёте и сводке — первая сессия не раньше seg_new_days до конца суток")
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')"))
D = sql(f"""SELECT g::date FROM generate_series(((now() AT TIME ZONE '{TZ}')::date - 25)::timestamp,
    ((now() AT TIME ZONE '{TZ}')::date - 3)::timestamp, interval '1 day') AS g
    WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE started_at >= ((g - interval '6 days' + interval '{rh} hours') AT TIME ZONE '{TZ}')
      AND started_at < ((g + interval '1 day {rh} hours') AT TIME ZONE '{TZ}')) ORDER BY g DESC LIMIT 1""")
check(f"пустые сутки {D} и 6 суток до них", D, D)
D5 = sql(f"SELECT ('{D}'::date - 5)::text")
ids = []
at_end(lambda: [sql(f"DELETE FROM sessions WHERE id='{i}'") for i in ids])
for who, day, hhmm in ((g36, D5, "12:00"), (g36, D, "12:00"), (g37, D, "15:00")):
    ids.append(sql(f"""INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total,
        minutes_used, billed_minutes, base_rate_pln, effective_rate_pln, ended_reason)
        SELECT '{who}', id, club_id, 'completed', {at_club(day, hhmm)}, {at_club(day, hhmm)} + interval '1 hour', 60,
        60, 60, 0, 0, 'z5' FROM computers WHERE name='PC-09' RETURNING id"""))
for days, want in ((7, 2), (3, 1)):
    set_settings(seg_new_days=days)
    rep = api("GET", f"/admin/reports/guests?from={D}&to={D}", None, "boss")[1]
    sh = api("GET", f"/admin/stats/shift?date={D}", None, "boss")[1]
    who = "Guest36 (первая сессия за 5 суток) и Guest37" if want == 2 else "только Guest37: первая сессия Guest36 раньше 3 суток"
    check(f"seg_new_days={days}: в отчёте за {D} новых {want} — {who}", rep.get("totals", {}).get("new") == want
          and rep.get("thresholds", {}).get("new_days") == days, J(rep.get("totals")))
    check(f"seg_new_days={days}: сводка смены {D} = отчёт — новых {want}", sh.get("new_guests") == want, sh.get("new_guests"))

section("фильтр «новые» у стойки — на «сейчас»")
set_settings(seg_new_days=365)
check("seg_new_days=365: Guest36 и Guest37 (первые сессии в этом году) — новые", listed("Guest36") and listed("Guest37"))
set_settings(seg_new_days=1)
check("seg_new_days=1: первые сессии давнее суток — не новые", not listed("Guest36") and not listed("Guest37"))
done()
