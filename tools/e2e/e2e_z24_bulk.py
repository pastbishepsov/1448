#!/usr/bin/env python3
"""З24-и4 «Массовые правки цен» (№53 «в»): POST /admin/prices/bulk (владелец) — зоны, товары (фильтр — категория) или
пакеты (фильтр — зона) разом: «+10 %», «−7,5 %», «+2 zł» или только округление — до 0,50 или до целых. Просмотр
(apply: false) ничего не пишет; запись (apply: true) — с ценами просмотра (expect): цену правили между просмотром и
записью — 409 prices_moved со списком, не пишется ничего; цена вышла бы за границы — 400 out_of_range со списком.
Одна запись — одна транзакция и одна строка журнала «prices_bulk». Админу — 403.
Данные: временные позиции «z24b …» (категория z24b; одна выключенная), зоны «z24b …» и пакеты «z24b …» в своей зоне;
все — свои, чужие цены проверка не трогает (зоны — списком ids). В конце позиции — drop_good, пакеты и зоны — SQL-ом."""
from _z import *

PRE, CAT = "z24b ", "z24b"


def wipe():
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{PRE}%'"):
        drop_good(gid)
    sql(f"DELETE FROM time_packages WHERE name LIKE '{PRE}%'")
    sql(f"DELETE FROM zones WHERE name LIKE '{PRE}%'")


wipe()
at_end(wipe)
bulk = lambda who="boss", **kw: api("POST", "/admin/prices/bulk", kw, who)
seen = lambda: {x.get("id") for x in audit("boss", limit=60)}
A0 = seen()
price = lambda table, gid, col="price_pln": float(sql(f"SELECT {col} FROM {table} WHERE id = '{gid}'") or 0)
mk = lambda name, p, **kw: (api("POST", "/admin/goods", {"name": PRE + name, "category": CAT, "price_pln": p, **kw}, "boss")[1].get("good") or {}).get("id")
G = {"Cola": mk("Cola", 5), "Woda": mk("Woda", 3), "Lays": mk("Lays", 6.2), "Off": mk("Off", 4, active=False)}
Z = {n: (api("POST", "/admin/zones", {"name": PRE + n, "rate_pln": r}, "boss")[1].get("zone") or {}).get("id")
     for n, r in (("Зал", 20), ("Ложа", 30), ("Дёшево", 0.1))}
P = {n: (api("POST", "/admin/packages", {"name": PRE + n, "zone_id": Z["Зал"], "minutes": m, "price_pln": p}, "boss")[1].get("package") or {}).get("id")
     for n, m, p in (("3ч", 180, 45), ("5ч", 300, 70))}
check("данные: 4 позиции z24b (одна выключена), 3 зоны и 2 пакета «z24b …»", all(G.values()) and all(Z.values()) and all(P.values()),
      J([G, Z, P]))
exp = lambda d: {r["id"]: r["was"] for r in d.get("rows") or []}
names = lambda d: [(r.get("name"), r.get("was"), r.get("will")) for r in d.get("rows") or []]

section("товары: категория, +10 %, до 0,50")
s1, d1 = bulk(kind="goods", category=CAT, pct=10, round=0.5)
check("просмотр: только включённые позиции z24b — Cola 5 → 5,50, Lays 6,20 → 7,00 (6,82), Woda 3 → 3,50 (3,30); ничего не записано",
      s1 == 200 and d1.get("applied") is False and sorted(names(d1)) == sorted([(PRE + "Cola", 5, 5.5), (PRE + "Lays", 6.2, 7), (PRE + "Woda", 3, 3.5)])
      and d1.get("changed") == 3 and d1.get("op") == "+10 %, до 0,50" and d1.get("scope") == f"товары «{CAT}»"
      and price("goods", G["Cola"]) == 5, J([s1, d1], 600))
s1b, d1b = bulk(kind="goods", category=CAT, pct=10, round=0.5, all=True)
check("и выключенные (all) — Off 4 → 4,50 (4,40)", s1b == 200 and (PRE + "Off", 4, 4.5) in names(d1b) and len(d1b.get("rows") or []) == 4, J(names(d1b)))
s2, d2 = bulk(kind="goods", category=CAT, pct=10, round=0.5, apply=True)
check("запись без цен просмотра — 400 need_preview", s2 == 400 and d2.get("code") == "need_preview", J([s2, d2]))
s3, d3 = bulk(kind="goods", category=CAT, pct=10, round=0.5, apply=True, expect=exp(d1))
check("запись с ценами просмотра — 200: Cola 5,50, Woda 3,50, Lays 7,00; выключенная Off — как была, 4",
      s3 == 200 and d3.get("applied") is True and d3.get("changed") == 3 and price("goods", G["Cola"]) == 5.5
      and price("goods", G["Woda"]) == 3.5 and price("goods", G["Lays"]) == 7 and price("goods", G["Off"]) == 4, J([s3, d3], 500))

section("цену правили между просмотром и записью — 409, ничего не записано")
s4, d4 = bulk(kind="goods", category=CAT, add_pln=1)
api("PATCH", f"/admin/goods/{G['Woda']}", {"price_pln": 4}, "boss")
s5, d5 = bulk(kind="goods", category=CAT, add_pln=1, apply=True, expect=exp(d4))
moved = d5.get("rows") or []
check("Woda стала 4 после просмотра — 409 prices_moved: «Woda» было 3,50, сейчас 4; Cola и Lays не тронуты",
      s4 == 200 and s5 == 409 and d5.get("code") == "prices_moved" and len(moved) == 1 and moved[0].get("name") == PRE + "Woda"
      and moved[0].get("was") == 3.5 and moved[0].get("now") == 4 and price("goods", G["Cola"]) == 5.5 and price("goods", G["Lays"]) == 7,
      J([s5, d5]))
