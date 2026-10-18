#!/usr/bin/env python3
"""З15-и3 «Права админа» (№29, З-Р20). Админ у стойки получал аккаунты персонала целиком: GET /admin/users?role=owner —
аккаунт boss со всеми полями, ?role=admin и ?role=all — коллеги; GET /admin/users/<id сотрудника> — его карточка с пакетами
и долгом; а в журнале видел правки табеля владельцем (work_edit: кому, какие часы были и стали). Теперь админу — только
гости: role=admin, owner и all — 403 owner_only, карточка сотрудника и её пакеты и долг — 403 owner_only, строк work_edit в
его журнале нет; приход и уход (work_start, work_stop) он видит, как раньше. Владелец видит всё. Временный z15h — сотрудник,
чей табель правит владелец (запись проверки удаляется, z15h уволен), гость Guest46 — только читается; денег не трогает."""
from _z import *

Z = "z15h"
zid = hire_temp(Z)
G = uid("Guest46")
BOSS, A2 = uid("boss"), uid("admin2")
NOTE = "z15h проверка прав"
at_end(lambda: sql(f"DELETE FROM work_entries WHERE user_id = '{zid}' AND note LIKE '{NOTE}%'"))
for e in rows(f"SELECT id FROM work_entries WHERE user_id = '{zid}' AND note LIKE '{NOTE}%'"):  # хвост упавшего прогона
    api("DELETE", f"/admin/work/{e}", None, "boss")

section("список пользователей: админу — только гости")
for role in ("admin", "owner", "all"):
    s, d = api("GET", f"/admin/users?role={role}", None, "admin1")
    check(f"admin1: ?role={role} — 403 owner_only, аккаунтов персонала в ответе нет",
          s == 403 and d.get("code") == "owner_only" and "users" not in d, f"{s} {J(d, 200)}")
s, d = api("GET", "/admin/users?role=player&q=Guest46", None, "admin1")
s2, d2 = api("GET", "/admin/users?q=boss", None, "admin1")
check("admin1: гости — как раньше (Guest46 находится), а поиск «boss» персонал не находит",
      s == 200 and any(u.get("id") == G for u in d.get("users", [])) and s2 == 200
      and not any(u.get("role") in ("admin", "owner") for u in d2.get("users", [])), f"{s} {s2} {J(d2.get('users', [])[:2], 200)}")
s, d = api("GET", "/admin/users?role=hacker", None, "admin1")
check("кривой role — по-прежнему 400 bad_role", s == 400 and d.get("code") == "bad_role", f"{s} {J(d)}")
seen = {}
for role in ("admin", "owner", "all"):
    s, d = api("GET", f"/admin/users?role={role}", None, "boss")
    seen[role] = s == 200 and any(u.get("role") == role or (role == "all" and u.get("role") != "player") for u in d.get("users", []))
check("владелец: ?role=admin, owner и all — как раньше, персонал в списке", all(seen.values()), J(seen))

section("карточка: сотрудника — только владельцу")
res = {}
for nick, i in (("boss", BOSS), ("admin2", A2), (Z, zid), ("admin1 (своя)", uid("admin1"))):
    res[nick] = [api("GET", f"/admin/users/{i}{tail}", None, "admin1") for tail in ("", "/packages", "/debt")]
check("admin1: карточка boss, admin2, z15h и своя, их пакеты и долг — 403 owner_only, данных аккаунта в ответе нет",
      all(s == 403 and d.get("code") == "owner_only" and "user" not in d and "packages" not in d
          for v in res.values() for s, d in v), J({k: [s for s, _ in v] for k, v in res.items()}))
s, d = api("GET", f"/admin/users/{G}", None, "admin1")
s2, d2 = api("GET", f"/admin/users/{G}/packages", None, "admin1")
s3, d3 = api("GET", f"/admin/users/{G}/debt", None, "admin1")
check("admin1: карточка гостя Guest46, его пакеты и долг — как раньше, 200",
      s == 200 and (d.get("user") or {}).get("id") == G and s2 == 200 and "packages" in d2 and s3 == 200, f"{s} {s2} {s3}")
s, d = api("GET", f"/admin/users/{A2}", None, "boss")
s2, d2 = api("GET", f"/admin/users/{A2}/packages", None, "boss")
check("владелец: карточка admin2 и его пакеты — как раньше, 200", s == 200 and (d.get("user") or {}).get("id") == A2 and s2 == 200,
      f"{s} {s2}")

section("журнал: правки табеля — только владельцу")
day = club_day(-3)
seen_o = {x.get("id") for x in audit(limit=60)}
s, c = api("POST", "/admin/work", {"nickname": Z, "date": day, "start": "10:00", "end": "12:00", "note": NOTE}, "boss")
eid = ((c or {}).get("entry") or {}).get("id") or sql(f"SELECT id FROM work_entries WHERE user_id = '{zid}' AND note = '{NOTE}'")
s2, _ = api("PATCH", f"/admin/work/{eid}", {"start": "10:00", "end": "13:00", "note": NOTE + ": правка"}, "boss")
s3, _ = api("DELETE", f"/admin/work/{eid}", None, "boss")
own = [x for x in audit(limit=60) if x.get("id") not in seen_o and x.get("kind") == "work_edit" and x.get("nickname") == Z]
check("владелец: добавил, поправил и удалил запись табеля z15h — в его журнале три строки work_edit «табель: …»",
      (s, s2, s3) == (201, 200, 200) and len(own) == 3 and all((x.get("text") or "").startswith("табель: ") for x in own),
      f"{(s, s2, s3)} {J(own, 400)}")
adm = audit("admin1", limit=200)
adm_kind = audit("admin1", kind="work_edit", limit=200)
check("admin1: ни этих строк, ни вообще work_edit — ни в журнале, ни фильтром kind=work_edit",
      not [x for x in adm if x.get("kind") == "work_edit"] and not adm_kind and not ({x.get("id") for x in own} & {x.get("id") for x in adm}),
      f"{len(adm)} {J(adm_kind, 200)}")
tok(Z)  # вход z15h в админку — «пришёл»: приход и уход админ видит, как раньше
ws = [x for x in audit("admin1", limit=60) if x.get("kind") == "work_start" and x.get("admin") == Z]
check("admin1: приход z15h («табель: приход», work_start) в журнале есть — как раньше", bool(ws), J(ws[:1], 200))
done()
