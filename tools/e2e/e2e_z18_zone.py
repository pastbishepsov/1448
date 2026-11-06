#!/usr/bin/env python3
"""З18-и1 «Удаление зоны с живыми минутами» (№8, З-Р7; из №40 — «только переименовать» и имя у живых остатков).
Сценарий L ревью 23.09: зона «z18 Tmp» по 10 zł/ч, 30 мин за монеты Guest48 → удаление → 200, минуты «без зоны» →
10 минут на VIP-01 отыграны ими бесплатно. Теперь: удаление при живых минутах — 409 has_minutes с числом минут и гостей
(у Guest48 30 мин, у Guest49 15), минуты на VIP не тратятся; переименование — новое имя у живых выдач; мастер —
POST /admin/zones/:id/move-minutes: перенос в Standard (тратятся там, гостям — уведомление, в журнал — строка на гостя),
сжечь (остаток 0, уведомление), потом удаление — 200; продажи пакетов держат зону — 409 package_history («только
переименовать»), непроданный пакет каталога — 409 has_packages. З18-и1а (миграция 092): сожжённое — у выдачи момент и
сколько (burned_at, burned_minutes), и «Сгорело за период» в «Деньгах» и «Кассе» — строкой «минуты зоны» по тарифу
выдачи, и после удаления зоны — с её именем. Временные зоны «z18 …» и всё их — удаляются в начале и в конце; выдачи и
погашения Guest48 и Guest49 — тоже (гости З18–З19), монеты им ставятся SQL-ом, кошелёк Guest48 докладывается до 30 zł."""
from _z import *

G1, G2 = "Guest48", "Guest49"
U1, U2 = uid(G1), uid(G2)
ZN = ("z18 Tmp", "z18 Tmp-2", "z18 Пак")


def tidy():
    end_all(G1, G2)
    sql(f"DELETE FROM minute_grants WHERE user_id IN ('{U1}', '{U2}')")
    sql(f"DELETE FROM coin_redemptions WHERE user_id IN ('{U1}', '{U2}')")
    names = ", ".join(f"'{n}'" for n in ZN)
    sql(f"DELETE FROM user_packages WHERE zone_id IN (SELECT id FROM zones WHERE name IN ({names}))")
    sql(f"DELETE FROM time_packages WHERE zone_id IN (SELECT id FROM zones WHERE name IN ({names}))")
    sql(f"DELETE FROM zones WHERE name IN ({names})")


tidy()
at_end(tidy)
sql(f"UPDATE users SET coins_balance = 1000, coin_minutes = 0 WHERE id IN ('{U1}', '{U2}')")  # без старого пула без зоны
ensure_wallet(G1, 30)
STD = sql("SELECT id FROM zones WHERE name = 'Standard'")
grants = lambda u: ",".join(rows(f"SELECT zone_name || ':' || minutes_left FROM minute_grants WHERE user_id = '{u}' ORDER BY created_at"))
zone = lambda zid: next((z for z in api("GET", "/admin/zones", None, "boss")[1].get("zones", []) if z["id"] == zid), None)
note = lambda u, t: sql(f"SELECT payload::text FROM notifications WHERE user_id = '{u}' AND type = '{t}' ORDER BY created_at DESC LIMIT 1")
last_audit = lambda action, u: sql(f"SELECT details FROM admin_actions WHERE action = '{action}' AND target_user_id = '{u}' "
                                   "ORDER BY created_at DESC LIMIT 1")

section("сценарий L: минуты зоны не отвязываются от неё")
s, d = api("POST", "/admin/zones", {"name": "z18 Tmp", "rate_pln": 10}, "boss")
ZT = (d.get("zone") or {}).get("id")
s1, r1 = api("POST", f"/admin/users/{U1}/redeem", {"zone_id": ZT, "minutes": 30}, "admin1")
s2, r2 = api("POST", f"/admin/users/{U2}/redeem", {"zone_id": ZT, "minutes": 15}, "admin1")
check("зона «z18 Tmp» по 10 zł/ч; Guest48 — 30 мин за 100 монет, Guest49 — 15 мин",
      s == 201 and s1 == 200 and r1.get("coins") == 100 and s2 == 200 and grants(U1) == "z18 Tmp:30" and grants(U2) == "z18 Tmp:15",
      f"{s} {s1} {r1.get('coins')} {s2} · {grants(U1)} · {grants(U2)}")
