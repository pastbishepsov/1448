#!/usr/bin/env python3
"""З18-и5 «Карточка клуба — сервер» (№34, З-Р23; из №40 — публичные /clubs белым списком). Владелец правит карточку
клуба — PATCH /admin/clubs/:id: имя, адрес, телефон, Telegram, Instagram, часы работы и клубный тариф; раньше — только
SQL. Поле не прислано — не трогается; телефон и соцсети пустой строкой — стираются; Telegram и Instagram — именем без
«@» и ссылки; часы null — круглосуточно; правка — строкой club_update «было → стало», сохранение без правок — без
записи и без строки. Кривое — 400 с кодом поля, и ничего из запроса не сохраняется. Админу — 403. Наружу (без входа):
GET /clubs, /clubs/:id — без rtp_modifier, схемы зала и is_active, часы — объектом; /clubs/:id/computers — имя, зона,
статус и место на схеме, без MAC и придержания. «ПК без зоны запрещён» (pc_zone_required): пока есть ПК без зоны —
409 с их именами; включён — новый ПК без зоны и снятие зоны — 400 zone_required, смена зоны — можно. Временные ПК
z18c-1…3 удаляются в начале и в конце, карточка клуба и настройка — как были."""
import json

from _z import *

CLUB = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")
ORIG = sql(f"SELECT row_to_json(c)::text FROM clubs c WHERE id = '{CLUB}'")
COLS = "name, address, phone, telegram, instagram, working_hours, base_rate_pln"
q = lambda s: "$z18c$" + s + "$z18c$"


def restore():
    sql(f"UPDATE clubs SET ({COLS}) = (SELECT {COLS} FROM json_populate_record(NULL::clubs, {q(ORIG)})) WHERE id = '{CLUB}'")


PCS = "DELETE FROM computers WHERE name LIKE 'z18c-%'"
sql(PCS)  # хвост прошлого прогона
at_end(lambda: sql(PCS))
at_end(restore)
set_settings(pc_zone_required=0)  # и вернётся как была
last_club = lambda: sql("SELECT coalesce(max(created_at)::text, '') FROM admin_actions WHERE action = 'club_update'")
club_line = lambda: sql("SELECT details FROM admin_actions WHERE action = 'club_update' ORDER BY created_at DESC LIMIT 1")
STD = sql("SELECT id FROM zones WHERE name = 'Standard'")
VIP = sql("SELECT id FROM zones WHERE name = 'VIP'")

section("наружу — белым списком")
s, pc = api("POST", "/admin/computers", {"name": "z18c-1"}, "boss")
P1 = pc.get("computer_id", "")
api("PATCH", f"/admin/computers/{P1}", {"mac": "AA:BB:CC:DD:EE:18"}, "boss")
sql(f"UPDATE computers SET hold_until = now() + interval '5 minutes', hold_by = (SELECT id FROM users WHERE nickname = 'admin1') WHERE id = '{P1}'")
s1, lst = api("GET", "/clubs")
s2, one = api("GET", f"/clubs/{CLUB}")
s3, cps = api("GET", f"/clubs/{CLUB}/computers")
CARD = {"id", "name", "address", "latitude", "longitude", "phone", "telegram", "instagram", "working_hours", "base_rate_pln"}
EXTRA = {"computers_total", "computers_free", "zones"}
row = next((c for c in lst.get("clubs", []) if c.get("id") == CLUB), {})
check("GET /clubs и /clubs/:id — карточка: имя, адрес, координаты, телефон, соцсети, часы работы, клубный тариф, ПК и зоны; "
      "без rtp_modifier, схемы зала и is_active", s1 == 200 and s2 == 200 and set(row) == CARD | EXTRA and set(one) == CARD | EXTRA,
      J([sorted(row), sorted(one)], 600))
mine = next((p for p in cps.get("computers", []) if p.get("name") == "z18c-1"), {})
check("GET /clubs/:id/computers — имя, зона, статус и место на схеме; MAC и придержания нет (у z18c-1 — оба есть)",
      s3 == 200 and cps.get("computers") and all(set(p) == {"id", "name", "zone", "status", "pos_x", "pos_y"} for p in cps["computers"])
      and mine.get("status") == "available", J(mine, 400))
