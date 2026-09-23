#!/usr/bin/env python3
"""З0-и4 «Потолок кейсов для админа» (№9 «а»): настройка admin_day_case_cap — сколько кейсов админ выдаёт
за клубные сутки (0 — без лимита); сверх потолка — 403 day_cap, владелец не ограничен, как у XP и депозитов.
Временный сотрудник z0case, гость Guest32. Идемпотентен: настройка вернётся, невскрытые кейсы проверки
у Guest32 убираются, z0case уволен."""
from _z import *

g32 = uid("Guest32")
zid = hire_temp("z0case")
start = sql("SELECT now()")
at_end(lambda: sql(f"DELETE FROM cases WHERE user_id='{g32}' AND source='admin_grant' AND opened_at IS NULL AND created_at >= '{start}'"))
case = lambda who: api("POST", f"/admin/users/{g32}/grant", {"type": "case", "case_tier": "light", "reason": "e2e z0 кейсы"}, who)
# сколько z0case уже выдал за эти клубные сутки (повторный прогон в тот же день)
used = int(sql(f"SELECT count(*) FROM admin_grants WHERE admin_id='{zid}' AND grant_type='case' AND created_at >= "
               f"{at_club(club_day(), '%02d:00' % settings().get('report_hour', 8))}"))

section("потолок у админа")
s, d = set_settings(admin_day_case_cap=used + 2)
check(f"настройка admin_day_case_cap={used + 2} сохраняется и видна", s == 200 and settings().get("admin_day_case_cap") == used + 2, f"{s} {d.get('code', '')}")
c1, c2 = case("z0case"), case("z0case")
check("два кейса в пределах потолка → 201, 201", c1[0] == 201 and c2[0] == 201, f"{c1[0]} {c2[0]}")
s, d = case("z0case")
check("третий → 403 day_cap с понятным текстом", s == 403 and d.get("code") == "day_cap" and "кейс" in d.get("message", ""), f"{s} {d.get('code')} {d.get('message')}")
check("… и кейса не прибавилось", int(sql(f"SELECT count(*) FROM cases WHERE user_id='{g32}' AND created_at >= '{start}'")) == 2)

section("владелец и «0 — без лимита»")
check("владелец при том же потолке → 201", case("boss")[0] == 201)
set_settings(admin_day_case_cap=0)
check("потолок 0 — админ снова выдаёт → 201", case("z0case")[0] == 201)
s, d = api("PUT", "/admin/settings", {"admin_day_case_cap": 1001}, "boss")
check("больше 1000 не сохраняется → 400 bad_value", s == 400 and d.get("code") == "bad_value", f"{s} {d.get('code')}")
done()
