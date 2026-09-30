#!/usr/bin/env python3
"""З11-и1 «Способ возврата — сервер» (№17; решения основателя 30.09). Отмена депозита, продажи и чека, выданного пакета и
оплаченного заказа кухни принимает «как вернули» — refund_method: cash (наличными), card, blik, wallet (на кошелёк), none
(не возвращали); без поля — как платили. Способ ложится в строку отмены; «должно быть в ящике» уменьшают только наличные
возвраты: вернули картой — ящик цел, налом — меньше. Админ возвращает тем же способом, каким платили, или не возвращает —
другой способ выбирает владелец (403 refund_owner_only); на кошелёк — только оплаченное кошельком (400 refund_wallet_only);
у долга и неоплаченного заказа — только «не возвращали» (400 refund_unpaid); кривой способ — 400. Журнал — «вернули …» /
«денег не возвращали». Отменённые до миграции 079 — «как платили».
Данные: временный админ z11rf на смене (ящик заводится, если его нет); гости Guest41 (кошелёк докладывается до 60 zł) и
Guest42 (кошелёк докладывается до 10 zł; за PC-08 — продажа и заказ кухни в долг, сессия в конце завершается); позиции «z11 Возврат A» (10 zł) и «z11
Возврат B» (5 zł) заводятся каждый прогон и в конце удаляются вместе с поставками; самый дешёвый пакет каталога. Всё, что
проверка продаёт, пополняет и выдаёт, по ходу отменяется; наличные туда и обратно сходятся — ящик в конце как в начале."""
from _z import *

A, B, MARK = "z11 Возврат A", "z11 Возврат B", "z11 возврат"
G41, G42 = uid("Guest41"), uid("Guest42")


def gone():
    for oid in rows(f"SELECT id FROM kitchen_orders WHERE name IN ('{A}', '{B}') AND status IN ('new','accepted','preparing','delivering')"):
        api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for sid in rows(f"SELECT id FROM sales WHERE name IN ('{A}', '{B}') AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {}, "boss")
    for n in (A, B):
        for gid in rows(f"SELECT id FROM goods WHERE name = '{n}'"):
            drop_good(gid)


gone()
end_all("Guest42")
hire_temp("z11rf")
tok("z11rf")  # вход в админку открывает смену
drawer_ready("z11rf")
at_end(lambda: end_all("Guest42"))
at_end(gone)  # уборка — в обратном порядке: продажи и заказы, потом сессия
ensure_wallet("Guest41", 60)
deps, packs = [], []
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in deps if sql(f"SELECT voided_at IS NULL FROM deposits WHERE id = '{d}'") == "t"])
at_end(lambda: [api("POST", f"/admin/user-packages/{p}/void", {}, "boss") for p in packs if sql(f"SELECT voided_at IS NULL FROM user_packages WHERE id = '{p}'") == "t"])
mk = lambda name, price: (api("POST", "/admin/goods", {"name": name, "category": "z11", "price_pln": price, "stock": 20}, "boss")[1].get("good") or {}).get("id")
ga, gb = mk(A, 10), mk(B, 5)
exp = lambda: api("GET", "/admin/work/closeout", None, "z11rf")[1].get("expected_pln", 0)
E0 = exp()
rm = lambda table, i: sql(f"SELECT COALESCE(refund_method, '-') FROM {table} WHERE id = '{i}'") if i else "?"
body = lambda how: {} if how is None else {"refund_method": how}


def sell(method, g=None, **kw):
    d = api("POST", "/admin/sales", {"good_id": g or ga, "qty": 1, "method": method, **kw}, "z11rf")[1]
    return (d.get("sale") or {}).get("id"), (d.get("receipt") or {}).get("id")


def void(sid, who="z11rf", how=None):
    s, d = api("POST", f"/admin/sales/{sid}/void", body(how), who)
    return s, d.get("code") or d.get("refund_method")


section("продажа: как вернули")
s1, _ = sell("cash")
e1 = exp()
r1 = void(s1)
check("без поля — как платили: продажа налом → вернули наличными (в ответе и в строке), «должно быть в ящике» −10",
      r1 == (200, "cash") and rm("sales", s1) == "cash" and near(exp(), e1 - 10), J([r1, rm("sales", s1), e1, exp()]))
s2, _ = sell("cash")
e2 = exp()
r2 = [void(s2, "z11rf", "card"), void(s2, "boss", "card")]
check("налом платили, вернули на карту: админу — 403 refund_owner_only, владельцу — 200; ящик цел",
      r2 == [(403, "refund_owner_only"), (200, "card")] and rm("sales", s2) == "card" and near(exp(), e2), J([r2, e2, exp()]))