z = zone(ZT) or {}
check("в списке зон: у гостей 45 мин, у двух гостей; держат минуты (delete_block has_minutes)",
      z.get("live_minutes") == 45 and z.get("live_guests") == 2 and z.get("delete_block") == "has_minutes", J(z))
s, d = api("DELETE", f"/admin/zones/{ZT}", None, "boss")
check("удаление — 409 has_minutes: «У 2 гостей 45 мин этой зоны — перенеси их в другую зону или сожги»; зона и минуты на месте",
      s == 409 and d.get("code") == "has_minutes" and d.get("minutes") == 45 and d.get("guests") == 2
      and d.get("message", "").startswith("У 2 гостей 45 мин этой зоны — перенеси их в другую зону или сожги")
      and zone(ZT) is not None and grants(U1) == "z18 Tmp:30", f"{s} {J(d)}")
sid = seat(G1, "VIP-01", minutes_ago=10)
s, d = end(sid)
row = sql(f"SELECT billed_minutes || '|' || coin_minutes_used || '|' || charged_grosz FROM sessions WHERE id = '{sid}'")
b, used, charged = (int(x) for x in row.split("|"))
check(f"10 минут на VIP-01: минуты «z18 Tmp» не тронуты (было бы {b} мин бесплатно), платит кошелёк — {charged / 100:.2f} zł",
      s == 200 and b >= 10 and used == 0 and charged > 0 and grants(U1) == "z18 Tmp:30", f"{row} · {grants(U1)}")

section("переименование — имя у живых остатков")
s, d = api("PATCH", f"/admin/zones/{ZT}", {"name": "z18 Tmp-2"}, "boss")
s2, mp = api("GET", "/me/packages", None, G1)
check("«z18 Tmp» → «z18 Tmp-2»: у выдач обоих гостей — новое имя, у гостя в /me/packages — тоже; в журнале — сколько остатков",
      s == 200 and grants(U1) == "z18 Tmp-2:30" and grants(U2) == "z18 Tmp-2:15"
      and [(x.get("zone"), x.get("minutes")) for x in mp.get("minutes_by_zone", [])] == [("z18 Tmp-2", 30)]
      and "z18 Tmp → z18 Tmp-2 · имя у живых остатков: выдач минут — 2, пакетов — 0" in sql(
          "SELECT details FROM admin_actions WHERE action = 'zone_update' ORDER BY created_at DESC LIMIT 1"),
      f"{s} {grants(U1)} {grants(U2)} · {J(mp.get('minutes_by_zone'))}")

section("мастер: перенос в другую зону")
bad = [api("POST", f"/admin/zones/{ZT}/move-minutes", b, "boss") for b in
       ({}, {"to_zone_id": ZT}, {"to_zone_id": "00000000-0000-0000-0000-000000000000"}, {"to_zone_id": STD, "burn": True})]
s, d = api("POST", f"/admin/zones/{ZT}/move-minutes", {"to_zone_id": STD}, "admin1")
check("без цели, в ту же зону, в несуществующую, «и перенести, и сжечь» — 400 bad_target, 400 same_zone, 404, 400; "
      "админу — 403 (только владелец)",
      [(x[0], x[1].get("code")) for x in bad] == [(400, "bad_target"), (400, "same_zone"), (404, "to_zone_not_found"),
                                                   (400, "bad_target")] and s == 403, J([(x[0], x[1].get("code")) for x in bad]) + f" · {s}")
