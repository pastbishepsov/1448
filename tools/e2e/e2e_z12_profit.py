#!/usr/bin/env python3
"""З12-и3 «Расчёт «Прибыли»» (№74; Д9; решение основателя 04.10 — потери склада вычитаются). GET /admin/reports/profit
(только владельцу): доходы по видам — пополнения, товары, пакеты, оплаченные за период живыми деньгами, и потом
отменённое тоже; − возвраты живыми деньгами по дню отмены; − себестоимость проданного и подаренного (по партиям);
− потери склада по себестоимости = валовая прибыль; − расходы по категориям без сторно и без закупок товара (Д9) =
результат; прошлый период — для сравнения. Ручной расчёт на шагах: поставка с ценой (закупка — не расход),
пополнение, пакет, продажа, продажа с отменой, подарок, бой, расход, отмена пополнения — каждая двигает ровно свои
строки, результат за все шаги — +4,00 при пакете за 50 zł. Доходы, возвраты и расходы — как прямой SQL до гроша,
себестоимость и потери — как в «Деньгах»; «прошлый период» — как отчёт за вчера.
Данные: гость Guest43; позиция «z12p Товар» (8 zł) и её поставка 10 шт по 3 zł (расход поставки в конце
сторнируется) заводятся каждый прогон и в конце удаляются; самый дешёвый пакет каталога; расход 50 zł в первой
видимой категории не-закупки (заметка «z12p…», в конце сторнируется). Пополнение отменяется по ходу, продажи и пакет —
в уборке; наличные не трогаются — всё картой."""
from _z import *

NAME, MARK = "z12p Товар", "z12p"
G43 = uid("Guest43")
PACK = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
PRICE = float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{PACK}'") or 0)
CAT, CATN = (sql("SELECT id || '|' || name FROM expense_categories WHERE NOT purchase AND NOT hidden ORDER BY sort, name LIMIT 1") or "|").split("|")
TODAY = club_day()
RH = sql("SELECT lpad(COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '0'), 2, '0') || ':00'")


def gone():
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE user_id = '{G43}' AND voided_at IS NULL"):
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for eid in rows(f"SELECT id FROM expenses WHERE storno_at IS NULL AND (note LIKE '{MARK}%' OR note LIKE '%{NAME}%')"):
        api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
at_end(gone)
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z12", "price_pln": 8}, "boss")[1].get("good") or {}).get("id")
g = lambda v: round((v or 0) * 100)


def profit(preset="today"):
    return api("GET", f"/admin/reports/profit?preset={preset}", None, "boss")[1]


def nums(p):
    """Строки «Прибыли» в грошах."""
    return {"deposits": g((p.get("income") or {}).get("deposits_pln")), "goods": g((p.get("income") or {}).get("goods_pln")),
            "packs": g((p.get("income") or {}).get("packs_pln")), "income": g((p.get("income") or {}).get("pln")),
            "returns": g((p.get("returns") or {}).get("pln")), "sold": g((p.get("cogs") or {}).get("sold_pln")),
            "gifts": g((p.get("cogs") or {}).get("gifts_pln")), "cogs": g((p.get("cogs") or {}).get("pln")),
            "losses": g((p.get("losses") or {}).get("pln")), "gross": g(p.get("gross_pln")),
            "expenses": g((p.get("expenses") or {}).get("pln")), "purchases": g((p.get("expenses") or {}).get("purchases_pln")),
            "result": g(p.get("result_pln"))}


def lines_ok(n):
    """Строки сходятся: доходы = Σ видов, себестоимость = проданное + подаренное, валовая = доходы − возвраты −
    себестоимость − потери, результат = валовая − расходы."""
    return n["income"] == n["deposits"] + n["goods"] + n["packs"] and n["cogs"] == n["sold"] + n["gifts"] \
        and n["gross"] == n["income"] - n["returns"] - n["cogs"] - n["losses"] and n["result"] == n["gross"] - n["expenses"]


