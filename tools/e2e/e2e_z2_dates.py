#!/usr/bin/env python3
"""З2-и2 «Колонки-даты — день клуба» (№4): даты из колонок date — табель, график, сдачи касс, наём и
увольнение, день рождения — сервер читает полуночью клуба, а не UTC. Нанятого сегодня можно уволить
сегодня; забытая смена «День 08–20» закрывается в 20:00 (720 минут), а не в 22:00; правка записи
табеля без даты не сдвигает время на 2 часа. Сутки в отчётах — по стенным часам клуба и в дни перевода
часов (решение 24.09): 29.03.2026 08:30 — в своих сутках, 26.10.2025 07:30 — в предыдущих.
Временные z2hire и z2shift, гость Guest33. Идемпотентен: пополнения отменяются, записи табеля z2shift удаляются
все — в начале (хвосты прошлых прогонов) и в конце, уже после увольнения (З15-и1а), временные уволены."""
from _z import *

TODAY = sql(f"SELECT ((now() AT TIME ZONE '{TZ}')::date)::text")  # календарный день по часам клуба
local = lambda col: f"to_char({col} AT TIME ZONE '{TZ}', 'YYYY-MM-DD HH24:MI')"  # момент по часам клуба

section(f"наём и увольнение в один день ({TODAY})")
hid = hire_temp("z2hire")
at_end(lambda: sql(f"UPDATE staff_profiles SET hired_at = '{club_day(-30)}' WHERE user_id='{hid}'"))  # до уборки
sql(f"UPDATE staff_profiles SET hired_at = '{TODAY}' WHERE user_id='{hid}'")
s, d = dismiss("z2hire")
check("нанят сегодня, уволен без даты (сегодня) → 200", s == 200 and d.get("dismissed_at") == TODAY, f"{s} {J(d)}")

section("забытая смена «День 08–20»")
Z2_ENTRIES = "DELETE FROM work_entries WHERE user_id = (SELECT id FROM users WHERE nickname = 'z2shift')"
sql(Z2_ENTRIES)  # хвосты прошлых прогонов: запись «Я пришёл», закрытая увольнением, через два дня ложилась под правку 09:00–21:00 (409)
at_end(lambda: sql(Z2_ENTRIES))  # в конце — все его записи, уже после увольнения (уборка идёт в обратном порядке)
zid = hire_temp("z2shift")
D = sql(f"SELECT ((now() AT TIME ZONE '{TZ}')::date - 2)::text")  # позавчера: ни одна идущая смена его не займёт
tok("z2shift")  # вход открывает запись табеля (Е3-и1) — её и превращаем в забытую
e1 = sql(f"SELECT id FROM work_entries WHERE user_id='{zid}' AND ended_at IS NULL")
sql(f"UPDATE work_entries SET date = '{D}', started_at = {at_club(D, '08:00')}, "
    f"shift_id = (SELECT id FROM shifts WHERE name='День') WHERE id='{e1}'")
s, d = api("POST", "/admin/work/start", {}, "z2shift")  # прошлая запись не закрыта — сервер закроет её по графику
row = sql(f"SELECT {local('ended_at')} || '|' || minutes || '|' || auto_closed FROM work_entries WHERE id='{e1}'")
check(f"закрыта по графику: {D} 20:00, 720 минут, пометка auto_closed", s == 201 and d.get("auto_closed_previous")
      and row == f"{D} 20:00|720|true", f"{s} {row}")

section("правка записи табеля без даты")
s, d = api("PATCH", f"/admin/work/{e1}", {"start": "09:00", "end": "21:00", "note": "z2: правка"}, "boss")
row = sql(f"SELECT {local('started_at')} || '|' || {local('ended_at')} || '|' || minutes || '|' || date FROM work_entries WHERE id='{e1}'")
check(f"09:00–21:00 легли как есть: {D} 09:00 → 21:00, 720 минут, день тот же", s == 200
      and row == f"{D} 09:00|{D} 21:00|720|{D}", f"{s} {row}")

section("сутки в отчёте «Деньги» в дни перевода часов")
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
for day, hhmm, want in (("2026-03-29", f"{rh:02d}:30", "2026-03-29"), ("2025-10-26", f"{rh - 1:02d}:30", "2025-10-25")):
    dep = topup("Guest33", 50, method="cash", who="boss")
    at_end(lambda dep=dep: api("POST", f"/admin/deposits/{dep}/void", {}, "boss"))
    sql(f"UPDATE deposits SET created_at = {at_club(day, hhmm)} WHERE id='{dep}'")
    s, m = api("GET", f"/admin/reports/money?from={want}&to={want}", None, "boss")
    days, total = m.get("days", []), m.get("totals", {}).get("deposits_pln", -1)
    check(f"{day} {hhmm} по клубу — в сутках {want}: столбик = итог", s == 200 and total >= 50 and len(days) == 1
          and days[0].get("date") == want and near(days[0].get("deposits_pln", 0), total), f"{J(days)} · итог {total}")
done()