s3, _ = sell("card")
e3 = exp()
r3 = void(s3, "boss", "cash")
check("картой платили, владелец вернул наличными → ящик −10", r3 == (200, "cash") and rm("sales", s3) == "cash" and near(exp(), e3 - 10),
      J([r3, e3, exp()]))
s4, _ = sell("cash")
e4 = exp()
r4 = void(s4, "z11rf", "none")
check("«не возвращали» — и админу: деньги остались, ящик цел", r4 == (200, "none") and rm("sales", s4) == "none" and near(exp(), e4), J([r4, e4, exp()]))
s5, _ = sell("cash")
r5 = [void(s5, "boss", "wallet"), void(s5, "z11rf", "налом"), void(s5, "z11rf", "BLIK")]
check("на кошелёк то, что платили налом, — 400 refund_wallet_only (и владельцу); кривой способ — 400; продажа цела",
      r5 == [(400, "refund_wallet_only"), (400, "bad_refund_method"), (400, "bad_refund_method")]
      and sql(f"SELECT voided_at IS NULL FROM sales WHERE id = '{s5}'") == "t", J(r5))
void(s5)

section("чек")
d = api("POST", "/admin/sales", {"items": [{"good_id": ga, "qty": 1}, {"good_id": gb, "qty": 1}], "method": "card"}, "z11rf")[1]
rid = (d.get("receipt") or {}).get("id")
e6 = exp()
rr = [api("POST", f"/admin/receipts/{rid}/void", body("blik"), who) for who in ("z11rf", "boss")]
rr = [(s, x.get("code") or x.get("refund_method")) for s, x in rr]
got = rows(f"SELECT refund_method FROM sales WHERE receipt_id = '{rid}'") if rid else []
check("чек картой, вернули по BLIK: админу — 403, владельцу — 200, у обеих позиций «blik»; ящик цел",
      rr == [(403, "refund_owner_only"), (200, "blik")] and got == ["blik", "blik"] and near(exp(), e6), J([rr, got]))

section("долг и неоплаченный заказ")
ensure_wallet("Guest42", 10)  # посадке нужен запас на старт
seat("Guest42", "PC-08")
sd, _ = sell("later", guest_id=G42)
rd = [void(sd, "z11rf", "cash"), void(sd)]
ko = (api("POST", "/admin/kitchen/orders", {"guest_id": G42, "good_id": ga, "qty": 1, "method": "later"}, "z11rf")[1].get("order") or {}).get("id")
rk = [api("POST", f"/admin/kitchen/orders/{ko}/cancel", body(h), "z11rf") for h in ("card", None)]
rk = [(s, x.get("code") or ("refund" in x)) for s, x in rk]
check("продажа в долг: «вернули налом» — 400 refund_unpaid, без поля — «не возвращали»; заказ кухни при выходе: «на карту» — 400, "
      "без поля — отменён без возврата", rd == [(400, "refund_unpaid"), (200, "none")] and rm("sales", sd) == "none"
      and rk == [(400, "refund_unpaid"), (200, False)], J([rd, rk]))

section("депозит")
dep = lambda pln, method: deps.append(topup("Guest41", pln, method=method, who="z11rf")) or deps[-1]
dv = lambda i, who="z11rf", how=None: (lambda r: (r[0], r[1].get("code") or r[1].get("refund_method")))(api("POST", f"/admin/deposits/{i}/void", body(how), who))
d1 = dep(20, "cash")
e7 = exp()
r7 = [dv(d1, how="card"), dv(d1, how="wallet"), dv(d1)]
check("депозит налом: «на карту» админу — 403, «на кошелёк» — 400 (депозит и есть кошелёк), без поля — наличными, ящик −20",
      r7 == [(403, "refund_owner_only"), (400, "refund_wallet_only"), (200, "cash")] and rm("deposits", d1) == "cash" and near(exp(), e7 - 20),
      J([r7, e7, exp()]))
d2, d3 = dep(20, "card"), dep(20, "cash")
e8 = exp()
r8 = [dv(d2, "boss", "cash"), dv(d3, "z11rf", "none")]
check("депозит картой — владелец вернул наличными (ящик −20); депозит налом — админ «не возвращал» (деньги в ящике, +20 осталось)",
      r8 == [(200, "cash"), (200, "none")] and (rm("deposits", d2), rm("deposits", d3)) == ("cash", "none") and near(exp(), e8 - 20),
      J([r8, e8, exp()]))

section("пакет")
pack = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
price = float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{pack}'") or 0)


