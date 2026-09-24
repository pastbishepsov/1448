#!/usr/bin/env python3
"""З1-и1 «Касса — окно смены» (№1 «а»): «должно быть в ящике» считается от начала смены (записи
табеля) или от прошлой сдачи этого админа, а не от начала клубных суток; сдача датируется днём своей
смены и видна в её сводке; при «сошлось» владелец тоже получает строку с суммой (З1-и2: служебной строкой
«только владельцу», заметка «передал смену» — отдельным сообщением сдавшего).
Временный сотрудник z1adm, гость Guest31. Идемпотентен: свои пополнения отменяет, z1adm уволен."""
from _z import *

g31 = uid("Guest31")
zid = hire_temp("z1adm")
# прошлые сдачи z1adm (повторный прогон) позже «вчерашнего» начала смены сдвинули бы окно — убираем
sql(f"DELETE FROM shift_closeouts WHERE user_id='{zid}'")
yday = club_day(-1)
deps = []
def cash(pln, who="z1adm"):
    deps.append(topup("Guest31", pln, method="cash", who=who))
    return deps[-1]
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in reversed(deps)])
stop = lambda counted, note="": api("POST", "/admin/work/stop", {"counted_pln": counted, "note": note}, "z1adm")
preview = lambda: api("GET", "/admin/work/closeout", None, "z1adm")[1]

section("ночная смена: вход вчера 21:00, депозит 23:00, сдача сегодня")
tok("z1adm")  # вход открывает смену (Е3-и1)
sql(f"UPDATE work_entries SET started_at = {at_club(yday, '21:00')}, date = '{yday}' WHERE user_id='{zid}' AND ended_at IS NULL")
d1 = cash(100)
sql(f"UPDATE deposits SET created_at = {at_club(yday, '23:00')} WHERE id='{d1}'")
p = preview()
check("«должно быть» за смену = 100 zł (депозит до начала клубных суток — в смене)", near(p.get("expected_pln", -1), 100), J(p, 200))
s, d = stop(100, "передал смену, всё на месте")
co = d.get("closeout", {})
check("сдал 100 → сошлось, расхождение 0", s == 200 and near(co.get("diff_pln", -1), 0), f"{s} {J(co, 200)}")
check(f"сдача датирована днём своей смены ({yday})", co.get("date") == yday, co.get("date"))
shift = api("GET", f"/admin/stats/shift?date={yday}", None, "boss")[1]
check("сдача видна в сводке вчерашней смены", any(c.get("id") == co.get("id") for c in shift.get("closeouts", [])), J(shift.get("closeouts"), 200))
msg = sql("SELECT text FROM staff_messages WHERE kind='system' AND visible_to='owner' AND text LIKE '🧮 z1adm %' ORDER BY created_at DESC LIMIT 1")
note = sql(f"SELECT text FROM staff_messages WHERE author_id='{zid}' ORDER BY created_at DESC LIMIT 1")
check("при «сошлось» владелец получил строку с суммой, заметка — отдельно", "сошлась" in msg and "100.00" in msg and "всё на месте" in note, f"{msg} | {note}")

section("две сдачи за сутки не пересекаются")
api("POST", "/admin/work/start", {}, "z1adm")
cash(50)
s, d = stop(50)
check("первая сдача: 50 из 50 — сошлось", s == 200 and near(d.get("closeout", {}).get("diff_pln", -1), 0), J(d.get("closeout"), 160))
api("POST", "/admin/work/start", {}, "z1adm")
cash(30)
cash(70, who="boss")  # чужой нал в эту смену: в «должно» админа не входит (ящик — З8), виден справочно
p = preview()
check("вторая смена: «должно быть» 30, а не 80", near(p.get("expected_pln", -1), 30), J(p, 200))
check("справочно «всего за смену по всем» — плюс 70 владельца", near(p.get("club_expected_pln", -1) - p.get("expected_pln", 0), 70), J(p, 200))
s, d = stop(30)
check("вторая сдача: 30 из 30 — сошлось, ложной недостачи нет", s == 200 and near(d.get("closeout", {}).get("diff_pln", -1), 0), J(d.get("closeout"), 160))
done()
