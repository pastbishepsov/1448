#!/usr/bin/env python3
"""З23-и5а «Штрихкоды — сервер» (№54 «в»): у позиции — штрихкод (goods.barcode, миграция 099): EAN-13 или EAN-8 с верной
контрольной цифрой, UPC-A (12 цифр) — тот же EAN-13 с ведущим нулём, пробелы сканера убираются; код уникален в клубе —
занят: 409 barcode_taken с именем позиции (и в гонке двух записей). POST и PATCH /admin/goods (владелец): поле barcode,
"" убирает, не прислано — как был; в ответе и в ценнике (GET /admin/goods) — barcode, null — нет; в журнале —
«штрихкод: …», «штрихкод A → B», «штрихкод убран (был …)». База держит то же: проверка формата и уникальный индекс.
Данные: позиции «z23b …» (категория z23b) заводятся каждый прогон и в конце удаляются SQL-ом (drop_good)."""
import threading

from _z import *

PRE = "z23b "
A, B, C = "5901234123457", "96385074", "4006381333931"  # EAN-13, EAN-8, EAN-13 — контрольные верные


def wipe():
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{PRE}%'"):
        drop_good(gid)


wipe()
at_end(wipe)
seen = lambda: {x.get("id") for x in audit("boss", limit=80)}
A0 = seen()
mk = lambda name, **kw: api("POST", "/admin/goods", {"name": PRE + name, "category": "z23b", "price_pln": 5, **kw}, "boss")
code = lambda gid: sql(f"SELECT COALESCE(barcode, 'нет') FROM goods WHERE id = '{gid}'")

section("позиция со штрихкодом")
s1, d1 = mk("Кола", barcode=A)
g1 = d1.get("good") or {}
cola = g1.get("id")
check("POST с EAN-13 — 201, в ответе и в базе тот же код", s1 == 201 and g1.get("barcode") == A and code(cola) == A, J([s1, g1]))
lst = {g.get("name"): g for g in api("GET", "/admin/goods", None, "admin1")[1].get("goods") or []}
check("в ценнике (и у админа) — barcode у позиции, у позиции без кода — null",
      (lst.get(PRE + "Кола") or {}).get("barcode") == A and "barcode" in (lst.get("Coca-Cola 0.5") or {"barcode": 1})
      and (lst.get("Coca-Cola 0.5") or {}).get("barcode") is None, J(lst.get(PRE + "Кола")))
s2, d2 = mk("Вода", barcode=" 590 1234 123457 ")
check("тот же код с пробелами сканера — 409 barcode_taken, в ответе — чья: «z23b Кола»",
      s2 == 409 and d2.get("code") == "barcode_taken" and d2.get("good") == PRE + "Кола" and PRE + "Кола" in (d2.get("message") or ""), J([s2, d2]))
s3, d3 = mk("Вода", barcode="036000291452")
water = (d3.get("good") or {}).get("id")
check("UPC-A из 12 цифр — 201, хранится EAN-13 с ведущим нулём", s3 == 201 and code(water) == "0036000291452", J([s3, d3.get("good")]))
bad = [("контрольная не та", "5901234123458"), ("EAN-8 с неверной контрольной", "96385075"), ("11 цифр", "59012341234"),
       ("14 цифр", "59012341234571"), ("буква", "590123412345A"), ("12 цифр — UPC-A не сходится", "590123412345")]
res = [(n, mk("Чипсы " + str(i), barcode=c)) for i, (n, c) in enumerate(bad)]
check("кривые коды — 400 bad_barcode: " + ", ".join(n for n, _ in bad),
      all(r[0] == 400 and r[1].get("code") == "bad_barcode" for _, r in res) and not rows(f"SELECT id FROM goods WHERE name LIKE '{PRE}Чипсы%'"),
      J([(n, r[0], r[1].get("code")) for n, r in res]))

section("правка штрихкода")
s4, d4 = api("PATCH", f"/admin/goods/{water}", {"barcode": B}, "boss")
s5, d5 = api("PATCH", f"/admin/goods/{water}", {"price_pln": 6}, "boss")
check("PATCH на EAN-8 — 200; правка без поля barcode код не трогает",
      s4 == 200 and (d4.get("good") or {}).get("barcode") == B and s5 == 200 and (d5.get("good") or {}).get("barcode") == B and code(water) == B,
      J([s4, d4.get("good"), s5]))
s6, d6 = api("PATCH", f"/admin/goods/{water}", {"barcode": A}, "boss")
check("PATCH на чужой код — 409 barcode_taken «z23b Кола», код прежний", s6 == 409 and d6.get("good") == PRE + "Кола" and code(water) == B, J([s6, d6]))
s7, d7 = api("PATCH", f"/admin/goods/{water}", {"barcode": "123"}, "boss")
check("PATCH на кривой — 400 bad_barcode, код прежний", s7 == 400 and d7.get("code") == "bad_barcode" and code(water) == B, J([s7, d7]))
s8, d8 = api("PATCH", f"/admin/goods/{cola}", {"barcode": ""}, "boss")
s9, d9 = api("PATCH", f"/admin/goods/{water}", {"barcode": A}, "boss")
check("код «Колы» убран (\"\" — null), и он свободен: «Вода» его берёт",
      s8 == 200 and (d8.get("good") or {}).get("barcode") is None and code(cola) == "нет" and s9 == 200 and code(water) == A, J([s8, s9, d9.get("good")]))
s10, _ = api("PATCH", f"/admin/goods/{cola}", {"barcode": C}, "admin1")
check("админ код не ставит — 403 (карточка позиции — владельца)", s10 == 403 and code(cola) == "нет", s10)
api("PATCH", f"/admin/goods/{water}", {"active": False}, "boss")
s11, d11 = api("PATCH", f"/admin/goods/{cola}", {"barcode": A}, "boss")
check("у выключенной позиции код тоже занят — 409", s11 == 409 and d11.get("good") == PRE + "Вода", J([s11, d11]))

section("журнал")
aud = [x.get("text", "") for x in audit("boss", limit=60) if x.get("id") not in A0 and PRE in (x.get("text") or "")]
want = [f"{PRE}Кола · 5,00 zł · остаток 0 · штрихкод {A}".replace("5,00", "5.00"), f"{PRE}Вода: штрихкод 0036000291452 → {B}",
        f"{PRE}Кола: штрихкод убран (был {A})", f"{PRE}Вода: штрихкод {B} → {A}"]
check("журнал: заведена с кодом, «штрихкод A → B», «штрихкод убран (был …)»", all(any(t.endswith(w) or t == w for t in aud) for w in want), J(aud, 900))

section("гонка двух записей одного кода")
res2 = {}


def race(n):
    res2[n] = mk("Гонка " + n, barcode=C)


th = [threading.Thread(target=race, args=(n,)) for n in ("1", "2")]
[t.start() for t in th]
[t.join() for t in th]
sts = sorted(v[0] for v in res2.values())
codes = [v[1].get("code") for v in res2.values() if v[0] == 409]
check("две позиции с одним кодом разом — одна 201, другая 409 barcode_taken; в базе код у одной",
      sts == [201, 409] and codes == ["barcode_taken"] and sql(f"SELECT count(*) FROM goods WHERE barcode = '{C}'") == "1", J(res2, 400))

section("база держит то же")
check("проверка формата goods_barcode_format и уникальный индекс idx_goods_barcode (клуб, код) — на месте",
      sql("SELECT count(*) FROM pg_constraint WHERE conname = 'goods_barcode_format'") == "1"
      and sql("SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_goods_barcode' AND indexdef LIKE '%(club_id, barcode)%'") == "1")

done()