def issue(method, g=G41):  # гость без сессии: биллинг не трогает минуты пакета
    p = (api("POST", f"/admin/users/{g}/packages", {"package_id": pack, "method": method}, "z11rf")[1].get("package") or {}).get("id")
    packs.append(p)
    return p


pv = lambda i, who="z11rf", how=None: (lambda r: (r[0], r[1].get("code") or (r[1].get("refund_method"), r[1].get("refund_pln"))))(
    api("POST", f"/admin/user-packages/{i}/void", body(how), who))
p1, p2 = issue("cash"), issue("card")
e9 = exp()
r9 = [pv(p1, how="card"), pv(p1, "boss", "card"), pv(p2, "boss", "cash")]
check(f"пакет налом — владелец вернул на карту {price:.2f} (ящик цел), пакет картой — наличными (ящик −{price:.2f}); админу чужой способ — 403",
      r9 == [(403, "refund_owner_only"), (200, ("card", price)), (200, ("cash", price))] and (rm("user_packages", p1), rm("user_packages", p2)) == ("card", "cash")
      and near(exp(), e9 - price), J([r9, e9, exp()]))
w0 = wallet("Guest41")
p3, p4 = issue("wallet", G41), issue("cash")
r10 = [pv(p3), pv(p4, "boss", "wallet"), pv(p4)]
check("пакет с кошелька — без поля на кошелёк, кошелёк как был; пакет налом «на кошелёк» — 400 и владельцу",
      r10 == [(200, ("wallet", price)), (400, "refund_wallet_only"), (200, ("cash", price))] and wallet("Guest41") == w0, J([r10, w0, wallet("Guest41")]))

section("заказ кухни с кошелька")
d = api("POST", "/admin/kitchen/orders", {"guest_id": G41, "good_id": ga, "qty": 1, "method": "wallet"}, "z11rf")[1]
ko, ks = (d.get("order") or {}).get("id"), d.get("sale_id")
w1, e11 = wallet("Guest41"), exp()
rk = [api("POST", f"/admin/kitchen/orders/{ko}/cancel", body("cash"), who) for who in ("z11rf", "boss")]
rk = [(s, x.get("code") or x.get("refund")) for s, x in rk]
check("заказ с кошелька, вернули наличными: админу — 403, владельцу — 200 «наличными 10,00»; кошелёк не пополнился, ящик −10",
      rk == [(403, "refund_owner_only"), (200, {"method": "cash", "pln": 10})] and rm("sales", ks) == "cash" and wallet("Guest41") == w1
      and near(exp(), e11 - 10), J([rk, w1, wallet("Guest41"), e11, exp()]))

section("журнал и старые отмены")
texts = {x.get("id", ""): x.get("text", "") for k in ("deposit_void", "sale_void") for x in audit("boss", kind=k, limit=100)}
acts = [x.get("text", "") for x in audit("boss", limit=150) if x.get("kind") in ("package_void", "kitchen_cancel")]
check("журнал: у отмены — как вернули («вернули наличными», «денег не возвращали», «возврат … на карту», «возврат 10.00 zł наличными»)",
      texts.get(f"deposit_void:{d2}", "").endswith("· вернули наличными") and texts.get(f"deposit_void:{d3}", "").endswith("· денег не возвращали")
      and texts.get(f"sale_void:{s2}", "").endswith("· вернули на карту") and texts.get(f"sale_void:{s4}", "").endswith("· денег не возвращали")
      and texts.get(f"sale_void:{sd}", "").endswith("· денег не брали, товар вернулся на склад")
      and any(t.endswith(f", возврат {price:.2f} zł на карту") for t in acts) and any(f"{A} ×1 · возврат 10.00 zł наличными" in t for t in acts),
      J([texts.get(f"deposit_void:{d2}"), texts.get(f"sale_void:{s4}"), acts[:3]], 600))
old = sql("SELECT (SELECT count(*) FROM deposits WHERE voided_at IS NOT NULL AND refund_method IS NULL AND method IN ('cash','card','blik')) + "
          "(SELECT count(*) FROM sales WHERE voided_at IS NOT NULL AND refund_method IS NULL AND (paid_at IS NULL OR method IN ('cash','card','blik','wallet'))) + "
          "(SELECT count(*) FROM user_packages WHERE voided_at IS NOT NULL AND refund_method IS NULL AND method IN ('cash','card','blik','wallet'))")
check("все отменённые — со способом возврата: старые — «как платили» (миграция 079), новые — как вернули", old == "0", old)
check("наличные туда и обратно сошлись: «должно быть в ящике» — как в начале проверки", near(exp(), E0), J([E0, exp()]))
done()