sql(f"UPDATE computers SET hold_until = NULL, hold_by = NULL WHERE id = '{P1}'")

section("PATCH /admin/clubs/:id — карточка клуба")
s, d = api("PATCH", f"/admin/clubs/{CLUB}", {"name": "z18c Клуб"}, "admin1")
check("админу — 403", s == 403, J([s, d]))
s, d = api("PATCH", "/admin/clubs/00000000-0000-0000-0000-0000000000ff", {"name": "z18c"}, "boss")
check("чужой клуб — 404 club_not_found", s == 404 and d.get("code") == "club_not_found", J([s, d]))
was = json.loads(ORIG)
HOURS = {d_: {"open": "10:00", "close": "23:00"} for d_ in ("mon", "tue", "wed", "thu", "fri")} | {"sat": {"open": "12:00", "close": "02:00"}}
t0 = last_club()
s, d = api("PATCH", f"/admin/clubs/{CLUB}", {"name": "  z18c Клуб  ", "address": " Warszawa, ul. Z18c 18 ", "phone": " +48 22 123-45-67 ",
                                              "telegram": "@z18c_club", "instagram": "https://www.instagram.com/z18c.club/",
                                              "working_hours": HOURS, "base_rate_pln": 21.5}, "boss")
cl = d.get("club") or {}
s2, one = api("GET", f"/clubs/{CLUB}")
check("владелец: имя, адрес, телефон, Telegram, Instagram, часы, тариф — 200; без пробелов по краям, Telegram и Instagram — "
      "именем; часы — объектом по дням; в GET /clubs/:id — то же",
      s == 200 and cl.get("name") == "z18c Клуб" and cl.get("address") == "Warszawa, ul. Z18c 18" and cl.get("phone") == "+48 22 123-45-67"
      and cl.get("telegram") == "z18c_club" and cl.get("instagram") == "z18c.club" and cl.get("working_hours") == HOURS
      and cl.get("base_rate_pln") == 21.5 and all(one.get(k) == cl.get(k) for k in CARD), J([s, cl, one], 900))
line = club_line() if last_club() != t0 else ""
check("журнал: club_update — «было → стало» по каждому полю: имя, адрес, телефон, Telegram, Instagram, часы словами, тариф",
      f"имя «{was['name']}» → «z18c Клуб»" in line and "адрес «" in line and "телефон " in line and "→ +48 22 123-45-67" in line
      and "Telegram " in line and "→ z18c_club" in line and "→ z18c.club" in line
      and " → пн–пт 10:00–23:00, сб 12:00–02:00, выходной: вс" in line and f"клубный тариф {float(was['base_rate_pln']):.2f} → 21.50 zł/ч" in line,
      line)
t1 = last_club()
s, d = api("PATCH", f"/admin/clubs/{CLUB}", {"name": "z18c Клуб", "telegram": "https://t.me/z18c_club", "working_hours": HOURS,
                                              "base_rate_pln": 21.50}, "boss")
check("то же ещё раз (Telegram ссылкой) — не правка: 200 unchanged, строки в журнале нет", s == 200 and d.get("unchanged") is True and last_club() == t1,
      J([s, d.get("unchanged"), last_club() == t1]))
s, d = api("PATCH", f"/admin/clubs/{CLUB}", {"phone": "", "instagram": " ", "working_hours": None}, "boss")
cl = d.get("club") or {}
line = club_line()
check("телефон и Instagram пустыми — стёрты, часы null — круглосуточно; в журнале — «→ —» и «→ круглосуточно»",
      s == 200 and cl.get("phone") is None and cl.get("instagram") is None and cl.get("working_hours") is None and cl.get("telegram") == "z18c_club"
      and "телефон +48 22 123-45-67 → —" in line and "Instagram z18c.club → —" in line and "часы пн–пт 10:00–23:00, сб 12:00–02:00, выходной: вс → круглосуточно" in line,
      J([s, cl, line], 600))

