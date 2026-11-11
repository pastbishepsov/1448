#!/usr/bin/env python3
"""З4-и3 «„Наиграно“ одним принципом» (№16, З-Р4): наигранное время везде считается по началу сессии, а идущая
сессия даёт минуты «на сейчас» — сколько уже учёл биллинг. Раньше сводка смены брала сессии, завершённые в эти
сутки, а отчёты — начатые, и идущая давала 0 и там, и там: днём «Наиграно» не видело тех, кто сидит за ПК, а
ночная сессия через report_hour попадала в сводке в один день, в отчётах — в другой. Guest35: идущая сессия
30 минут за PC-07 (платит кошельком) и ночная 23:00–09:00 в пустые сутки прошлого. Идемпотентен: сессии
удаляются, кошелёк докладывается до 20 zł."""
import time
from _z import *

fresh_day(30)  # З19-и9: ночью сессия «30 минут назад» — ещё в сегодняшних сутках
TODAY = club_day()

def played(day):
    """«Наиграно» за клубные сутки day, в минутах: сводка смены, «Загрузка», «Гости», «Персонал → по сменам»."""
    sh = api("GET", f"/admin/stats/shift?date={day}", None, "boss")[1].get("sessions", {})
    ld = api("GET", f"/admin/reports/load?from={day}&to={day}", None, "boss")[1].get("totals", {})
    gs = api("GET", f"/admin/reports/guests?from={day}&to={day}", None, "boss")[1].get("totals", {})
    st = api("GET", f"/admin/reports/staff?from={day}&to={day}", None, "boss")[1].get("by_shift", [])
    return {"сводка": sh.get("minutes", 0), "сводка: сессий": sh.get("started", 0),
            "загрузка": round(ld.get("hours", 0) * 60), "гости": round(gs.get("hours", 0) * 60),
            "по сменам": round(sum(x.get("hours", 0) for x in st) * 60)}

section("идущая сессия 30 минут — ~30 минут и в сводке, и в отчётах")
end_all("Guest35")
ensure_wallet("Guest35", 20)
b = played(TODAY)
sid = seat("Guest35", "PC-07", minutes_ago=30)
at_end(lambda: sql(f"DELETE FROM sessions WHERE id='{sid}'"))
at_end(lambda: end(sid))  # уборка в обратном порядке: сперва завершить, потом удалить
billed = 0
for _ in range(40):  # тик биллинга — раз в 30 с
    billed = int(sql(f"SELECT billed_minutes FROM sessions WHERE id='{sid}'") or 0)
    if billed >= 30:
        break
    time.sleep(1.5)
check("биллинг учёл 30 минут идущей сессии", billed >= 30, billed)
a = played(TODAY)
d = {k: a[k] - b[k] for k in a}
check("сводка смены: +30 минут и +1 сессия", 30 <= d["сводка"] <= 32 and d["сводка: сессий"] == 1, J(d))
check("«Загрузка» и «Гости»: +0,5 ч", abs(d["загрузка"] - 30) <= 6 and abs(d["гости"] - 30) <= 6, J(d))
check("«Персонал → по сменам»: +0,5 ч", abs(d["по сменам"] - 30) <= 9, J(d))
check("сводка = «Загрузка» (с точностью до округления часов)", abs(a["сводка"] - a["загрузка"]) <= 3, J(a))

section("ночная сессия через report_hour — в сутках начала и в сводке, и в отчётах")
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')"))
win = lambda col: (f"{col} >= ((g + interval '{rh} hours') AT TIME ZONE '{TZ}') "
                   f"AND {col} < ((g + interval '2 days {rh} hours') AT TIME ZONE '{TZ}')")
D = sql(f"""SELECT g::date FROM generate_series(((now() AT TIME ZONE '{TZ}')::date - 25)::timestamp,
    ((now() AT TIME ZONE '{TZ}')::date - 3)::timestamp, interval '1 day') AS g
    WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE {win('started_at')} OR {win('ended_at')}) ORDER BY g DESC LIMIT 1""")
check(f"двое пустых суток подряд: {D} и следующие", D, D)
D1 = sql(f"SELECT ('{D}'::date + 1)::text")
nid = sql(f"""INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total,
    minutes_used, billed_minutes, base_rate_pln, effective_rate_pln, ended_reason)
    SELECT '{uid("Guest35")}', id, club_id, 'completed', {at_club(D, '23:00')}, {at_club(D1, '09:00')}, 600,
    600, 600, 0, 0, 'z4' FROM computers WHERE name='PC-08' RETURNING id""")
at_end(lambda: sql(f"DELETE FROM sessions WHERE id='{nid}'"))
p0, p1 = played(D), played(D1)
check(f"{D} (сутки начала), сводка: 1 сессия, 600 минут", p0["сводка: сессий"] == 1 and p0["сводка"] == 600, J(p0))
check(f"{D}: «Загрузка», «Гости», «по сменам» — 10 ч", all(p0[k] == 600 for k in ("загрузка", "гости", "по сменам")), J(p0))
check(f"{D1} (сутки конца): 0 и в сводке, и в отчётах", not any(p1.values()), J(p1))
done()