s, d = api("POST", f"/admin/zones/{ZT}/move-minutes", {"to_zone_id": STD}, "boss")
n1, n2 = note(U1, "zone_minutes_moved"), note(U2, "zone_minutes_moved")
check("перенос в Standard — 200: 45 мин у 2 гостей; выдачи — «Standard», минута в минуту",
      s == 200 and d.get("minutes") == 45 and d.get("guests") == 2 and (d.get("to") or {}).get("name") == "Standard"
      and grants(U1) == "Standard:30" and grants(U2) == "Standard:15", f"{s} {J(d)} · {grants(U1)} {grants(U2)}")
check("каждому гостю — уведомление zone_minutes_moved (сколько, откуда, куда), в журнал — строка на гостя",
      '"from": "z18 Tmp-2"' in n1 and '"to": "Standard"' in n1 and '"minutes": 30' in n1 and '"minutes": 15' in n2
      and last_audit("zone_minutes_move", U1) == "минуты зоны z18 Tmp-2 → Standard: 30 мин"
      and last_audit("zone_minutes_move", U2) == "минуты зоны z18 Tmp-2 → Standard: 15 мин", f"{n1} · {n2} · {last_audit('zone_minutes_move', U1)}")
sid = seat(G1, "PC-08", minutes_ago=5)
s, d = end(sid)
row = sql(f"SELECT billed_minutes || '|' || coin_minutes_used || '|' || charged_grosz FROM sessions WHERE id = '{sid}'")
b, used, charged = (int(x) for x in row.split("|"))
check(f"на PC-08 (Standard) перенесённые минуты тратятся: {b} мин — из них, кошелёк не тронут",
      s == 200 and b >= 5 and used == b and charged == 0 and grants(U1) == f"Standard:{30 - b}", f"{row} · {grants(U1)}")
s, d = api("POST", f"/admin/zones/{ZT}/move-minutes", {"to_zone_id": STD}, "boss")
s2, d2 = api("DELETE", f"/admin/zones/{ZT}", None, "boss")
check("повтор переноса — 409 no_minutes; удаление теперь — 200, зоны нет",
      s == 409 and d.get("code") == "no_minutes" and s2 == 200 and zone(ZT) is None, f"{s} {d.get('code')} · {s2} {J(d2)}")

section("мастер: сжечь")
s, d = api("POST", "/admin/zones", {"name": "z18 Tmp", "rate_pln": 10}, "boss")
ZB = (d.get("zone") or {}).get("id")
s1, r1 = api("POST", f"/admin/users/{U2}/redeem", {"zone_id": ZB, "minutes": 20}, "admin1")
s, d = api("DELETE", f"/admin/zones/{ZB}", None, "boss")
check("новая «z18 Tmp», Guest49 — 20 мин; удаление — 409 «У 1 гостя 20 мин этой зоны»",
      s1 == 200 and s == 409 and d.get("message", "").startswith("У 1 гостя 20 мин этой зоны"), f"{s1} {s} {d.get('message')}")
burned = lambda url="/admin/reports/money?preset=today": api("GET", url, None, "boss")[1].get("burned") or {}
B0 = burned()
s, d = api("POST", f"/admin/zones/{ZB}/move-minutes", {"burn": True}, "boss")
nb = note(U2, "zone_minutes_burned")
check("сжечь — 200: 20 мин у 1 гостя на 3,33 zł по тарифу выдачи; остаток выдачи — 0; гостю — zone_minutes_burned; в журнале",
      s == 200 and d.get("burned") is True and d.get("minutes") == 20 and d.get("guests") == 1 and d.get("value_pln") == 3.33
      and grants(U2).endswith("z18 Tmp:0") and '"zone": "z18 Tmp"' in nb and '"minutes": 20' in nb
      and last_audit("zone_minutes_burn", U2) == "минуты зоны z18 Tmp сожжены: 20 мин (3.33 zł по тарифу выдачи)",
      f"{s} {J(d)} · {grants(U2)} · {nb} · {last_audit('zone_minutes_burn', U2)}")