section("кривое — 400 с кодом поля, ничего не сохраняется")
bad = [({"name": "   "}, "bad_name"), ({"address": ""}, "bad_address"), ({"phone": "abc"}, "bad_phone"), ({"phone": "12345"}, "bad_phone"),
       ({"telegram": "ab"}, "bad_telegram"), ({"telegram": "https://t.me/"}, "bad_telegram"), ({"instagram": "z18c club"}, "bad_instagram"),
       ({"working_hours": {"xyz": {"open": "10:00", "close": "23:00"}}}, "bad_hours"), ({"working_hours": {"mon": {"open": "25:00", "close": "23:00"}}}, "bad_hours"),
       ({"working_hours": {}}, "bad_hours"), ({"working_hours": "круглосуточно"}, "bad_hours"), ({"base_rate_pln": 0.004}, "bad_rate"),
       ({"base_rate_pln": 0}, "bad_rate"), ({"base_rate_pln": 501}, "bad_rate"), ({"name": "z18c Новое", "phone": "abc"}, "bad_phone")]
got = []
for body, code in bad:
    s, d = api("PATCH", f"/admin/clubs/{CLUB}", body, "boss")
    got.append([code, s, d.get("code"), bool(d.get("message"))])
s, one = api("GET", f"/clubs/{CLUB}")
check(f"{len(bad)} кривых запросов — 400 с кодом поля и фразой; последний (имя хорошее, телефон нет) — имя не сменилось",
      all(g[1] == 400 and g[2] == g[0] and g[3] for g in got) and one.get("name") == "z18c Клуб" and one.get("base_rate_pln") == 21.5,
      J([g for g in got if not (g[1] == 400 and g[2] == g[0] and g[3])] + [one.get("name")], 600))

section("«ПК без зоны запрещён»")
s, d = api("PUT", "/admin/settings", {"pc_zone_required": 1}, "boss")
check("включить, пока z18c-1 без зоны, — 409 pcs_without_zone с его именем; настройка — 0",
      s == 409 and d.get("code") == "pcs_without_zone" and "z18c-1" in (d.get("names") or []) and "z18c-1" in d.get("message", "")
      and settings().get("pc_zone_required") == 0, J([s, d]))
s0 = api("PATCH", f"/admin/computers/{P1}", {"zone_id": STD}, "boss")[0]
s, d = api("PUT", "/admin/settings", {"pc_zone_required": 1}, "boss")
check("z18c-1 в зоне Standard — включается: 200, pc_zone_required 1", s0 == 200 and s == 200 and d.get("settings", {}).get("pc_zone_required") == 1, J([s0, s]))
r = [api("POST", "/admin/computers", {"name": "z18c-2"}, "boss"), api("POST", "/admin/computers", {"name": "z18c-2", "zone_id": ""}, "boss"),
     api("PATCH", f"/admin/computers/{P1}", {"zone_id": ""}, "boss")]
zone_now = sql(f"SELECT coalesce(zone, '') FROM computers WHERE id = '{P1}'")
check("включён: новый ПК без зоны (и с пустой зоной) — 400 zone_required; снять зону у z18c-1 — 400, зона осталась",
      all(x[0] == 400 and x[1].get("code") == "zone_required" and x[1].get("message") for x in r) and zone_now == "Standard"
      and not rows("SELECT 1 FROM computers WHERE name = 'z18c-2'"), J([[x[0], x[1].get("code")] for x in r] + [zone_now]))
r = [api("POST", "/admin/computers", {"name": "z18c-2", "zone_id": VIP}, "boss"), api("PATCH", f"/admin/computers/{P1}", {"zone_id": VIP}, "boss")]
check("включён: с зоной — можно: новый z18c-2 в VIP — 201, z18c-1 → VIP — 200",
      r[0][0] == 201 and r[1][0] == 200 and sql(f"SELECT zone FROM computers WHERE id = '{P1}'") == "VIP", J([r[0][0], r[1][0]]))
s, d = api("PUT", "/admin/settings", {"pc_zone_required": 0}, "boss")
s2, d2 = api("POST", "/admin/computers", {"name": "z18c-3"}, "boss")
check("выключен — ПК без зоны снова можно: z18c-3 — 201", s == 200 and s2 == 201, J([s, s2, d2]))
done()
