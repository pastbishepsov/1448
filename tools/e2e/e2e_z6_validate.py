#!/usr/bin/env python3
"""З6-и3 «Деньги и числа — только валидные» (часть №40): цена зоны, товара и пакета — от 0,01 zł и не больше двух
знаков после запятой (0,004 падало ошибкой сервера на CHECK базы, 1,005 принималось, а в базе становилось 1,01);
остаток, приход и корректировка склада — до 1 000 000 штук, порядок в списках — ±10 000, в том числе у сортировки
каталога у стойки (3·10⁹ падало ошибкой сервера); категорию товара можно очистить; примечание к приходу и
корректировке — до 200 символов (З-Р36). На кривой ввод — понятный отказ 400, и ни одного 500 за всю проверку.
Временные зона «z6 проверка», её пакет и товар «z6 проверка …» убираются в конце, порядок у записи каталога
и шаблона смены возвращается как был."""
import time
from _z import *

codes = []  # (статус, запрос) всех вызовов — «ни одного 500»

def call(method, path, body=None, who="boss"):
    s, d = api(method, path, body, who)
    codes.append((s, f"{method} {path}"))
    return s, d

def refused(name, s, d, code, status=400):
    return check(f"{name} → {status} {code}", s == status and d.get("code") == code, f"{s} {J(d)}")

def drop_good(gid):
    """Временный товар стенда: движения и позиция — SQL-ом (удаление через API у позиции с историей — 409)."""
    if gid:
        sql(f"DELETE FROM stock_moves WHERE good_id='{gid}'; DELETE FROM goods WHERE id='{gid}'")

def new_good(name, **kv):
    s, d = call("POST", "/admin/goods", {"name": name, "category": "z6", "active": False, **kv})
    gid = d.get("good", {}).get("id", "")
    if gid:
        at_end(lambda: drop_good(gid))
    return s, d, gid

ts = int(time.time())

section("цены: от 0,01 zł и не больше двух знаков после запятой")
zid = sql("SELECT id FROM zones WHERE name='z6 проверка'")
if not zid:
    s, d = call("POST", "/admin/zones", {"name": "z6 проверка", "rate_pln": 10, "sort": 90})
    zid = d.get("zone", {}).get("id", "")
check("временная зона «z6 проверка» есть", zid, zid)
at_end(lambda: api("DELETE", f"/admin/zones/{zid}", None, "boss"))
s, d = call("POST", "/admin/zones", {"name": f"z6 копейки {ts}", "rate_pln": 0.004})
if s == 201:
    at_end(lambda z=d["zone"]["id"]: api("DELETE", f"/admin/zones/{z}", None, "boss"))
refused("новая зона с ценой часа 0,004 zł", s, d, "bad_rate")
s, d = call("PATCH", f"/admin/zones/{zid}", {"rate_pln": 1.005})
refused("цена часа зоны 1,005 zł", s, d, "bad_rate")
s, d = call("PATCH", f"/admin/zones/{zid}", {"rate_pln": 12.34})
check("цена часа 12,34 zł → 200, в базе 12.34", s == 200 and d.get("zone", {}).get("rate_pln") == 12.34
      and sql(f"SELECT rate_pln FROM zones WHERE id='{zid}'") == "12.34", f"{s} {J(d)}")

s, d, _ = new_good(f"z6 копейки {ts}", price_pln=0.004)
refused("новый товар по 0,004 zł", s, d, "bad_price")
s, d, _ = new_good(f"z6 три знака {ts}", price_pln=1.005)
refused("новый товар по 1,005 zł", s, d, "bad_price")
s, d, gid = new_good(f"z6 проверка {ts}", price_pln=2, stock=0)
check("временный товар «z6 проверка» заведён (2,00 zł)", s == 201 and gid, f"{s} {J(d)}")
s, d = call("PATCH", f"/admin/goods/{gid}", {"price_pln": 1.005})
refused("цена товара 1,005 zł", s, d, "bad_price")
s, d = call("PATCH", f"/admin/goods/{gid}", {"price_pln": 3e9})
refused("цена товара 3·10⁹ zł", s, d, "bad_price")
s, d = call("PATCH", f"/admin/goods/{gid}", {"price_pln": 19.99})
check("цена товара 19,99 zł → 200, в базе 19.99", s == 200 and d.get("good", {}).get("price_pln") == 19.99
      and sql(f"SELECT price_pln FROM goods WHERE id='{gid}'") == "19.99", f"{s} {J(d)}")

pack = {"name": f"z6 проверка {ts}", "zone_id": zid, "minutes": 60, "active": False}
s, d = call("POST", "/admin/packages", {**pack, "price_pln": 0.004})
if s == 201:
    at_end(lambda p=d["package"]["id"]: api("DELETE", f"/admin/packages/{p}", None, "boss"))
refused("новый пакет за 0,004 zł", s, d, "bad_price")
s, d = call("POST", "/admin/packages", {**pack, "price_pln": 1.005})
if s == 201:
    at_end(lambda p=d["package"]["id"]: api("DELETE", f"/admin/packages/{p}", None, "boss"))
refused("новый пакет за 1,005 zł", s, d, "bad_price")
s, d = call("POST", "/admin/packages", {**pack, "price_pln": 9.99})
pid = d.get("package", {}).get("id", "")
if pid:
    at_end(lambda: api("DELETE", f"/admin/packages/{pid}", None, "boss"))
check("пакет за 9,99 zł → 201", s == 201 and pid, f"{s} {J(d)}")

