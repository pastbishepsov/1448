#!/usr/bin/env python3
"""З1-и1 «Касса — окно смены» (№1 «а»): сдача датируется днём своей смены и видна в её сводке; при «сошлось»
владелец тоже получает строку с суммой (З1-и2: служебной строкой «только владельцу», заметка «передал смену» —
отдельным сообщением сдавшего). З8-и1: «должно быть в ящике» — ящик клуба (на начало — оставленное прошлой
сдачей, плюс всё наличное с неё), поэтому ни ночная смена, ни вторая сдача за сутки не дают ложных излишков
и недостач; сценарии ящика целиком — e2e_z8_drawer.
Временный сотрудник z1adm, гость Guest31. Идемпотентен: свои пополнения отменяет, z1adm уволен."""
from _z import *

g31 = uid("Guest31")
zid = hire_temp("z1adm")
yday = club_day(-1)
deps = []
def cash(pln, who="z1adm"):
    deps.append(topup("Guest31", pln, method="cash", who=who))
    return deps[-1]
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in reversed(deps)])
stop = lambda counted, note="": api("POST", "/admin/work/stop", {"counted_pln": counted, "note": note}, "z1adm")
preview = lambda: api("GET", "/admin/work/closeout", None, "z1adm")[1]

section("ночная смена: вход вчера 21:00, депозит, сдача сегодня")
tok("z1adm")  # вход открывает смену (Е3-и1)
drawer_ready("z1adm")
sql(f"UPDATE work_entries SET started_at = {at_club(yday, '21:00')}, date = '{yday}' WHERE user_id='{zid}' AND ended_at IS NULL")
p0 = preview()
cash(100)
p = preview()
check("депозит 100 zł налом — «должно быть в ящике» +100", near(p.get("expected_pln", 0) - p0.get("expected_pln", 0), 100), J(p, 200))
s, d = stop(p.get("expected_pln"), "передал смену, всё на месте")
co = d.get("closeout", {})
check("пересчитал столько, сколько должно быть → сошлось, расхождение 0", s == 200 and near(co.get("diff_pln", -1), 0), f"{s} {J(co, 200)}")
check(f"сдача датирована днём своей смены ({yday})", co.get("date") == yday, co.get("date"))
shift = api("GET", f"/admin/stats/shift?date={yday}", None, "boss")[1]
check("сдача видна в сводке вчерашней смены", any(c.get("id") == co.get("id") for c in shift.get("closeouts", [])), J(shift.get("closeouts"), 200))
msg = sql("SELECT text FROM staff_messages WHERE kind='system' AND visible_to='owner' AND text LIKE '🧮 z1adm %' ORDER BY created_at DESC LIMIT 1")
note = sql(f"SELECT text FROM staff_messages WHERE author_id='{zid}' ORDER BY created_at DESC LIMIT 1")
check("при «сошлось» владелец получил строку с суммой, заметка — отдельно", "сошлась" in msg and f"{p.get('expected_pln', 0):.2f}" in msg
      and "всё на месте" in note, f"{msg} | {note}")

section("две сдачи за сутки: вторая начинает с пересчитанного первой")
api("POST", "/admin/work/start", {}, "z1adm")
cash(50)
p1 = preview()
s, d = stop(p1.get("expected_pln"))
check("первая сдача: сколько должно быть — столько пересчитал, сошлось", s == 200 and near(d.get("closeout", {}).get("diff_pln", -1), 0), J(d.get("closeout"), 160))
api("POST", "/admin/work/start", {}, "z1adm")
cash(30)
p2 = preview()
check("вторая: на начало — пересчитанное первой, должно быть = на начало + 30 (деньги первой не считаются дважды)",
      near(p2.get("opening_pln", -1), p1.get("expected_pln", -2)) and near(p2.get("expected_pln", -1), p1.get("expected_pln", -2) + 30), J(p2, 200))
s, d = stop(p2.get("expected_pln"))
check("вторая сдача сошлась — ложной недостачи нет", s == 200 and near(d.get("closeout", {}).get("diff_pln", -1), 0), J(d.get("closeout"), 160))
done()
