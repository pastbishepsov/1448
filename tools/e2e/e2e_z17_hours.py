#!/usr/bin/env python3
"""З17-и2 «Нарезка по часам» (№13, З-Р6 «б»), «Отчёты → Загрузка». «Загрузка по часам суток» клала всю длину сессии в
час её начала: пять часов с 18:00 — столбик «18:00» на 5 ч, 19–22 пусто, и пик всегда у часов прихода. Теперь сессии
режутся по часам стенных часов клуба (паузы — пропорционально): у часа — наиграно, сколько сессий шло, занятость; пик —
по нарезке; heat — «день недели клубных суток × час». Пустые клубные сутки в прошлом: сессии Guest47 (SQL, без денег,
ended_reason z17h) — 18:00–23:00 на PC-07, 20:00–21:00 на PC-08, 10:00–12:00 на PC-09 с часом паузы (60 минут игры),
ночь 23:00–02:00 на VIP-01. В конце сессии удаляются (и при падении тоже)."""
import datetime
import math

from _z import *

G = "Guest47"
drop = lambda: sql(f"DELETE FROM sessions WHERE ended_reason = 'z17h' AND user_id = '{uid(G)}'")
drop()
at_end(drop)
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '8')"))
D = sql(f"""SELECT g::date::text FROM generate_series(((now() AT TIME ZONE '{TZ}')::date - 400)::timestamp,
    ((now() AT TIME ZONE '{TZ}')::date - 3)::timestamp, interval '1 day') AS g
    WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE started_at >= ((g + interval '{rh} hours') AT TIME ZONE '{TZ}')
      AND started_at < ((g + interval '1 day {rh} hours') AT TIME ZONE '{TZ}')) ORDER BY g DESC LIMIT 1""")
D1 = sql(f"SELECT ('{D}'::date + 1)::text")
NALL = int(sql("SELECT count(*) FROM computers"))


def ins(pc, frm, to, mins, next_day=False):
    """Завершённая сессия Guest47 в сутки D без денег; next_day — конец уже в календарный день D+1 (ночь)."""
    return sql(f"""INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ready_at, ended_at, minutes_total,
        minutes_used, billed_minutes, base_rate_pln, effective_rate_pln, ended_reason, zone_id, zone_name, xp_per_min,
        coins_per_min) SELECT '{uid(G)}', c.id, c.club_id, 'completed', {at_club(D, frm)}, {at_club(D, frm)},
        {at_club(D1 if next_day else D, to)}, {mins}, {mins}, {mins}, 0, 0, 'z17h', c.zone_id, COALESCE(c.zone, ''), 10, 2
        FROM computers c WHERE c.name = '{pc}' RETURNING id""")


pct = lambda minutes, capacity: math.floor(minutes / capacity * 1000 + 0.5) / 10  # как math.Round сервера
ins("PC-07", "18:00", "23:00", 300)
ins("PC-08", "20:00", "21:00", 60)
ins("PC-09", "10:00", "12:00", 60)  # час паузы: отрезок — 2 часа, игры — 60 минут
ins("VIP-01", "23:00", "02:00", 180, next_day=True)
s, d = api("GET", f"/admin/reports/load?from={D}&to={D}", None, "boss")
hrs = {h.get("hour"): h for h in d.get("hours") or []}
row = lambda h: (hrs.get(h, {}).get("hours"), hrs.get(h, {}).get("sessions"))
OM = (d.get("capacity") or {}).get("open_minutes")
WANT = {18: (1, 1), 19: (1, 1), 20: (2, 2), 21: (1, 1), 22: (1, 1), 23: (1, 1), 0: (1, 1), 1: (1, 1), 10: (0.5, 1), 11: (0.5, 1)}

section(f"сутки {D}")
check("18:00–23:00 на PC-07 — по часу в 18, 19, 21, 22, в 20 — ещё час PC-08: 2 ч и 2 сессии (раньше — 5 ч в «18:00»)",
      bool(D) and s == 200 and [row(h) for h in (18, 19, 20, 21, 22)] == [WANT[h] for h in (18, 19, 20, 21, 22)],
      f"{D} {s} · " + J({h: row(h) for h in range(24) if row(h) != (0, 0)}))
check("ночь 23:00–02:00 на VIP-01 — по часу в 23, 0 и 1; 10:00–12:00 с часом паузы — по 0,5 ч в 10 и 11",
      [row(h) for h in (23, 0, 1, 10, 11)] == [WANT[h] for h in (23, 0, 1, 10, 11)], J({h: row(h) for h in (23, 0, 1, 10, 11)}))
check("остальные часы пусты; сумма по часам = «Наиграно» (10 ч); пик — 20:00",
      all(row(h) == (0, 0) for h in range(24) if h not in WANT)
      and math.isclose(sum(hrs.get(h, {}).get("hours", 0) for h in range(24)), 10) and d.get("totals", {}).get("hours") == 10
      and d.get("peak_hour") == 20, f"пик {d.get('peak_hour')} · итог {J(d.get('totals'))}")
check(f"занятость часа — от ПК и часов работы ({OM} мин в сутки): 20:00 — 2 ч на {NALL} ПК = {pct(120, NALL * 60)} %, "
      f"18:00 — {pct(60, NALL * 60)} %, 15:00 — 0".replace(".", ","),
      OM == 1440 and hrs.get(20, {}).get("occupancy_pct") == pct(120, NALL * 60)
      and hrs.get(18, {}).get("occupancy_pct") == pct(60, NALL * 60) and hrs.get(15, {}).get("occupancy_pct") == 0,
      J({h: hrs.get(h, {}).get("occupancy_pct") for h in (20, 18, 15)}))
heat = d.get("heat") or []
dw = datetime.date.fromisoformat(D).weekday() if D else 0  # понедельник — 0, как у сервера
same = len(heat) == 7 and all(len(r) == 24 for r in heat) and all(
    (heat[dw][h].get("hours"), heat[dw][h].get("sessions")) == row(h) for h in range(24))
empty = len(heat) == 7 and all(c.get("hours") == 0 and c.get("sessions") == 0 for i, r in enumerate(heat) if i != dw for c in r)
check(f"heat — «день недели × час»: в строке этих суток (день {dw}, пн — 0) — та же раскладка, ночь — там же, а не в "
      f"следующем дне; остальные дни пусты", same and empty,
      f"{len(heat)} строк · день {dw}: {J([(c.get('hours'), c.get('sessions')) for c in heat[dw]] if len(heat) > dw else None)}")
done()