s, d = api("DELETE", f"/admin/zones/{ZB}", None, "boss")
check("после сжигания удаление — 200", s == 200 and zone(ZB) is None, f"{s} {J(d)}")
mark = sql(f"SELECT burned_minutes || '|' || (burned_at IS NOT NULL)::text || '|' || (burned_at > now() - interval '5 minutes')::text "
           f"|| '|' || COALESCE(zone_id::text, 'нет') FROM minute_grants WHERE user_id = '{U2}' AND zone_name = 'z18 Tmp'")
check("З18-и1а: у сожжённой выдачи — сколько и когда (burned_minutes 20, burned_at — только что); зоны у неё больше нет, имя осталось",
      mark == "20|true|true|нет", mark)
B1, BK = burned(), burned("/admin/cash/book?preset=today")
m0, m1 = B0.get("minutes") or {}, B1.get("minutes") or {}
it = next((x for x in B1.get("items") or [] if x.get("kind") == "minutes" and x.get("guest") == G2), {})
check("«Сгорело за период» за сегодня: «минуты» +1 строка, +20 мин, +3,33 zł; итог +3,33 zł — сумма пакетов, минут и монет",
      m1.get("count") == (m0.get("count") or 0) + 1 and m1.get("minutes") == (m0.get("minutes") or 0) + 20
      and near(m1.get("pln", 0) - (m0.get("pln") or 0), 3.33) and near(B1.get("pln", 0) - (B0.get("pln") or 0), 3.33)
      and near(B1.get("pln", 0), sum((B1.get(k) or {}).get("pln", 0) for k in ("packages", "minutes", "coins"))), J([m0, m1, B1.get("pln")]))
check("в списке сгоревшего — «минуты» Guest49: зона «z18 Tmp» (её уже удалили), 20 мин, 3,33 zł; «Касса» отдаёт то же",
      it.get("name") == "z18 Tmp" and it.get("qty") == 20 and it.get("pln") == 3.33 and BK.get("minutes") == m1
      and BK.get("pln") == B1.get("pln"), J([it, BK.get("minutes")]))

section("пакеты держат зону")
s, d = api("POST", "/admin/zones", {"name": "z18 Пак", "rate_pln": 12}, "boss")
ZP = (d.get("zone") or {}).get("id")
s1, p = api("POST", "/admin/packages", {"name": "z18 час", "zone_id": ZP, "minutes": 60, "price_pln": 10}, "boss")
PK = (p.get("package") or p).get("id")
s2, d2 = api("DELETE", f"/admin/zones/{ZP}", None, "boss")
check("непроданный пакет каталога — 409 has_packages «сначала удали их в «Пакетах времени»»",
      s1 == 201 and s2 == 409 and d2.get("code") == "has_packages" and "сначала удали их" in d2.get("message", ""), f"{s1} {s2} {J(d2)}")
sql(f"""INSERT INTO user_packages (club_id, user_id, package_id, zone_id, name, zone_name, minutes_total, minutes_left, price_pln,
    method, comp_reason, voided_at, created_at) SELECT club_id, '{U1}', '{PK}', id, 'z18 час', name, 60, 0, 0, 'comp', 'z18 проверка',
    '2020-01-01', '2020-01-01' FROM zones WHERE id = '{ZP}'""")  # подарок, отменён, в 2020-м — ни в выручку, ни в отчёты
s, d = api("DELETE", f"/admin/zones/{ZP}", None, "boss")
z = zone(ZP) or {}
s3, d3 = api("PATCH", f"/admin/zones/{ZP}", {"name": "z18 Пак"}, "boss")
check("продажа пакета (даже отменённая, 2020 год) — 409 package_history «только переименовать», в списке — package_sales 1; "
      "переименовать можно",
      s == 409 and d.get("code") == "package_history" and d.get("sales") == 1 and "можно только переименовать" in d.get("message", "")
      and z.get("package_sales") == 1 and z.get("delete_block") == "package_history" and s3 == 200, f"{s} {J(d)} · {J(z)} · {s3}")
done()