section("категория товара очищается")
s, _ = call("PATCH", f"/admin/goods/{gid}", {"category": "z6 кат"})
s2, d = call("PATCH", f"/admin/goods/{gid}", {"low_stock": 1})
check("правка без поля category категорию не трогает", s == 200 and s2 == 200 and d.get("good", {}).get("category") == "z6 кат", f"{s} {s2} {J(d)}")
s, d = call("PATCH", f"/admin/goods/{gid}", {"category": ""})
cat = sql(f"SELECT category FROM goods WHERE id='{gid}'")
check("пустая категория → 200, категория очищена", s == 200 and d.get("good", {}).get("category") == "" and cat == "", f"{s} {J(d)}, в базе «{cat}»")

section("примечание к приходу и корректировке — до 200 символов")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": 2, "reason": "supply", "note": "я" * 201})
refused("приход с примечанием в 201 символ", s, d, "bad_note")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": -1, "reason": "adjust", "note": "б" * 201})
refused("корректировка с причиной в 201 символ", s, d, "bad_note")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": 2, "reason": "supply", "note": "я" * 200}, "admin1")
note = sql(f"SELECT length(note) FROM stock_moves WHERE good_id='{gid}' ORDER BY created_at DESC LIMIT 1")
check("приход у стойки с примечанием ровно в 200 символов → 200", s == 200 and note == "200", f"{s} {J(d)}, в движении {note}")

section("целые в границах: склад до 1 000 000 штук, порядок ±10 000")
s, d, _ = new_good(f"z6 миллион {ts}", price_pln=1, stock=1000001)
refused("новый товар с остатком 1 000 001", s, d, "bad_stock")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": 3000000000, "reason": "supply", "note": "z6"}, "admin1")
refused("приход 3·10⁹ штук", s, d, "bad_delta")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": -3000000000, "reason": "adjust", "note": "z6"}, "admin1")
refused("корректировка −3·10⁹ штук", s, d, "bad_delta")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": 1000001, "reason": "supply", "note": "z6"}, "admin1")
refused("приход 1 000 001 штуки", s, d, "bad_delta")
st = int(sql(f"SELECT stock FROM goods WHERE id='{gid}'") or 0)
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": 1000000 - st, "reason": "supply", "note": "z6"}, "admin1")
check(f"приход до ровно 1 000 000 на складе (+{1000000 - st}) → 200", s == 200 and d.get("good", {}).get("stock") == 1000000, f"{s} {J(d)}")
s, d = call("POST", f"/admin/goods/{gid}/stock", {"delta": 1, "reason": "supply", "note": "z6"}, "admin1")
refused("ещё +1 сверх 1 000 000 на складе", s, d, "stock_limit", 409)
st = sql(f"SELECT stock FROM goods WHERE id='{gid}'")
check("остаток — ровно 1 000 000", st == "1000000", st)

s, d = call("PATCH", f"/admin/goods/{gid}", {"sort": 3000000000})
refused("порядок товара 3·10⁹", s, d, "bad_sort")
s, d = call("PATCH", f"/admin/goods/{gid}", {"sort": 10001})
refused("порядок товара 10 001", s, d, "bad_sort")
s, d = call("PATCH", f"/admin/goods/{gid}", {"sort": -10000})
check("порядок товара −10 000 → 200", s == 200 and d.get("good", {}).get("sort") == -10000, f"{s} {J(d)}")
s, d = call("POST", "/admin/zones", {"name": f"z6 порядок {ts}", "rate_pln": 10, "sort": 3000000000})
if s == 201:
    at_end(lambda z=d["zone"]["id"]: api("DELETE", f"/admin/zones/{z}", None, "boss"))
refused("новая зона с порядком 3·10⁹", s, d, "bad_sort")
s, d = call("PATCH", f"/admin/zones/{zid}", {"sort": 3000000000})
refused("порядок зоны 3·10⁹", s, d, "bad_sort")
s, d = call("PATCH", f"/admin/packages/{pid}", {"sort": 3000000000})
refused("порядок пакета 3·10⁹", s, d, "bad_sort")

app, app_sort = (rows("SELECT id || '|' || sort FROM catalog_apps ORDER BY id LIMIT 1") or ["|"])[0].split("|")
at_end(lambda: sql(f"UPDATE catalog_apps SET sort={app_sort} WHERE id='{app}'"))
s, d = call("POST", f"/admin/catalog/{app}/sort", {"sort": 3000000000}, "admin1")
refused(f"у стойки: порядок «{app}» в каталоге 3·10⁹", s, d, "bad_sort")
s, d = call("POST", f"/admin/catalog/{app}/sort", {"sort": 10001}, "admin1")
refused(f"у стойки: порядок «{app}» 10 001", s, d, "bad_sort")
s, d = call("POST", f"/admin/catalog/{app}/sort", {"sort": int(app_sort)}, "admin1")
check(f"у стойки: порядок «{app}» {app_sort} → 200", s == 200 and d.get("sort") == int(app_sort), f"{s} {J(d)}")

sh, sh_sort = (rows("SELECT id || '|' || sort FROM shifts ORDER BY sort, name LIMIT 1") or ["|"])[0].split("|")
at_end(lambda: sql(f"UPDATE shifts SET sort={sh_sort} WHERE id='{sh}'"))
s, d = call("PATCH", f"/admin/shifts/{sh}", {"sort": 3000000000})
refused("порядок шаблона смены 3·10⁹", s, d, "bad_sort")

section("ни одного 500")
bad = [f"{s} {q}" for s, q in codes if s >= 500]
check(f"за проверку {len(codes)} запросов, ответов 5xx нет", not bad, "; ".join(bad))
done()