def direct():
    """Доходы, возвраты живыми деньгами, расходы и закупки за сегодня прямым SQL, в грошах."""
    d0, d1 = at_club(TODAY, RH), at_club(club_day(1), RH)
    w = lambda col: f"{col} >= {d0} AND {col} < {d1}"
    live = "COALESCE(refund_method, method) IN ('cash', 'card', 'blik')"
    q = f"""SELECT concat_ws('|',
      (SELECT COALESCE(SUM(ROUND(amount_pln * 100)), 0) FROM deposits WHERE {w('created_at')}),
      (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales WHERE {w('paid_at')} AND method NOT IN ('wallet', 'comp')),
      (SELECT COALESCE(SUM(ROUND(price_pln * 100)), 0) FROM user_packages WHERE {w('created_at')} AND method NOT IN ('wallet', 'comp')),
      (SELECT COALESCE(SUM(refund_grosz), 0) FROM deposits WHERE {w('voided_at')} AND {live})
        + (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales WHERE {w('voided_at')} AND paid_at IS NOT NULL AND method <> 'comp' AND {live})
        + (SELECT COALESCE(SUM(refund_grosz), 0) FROM user_packages WHERE {w('voided_at')} AND method <> 'comp' AND {live}),
      (SELECT COALESCE(SUM(e.gross_grosz), 0) FROM expenses e JOIN expense_categories c ON c.id = e.category_id
        WHERE e.storno_at IS NULL AND NOT c.purchase AND e.op_date = '{TODAY}'),
      (SELECT COALESCE(SUM(e.gross_grosz), 0) FROM expenses e JOIN expense_categories c ON c.id = e.category_id
        WHERE e.storno_at IS NULL AND c.purchase AND e.op_date = '{TODAY}'))"""
    return dict(zip(["deposits", "goods", "packs", "returns", "expenses", "purchases"], (int(float(x)) for x in sql(q).split("|"))))


def same_as_sql(n):
    d = direct()
    bad = {k: (n[k], d[k]) for k in d if n[k] != d[k]}
    m = api("GET", "/admin/reports/money?preset=today", None, "boss")[1]
    gm, gf, ls = m.get("goods_margin") or {}, m.get("gifts") or {}, m.get("losses") or {}
    for k, v in (("sold", gm.get("cost_pln")), ("gifts", gf.get("cost_pln")), ("losses", ls.get("cost_pln"))):
        if n[k] != g(v):
            bad[k + " (как в «Деньгах»)"] = (n[k], g(v))
    return not bad, bad


section("снимок: строки сходятся, прямой SQL, «Деньги», прошлый период, права")
P0 = profit()
N0 = nums(P0)
check("строки сходятся: доходы = пополнения + товары + пакеты; себестоимость = проданное + подаренное; валовая = доходы − "
      "возвраты − себестоимость − потери; результат = валовая − расходы", P0.get("income") and lines_ok(N0), J(N0))
ok, bad = same_as_sql(N0)
check("доходы по видам, возвраты живыми деньгами, расходы и закупки — как прямой SQL до гроша; себестоимость проданного и "
      "подаренного и потери склада — как в «Деньгах»", ok, J(bad))
# З19-и1: «прошлый период» у сегодня — вчера до того же часа (суммы — в e2e_z19_reports); у «вчера» — позавчера целиком
pp, yp = P0.get("prev_period") or {}, profit("yesterday").get("prev_period") or {}
check("«прошлый период» у сегодня — вчерашние сутки до того же часа (until «ЧЧ:ММ»); у «вчера» — позавчера целиком",
      pp.get("from") == pp.get("to") == club_day(-1) and len(pp.get("until") or "") == 5
      and yp.get("from") == yp.get("to") == club_day(-2) and "until" not in yp, J([pp, yp]))
adm = api("GET", "/admin/reports/profit?preset=today", None, "admin1")[0]
check("админу — 403: «Прибыль» видит только владелец", adm == 403, adm)


def step(title, act, want):
    """act() → что сделали; want — ожидаемые приращения строк в грошах (не названные — без изменений)."""
    a = nums(profit())
    r = act()
    b = nums(profit())
    dd = {k: b[k] - a[k] for k in b}
    exp = {k: want.get(k, 0) for k in b}
    check(title, r and dd == exp and lines_ok(b), J({"было": a, "сдвиг": dd, "ждали": exp, "r": r}))
    return r


section("ручной расчёт по шагам")
first = nums(profit())
sup = None


def supply():
    global sup
    sup = (api("POST", "/admin/supplies", {"lines": [{"good_id": good, "qty": 10}], "note": f"{MARK}: приход"}, "boss")[1].get("supply") or {}).get("id")
    return api("PUT", f"/admin/supplies/{sup}/pricing", {"lines": [{"good_id": good, "prices": [{"qty": 10, "price_pln": 3}]}], "method": "bank"}, "boss")[0] == 200


