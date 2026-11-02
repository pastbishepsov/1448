#!/usr/bin/env python3
"""З2-и1 «Часовой пояс соединения с базой» (№4, сценарий B ревью): отчёты считают дни и часы в поясе клуба,
даже когда DATABASE_URL пояса не задаёт — стенд так и запущен, Postgres живёт в Etc/UTC. Утреннее
пополнение (report_hour:30 по клубу — день по UTC в это время ещё вчерашний) ложится в свой день, сумма
столбиков равна итогу; сессия 20:00–22:00 — в часах 20 и 21 по часу (с З17-и2 сессии режутся по часам), пик — 20.
Всё — в пустом дне прошлого, чтобы чужие строки стенда не мешали. Гость Guest33. Идемпотентен: пополнение
отменяется, сессия удаляется."""
from _z import *

rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
# последний день в прошлом (40–400 дней назад), где нет ни живых пополнений, ни сессий
day = sql(f"""SELECT g::date FROM generate_series(((now() AT TIME ZONE '{TZ}')::date - 400)::timestamp,
    ((now() AT TIME ZONE '{TZ}')::date - 40)::timestamp, interval '1 day') AS g WHERE NOT EXISTS (SELECT 1 FROM deposits WHERE voided_at IS NULL
      AND created_at >= (g AT TIME ZONE '{TZ}') AND created_at < ((g + interval '2 days') AT TIME ZONE '{TZ}'))
    AND NOT EXISTS (SELECT 1 FROM sessions WHERE started_at >= (g AT TIME ZONE '{TZ}')
      AND started_at < ((g + interval '2 days') AT TIME ZONE '{TZ}')) ORDER BY g DESC LIMIT 1""")
check(f"пустой день для проверки найден: {day}", day, day)

section(f"утреннее пополнение {day} {rh:02d}:30 по клубу")
dep = topup("Guest33", 100, method="cash", who="boss")
at_end(lambda: api("POST", f"/admin/deposits/{dep}/void", {}, "boss"))
sql(f"UPDATE deposits SET created_at = {at_club(day, f'{rh:02d}:30')} WHERE id='{dep}'")
s, m = api("GET", f"/admin/reports/money?from={day}&to={day}", None, "boss")
days = m.get("days", [])
total = m.get("totals", {}).get("deposits_pln", -1)
check("итог периода — 100 zł", s == 200 and near(total, 100), f"{s} {total}")
check(f"пополнение в графике своего дня {day}", len(days) == 1 and days[0].get("date") == day and near(days[0].get("deposits_pln", 0), 100), J(days))
check("сумма столбиков = итог", near(sum(x.get("deposits_pln", 0) for x in days), total), f"{sum(x.get('deposits_pln', 0) for x in days)} vs {total}")

section(f"сессия {day} 20:00–22:00 по клубу")
pc = pcs()["PC-07"]
sid = sql(f"""INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ready_at, ended_at,
    minutes_total, minutes_used, base_rate_pln, effective_rate_pln)
    SELECT '{uid("Guest33")}', id, club_id, 'completed', {at_club(day, '20:00')}, {at_club(day, '20:00')},
    {at_club(day, '22:00')}, 120, 120, 0, 0 FROM computers WHERE id='{pc["id"]}' RETURNING id""")
at_end(lambda: sql(f"DELETE FROM sessions WHERE id='{sid}'"))
s, ld = api("GET", f"/admin/reports/load?from={day}&to={day}", None, "boss")
hours = {h.get("hour"): h for h in ld.get("hours", [])}
busy = {h: v for h, v in hours.items() if v.get("sessions")}
check("загрузка по часам: сессия 20:00–22:00 — в часах 20 и 21 по 1 ч (З17-и2: нарезка по часам)", s == 200
      and sorted(busy) == [20, 21] and all(v.get("sessions") == 1 and near(v.get("hours", 0), 1) for v in busy.values()), J(busy))
check("пик — 20:00 (первый из равных)", ld.get("peak_hour") == 20, ld.get("peak_hour"))
check(f"по дням — в своём дне {day}", [(x.get("date"), x.get("count")) for x in ld.get("days", [])] == [(day, 1)], J(ld.get("days")))
done()