s6, d6 = bulk(kind="goods", ids=[G["Cola"], G["Lays"]], round=1)
s7, d7 = bulk(kind="goods", ids=[G["Cola"], G["Lays"]], round=1, apply=True, expect=exp(d6))
check("только округление до целых, выбранные — Cola 5,50 → 6, Lays 7 — без изменений (1 из 2); Woda не в выборе — 4",
      s6 == 200 and d6.get("op") == "округление до целых" and s7 == 200 and d7.get("changed") == 1 and price("goods", G["Cola"]) == 6
      and price("goods", G["Lays"]) == 7 and price("goods", G["Woda"]) == 4, J([s7, d7], 400))

section("зоны и пакеты")
zids = [Z["Зал"], Z["Ложа"]]
s8, d8 = bulk(kind="zones", ids=zids, add_pln=2)
s9, d9 = bulk(kind="zones", ids=zids, add_pln=2, apply=True, expect=exp(d8))
check("зоны списком: +2 zł — «z24b Зал» 20 → 22, «z24b Ложа» 30 → 32; чужие зоны — как были",
      s9 == 200 and d9.get("changed") == 2 and price("zones", Z["Зал"], "rate_pln") == 22 and price("zones", Z["Ложа"], "rate_pln") == 32
      and len(d8.get("rows") or []) == 2, J([s9, d9], 400))
s10, d10 = bulk(kind="zones", ids=[Z["Дёшево"]], add_pln=-0.2)
s11, d11 = bulk(kind="zones", ids=[Z["Дёшево"]], add_pln=-0.2, apply=True, expect=exp(d10))
check("за границы: «z24b Дёшево» 0,10 − 0,20 zł — в просмотре out_of_range, запись — 400 out_of_range, цена — 0,10",
      s10 == 200 and len(d10.get("out_of_range") or []) == 1 and s11 == 400 and d11.get("code") == "out_of_range"
      and (d11.get("rows") or [{}])[0].get("name") == PRE + "Дёшево" and price("zones", Z["Дёшево"], "rate_pln") == 0.1, J([d10, s11, d11], 500))
s12, d12 = bulk(kind="packages", zone_id=Z["Зал"], pct=-7.5, round=0.5)
s13, d13 = bulk(kind="packages", zone_id=Z["Зал"], pct=-7.5, round=0.5, apply=True, expect=exp(d12))
check("пакеты зоны «z24b Зал»: −7,5 %, до 0,50 — 3ч 45 → 41,50 (41,625), 5ч 70 → 65 (64,75)",
      s13 == 200 and d12.get("scope") == f"пакеты зоны «{PRE}Зал»" and price("time_packages", P["3ч"]) == 41.5 and price("time_packages", P["5ч"]) == 65,
      J([d12, s13, d13], 500))

section("проверки, права, журнал")
bad = [("чужой вид — 400 bad_kind", {"kind": "users", "pct": 10}, "bad_kind"),
       ("ни процентов, ни злотых, ни округления — 400 bad_op", {"kind": "goods"}, "bad_op"),
       ("и проценты, и злотые — 400 bad_op", {"kind": "goods", "pct": 5, "add_pln": 1}, "bad_op"),
       ("−95 % — 400 bad_pct", {"kind": "goods", "pct": -95}, "bad_pct"),
       ("округление до 0,10 — 400 bad_round", {"kind": "goods", "pct": 5, "round": 0.1}, "bad_round"),
       ("пакеты чужой зоны — 404 zone_not_found", {"kind": "packages", "zone_id": "00000000-0000-0000-0000-000000000000", "pct": 5}, "zone_not_found"),
       ("категория без позиций — 400 nothing_selected", {"kind": "goods", "category": "z24b-нет", "pct": 5}, "nothing_selected")]
res = [(n, *bulk(**body)) for n, body, _ in bad]
check("кривые запросы: " + "; ".join(n for n, _, _ in bad),
      all(d.get("code") == want for (_, _, d), (_, _, want) in zip(res, bad)), J([(n, s, d.get("code")) for n, s, d in res]))
sa, _ = bulk("admin1", kind="goods", category=CAT, pct=10)
check("админу — 403", sa == 403, sa)
aud = [(x.get("kind"), x.get("text", "")) for x in audit("boss", limit=60) if x.get("id") not in A0 and x.get("kind") == "prices_bulk"]
want = [f"товары «{CAT}»: +10 %, до 0,50 — 3 из 3: {PRE}Cola 5,00 → 5,50 zł; {PRE}Lays 6,20 → 7,00 zł; {PRE}Woda 3,00 → 3,50 zł",
        f"товары: округление до целых — 1 из 2: {PRE}Cola 5,50 → 6,00 zł",
        f"зоны: +2,00 zł — 2 из 2: {PRE}Зал 20,00 → 22,00 zł; {PRE}Ложа 30,00 → 32,00 zł",
        f"пакеты зоны «{PRE}Зал»: −7,5 %, до 0,50 — 2 из 2: {PRE}3ч 45,00 → 41,50 zł; {PRE}5ч 70,00 → 65,00 zł"]
check("журнал: четыре строки «prices_bulk» — по записи, отказы (409, 400) не пишутся", sorted(t for _, t in aud) == sorted(want), J(aud, 900))

done()