step("поставка 10 шт по 3 zł с ценой: закупка 30,00 — не расход (Д9), прибыль не тронута", supply, {"purchases": 3000})
dep = step("пополнение 100 zł картой — доходы +100,00 (пополнения), результат +100,00",
           lambda: topup("Guest43", 100), {"deposits": 10000, "income": 10000, "gross": 10000, "result": 10000})
step(f"пакет за {PRICE:.2f} картой — доходы +{PRICE:.2f} (пакеты), результат +{PRICE:.2f}",
     lambda: (api("POST", f"/admin/users/{G43}/packages", {"package_id": PACK, "method": "card"}, "boss")[1].get("package") or {}).get("id"),
     {"packs": round(PRICE * 100), "income": round(PRICE * 100), "gross": round(PRICE * 100), "result": round(PRICE * 100)})
sell = lambda method, qty=1, **kw: api("POST", "/admin/sales", {"good_id": good, "qty": qty, "method": method, **kw}, "boss")
step("продажа 2 × 8 картой — доходы +16,00 (товары), себестоимость +6,00 (2 шт по 3 из партии), результат +10,00",
     lambda: sell("card", 2)[0] == 201, {"goods": 1600, "income": 1600, "sold": 600, "cogs": 600, "gross": 1000, "result": 1000})


def sell_void():
    sid = (sell("card")[1].get("sale") or {}).get("id")
    return api("POST", f"/admin/sales/{sid}/void", {"refund_method": "card"}, "boss")[1].get("refund_method") == "card"


step("продажа 8 zł картой и её отмена с возвратом на карту — доходы +8,00 (отменённое тоже доход), возвраты +8,00 в "
     "день отмены, себестоимость 0 (товар вернулся на склад), результат 0", sell_void,
     {"goods": 800, "income": 800, "returns": 800})
step("подарок 1 шт — доходов нет, себестоимость подаренного +3,00 (Д9), результат −3,00",
     lambda: sell("comp", comp_reason=f"{MARK}: проверка прибыли")[0] == 201, {"gifts": 300, "cogs": 300, "gross": -300, "result": -300})
step("бой 1 шт — потери склада +3,00 по себестоимости партии (решение основателя 04.10), результат −3,00",
     lambda: api("POST", f"/admin/goods/{good}/stock", {"delta": -1, "reason": "adjust", "note": f"{MARK}: бой"}, "boss")[0] == 200,
     {"losses": 300, "gross": -300, "result": -300})
eid = step(f"расход 50 zł («{CATN}», банк) — расходы +50,00, результат −50,00",
           lambda: (api("POST", "/admin/expenses", {"op_date": TODAY, "category_id": CAT, "amount_pln": 50, "method": "bank",
                                                     "note": f"{MARK}: проверка прибыли"}, "boss")[1].get("expense") or {}).get("id"),
           {"expenses": 5000, "result": -5000})
step("отмена пополнения с возвратом на карту — доходы те же (пополнение было), возвраты +100,00, результат −100,00",
     lambda: api("POST", f"/admin/deposits/{dep}/void", {"refund_method": "card"}, "boss")[1].get("refund_method") == "card",
     {"returns": 10000, "gross": -10000, "result": -10000})
last = nums(profit())
check(f"за все шаги результат +{(4 + PRICE - 50):.2f} = 100 + {PRICE:.0f} + 10 − 3 − 3 − 50 − 100 (ручной расчёт)",
      last["result"] - first["result"] == round((4 + PRICE - 50) * 100), J({"first": first["result"], "last": last["result"]}))
cat = next((x for x in (profit().get("expenses") or {}).get("by_category") or [] if x.get("category_id") == CAT), {})
check(f"расходы по категориям: «{CATN}» — с нашими 50,00; категории закупки в списке нет",
      cat.get("pln", 0) >= 50 and not any(x.get("category_id") in rows("SELECT id FROM expense_categories WHERE purchase")
                                          for x in (profit().get("expenses") or {}).get("by_category") or []), J(cat))
ok, bad = same_as_sql(last)
check("после всего — снова как прямой SQL и «Деньги»", ok, J(bad))
done()
