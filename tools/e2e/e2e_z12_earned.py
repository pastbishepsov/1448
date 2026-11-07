#!/usr/bin/env python3
"""З12-и2 «Расход пакетов и отработанная выручка» (№49; З-Р9; Д7; решение основателя 03.10). Биллинг пишет каждое
списание минут пакета строкой pack_spends (пакет, сессия, минуты, когда, доля цены). GET /admin/reports/money отдаёт
earned — «Отработанную выручку» за период: время с кошелька + минуты пакетов по их цене + товары у стойки по моменту
продажи (любым способом, кроме подарков) + кухня по моменту выдачи; до журнала — оценка по средней цене минуты пакета
(Д7); «отдано временем» — минуты за монеты и компенсации по тарифу зоны — рядом отдельной строкой, в выручку не входит
(решение основателя 03.10). Сессия на зонных минутах, пакете и кошельке даёт ожидаемые суммы; все части сходятся с
прямым SQL до гроша.
Данные: гость Guest44 (монеты докладываются до 1000, кошелёк — до 20 zł, пул минут без зоны — 0); за PC-07 сессия,
сдвинутая на час назад, в конце завершается; самый дешёвый пакет каталога (картой; остаток SQL-ом — 20 минут, «как
будто 160 уже отыграно»); 30 минут за монеты в Standard; позиция «z12e Товар» (7 zł) заводится каждый прогон и в конце
удаляется; продажи и пакет отменяются, заказ кухни остаётся выданным и оплаченным картой; строки расхода сессии в
середине удаляются SQL-ом — так выглядят минуты до журнала."""
from decimal import Decimal

from _z import *

NAME, MARK = "z12e Товар", "z12e"
G44 = uid("Guest44")
PACK = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
PRICE = float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{PACK}'") or 0)
PMIN = int(sql(f"SELECT minutes FROM time_packages WHERE id = '{PACK}'") or 0)
STD = sql("SELECT id FROM zones WHERE name = 'Standard'")
LEFT = 20  # минут пакета остаётся к сессии


def gone():
    end_all("Guest44")
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL AND id NOT IN (SELECT sale_id FROM kitchen_orders WHERE sale_id IS NOT NULL)"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for oid in rows(f"SELECT id FROM kitchen_orders WHERE name = '{NAME}' AND status IN ('new','accepted','preparing','delivering')"):
        api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE user_id = '{G44}' AND voided_at IS NULL"):
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for rid in rows(f"SELECT id FROM coin_redemptions WHERE user_id = '{G44}' AND voided_at IS NULL"):
        api("POST", f"/admin/coin-redemptions/{rid}/void", {}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
at_end(gone)
sql(f"UPDATE users SET coins_balance = GREATEST(coins_balance, 1000), coin_minutes = 0 WHERE id = '{G44}'")
ensure_wallet("Guest44", 20)
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z12", "price_pln": 7, "stock": 10}, "boss")[1].get("good") or {}).get("id")
RH = sql("SELECT lpad(COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '0'), 2, '0') || ':00'")
g = lambda v: round((v or 0) * 100)


def earned(preset="today"):
    return api("GET", f"/admin/reports/money?preset={preset}", None, "boss")[1].get("earned") or {}


def direct(day=0):
    """«Отработанная выручка» за клубные сутки (0 — сегодня, −1 — вчера) прямым SQL, в грошах — независимо от сервера."""
    DAY0, DAY1 = at_club(club_day(day), RH), at_club(club_day(day + 1), RH)
    w = f"created_at >= {DAY0} AND created_at < {DAY1}"
    sw = f"status <> 'cancelled' AND started_at >= {DAY0} AND started_at < {DAY1}"
    q = f"""SELECT concat_ws('|',
      (SELECT COALESCE(-SUM(amount_grosz), 0) FROM wallet_transactions WHERE kind = 'session_spend' AND amount_grosz < 0 AND {w}),
      (SELECT COALESCE(SUM(grosz), 0) FROM pack_spends WHERE {w}),
      (SELECT COALESCE(SUM(minutes), 0) FROM pack_spends WHERE {w}),
      (SELECT COALESCE(SUM(GREATEST(pack_minutes_used - COALESCE((SELECT SUM(minutes) FROM pack_spends p WHERE p.session_id = s.id), 0), 0)), 0)
         FROM sessions s WHERE {sw}),
      (SELECT COALESCE(SUM(price_pln * 100), 0) FROM user_packages WHERE method <> 'comp'),
      (SELECT COALESCE(SUM(minutes_total), 0) FROM user_packages WHERE method <> 'comp'),
      (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales s WHERE voided_at IS NULL AND method <> 'comp' AND {w}
         AND NOT EXISTS (SELECT 1 FROM kitchen_orders o WHERE o.sale_id = s.id)),
      (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM kitchen_orders WHERE status = 'done' AND COALESCE(pay_method, '') <> 'comp'
         AND status_at >= {DAY0} AND status_at < {DAY1}),
      (SELECT COALESCE(SUM(coin_minutes_used), 0) FROM sessions WHERE {sw}),
      (SELECT COALESCE(SUM(ROUND(coin_minutes_used * base_rate_pln * 100 / 60.0)), 0) FROM sessions WHERE {sw}))"""
    v = [float(x) for x in sql(q).split("|")]
    est = int(Decimal(int(v[3])) * Decimal(str(v[4])) / Decimal(str(v[5])) + Decimal("0.5")) if v[5] else 0
    return {"wallet": int(v[0]), "packs": int(v[1]), "pack_min": int(v[2]), "est_min": int(v[3]), "est": est,
            "goods": int(v[6]), "kitchen": int(v[7]), "given_min": int(v[8]), "given": int(v[9])}


def as_api(e):
    return {"wallet": g(e.get("wallet_pln")), "packs": g(e.get("packs_pln")), "pack_min": e.get("packs_minutes"),
            "est_min": e.get("estimate_minutes"), "est": g(e.get("estimate_pln")), "goods": g(e.get("goods_pln")),
            "kitchen": g(e.get("kitchen_pln")), "given_min": (e.get("given") or {}).get("minutes"), "given": g((e.get("given") or {}).get("pln"))}


def sum_ok(e):
    """Одна цифра — сумма частей; «отдано временем» в неё не входит."""
    return g(e.get("pln")) == sum(g(e.get(k)) for k in ("wallet_pln", "packs_pln", "estimate_pln", "goods_pln", "kitchen_pln"))


def same_as_sql(e, day=0):
    a, d = as_api(e), direct(day)
    bad = {k: (a[k], d[k]) for k in d if a[k] != d[k]}
    return not bad, bad


section("снимок: прямой SQL и сумма частей")
E0 = earned()
ok, bad = same_as_sql(E0)
check("«Отработанная выручка» за сегодня — как прямой SQL до гроша: кошелёк, пакеты по журналу, оценка до журнала, товары, "
      "кухня, «отдано временем»", ok and E0, J(bad))
check("одна цифра — сумма частей, «отдано временем» в неё не входит", sum_ok(E0) and E0, J({k: E0.get(k) for k in ("pln", "wallet_pln", "packs_pln", "goods_pln", "kitchen_pln")}))

section("сессия: зонные минуты за монеты → пакет → кошелёк")
pa = (api("POST", f"/admin/users/{G44}/packages", {"package_id": PACK, "method": "card"}, "boss")[1].get("package") or {}).get("id")
sql(f"UPDATE user_packages SET minutes_left = {LEFT} WHERE id = '{pa}'")
rd = api("POST", f"/admin/users/{G44}/redeem", {"zone_id": STD, "minutes": 30}, "boss")[1]
sid = seat("Guest44", "PC-07", minutes_ago=60)
end(sid)
S = dict(zip(["billed", "coin", "pack", "money", "charged", "base"],
             sql(f"SELECT concat_ws('|', billed_minutes, coin_minutes_used, pack_minutes_used, money_minutes, charged_grosz, base_rate_pln) FROM sessions WHERE id = '{sid}'").split("|")))
spends = rows(f"SELECT user_package_id || '|' || SUM(minutes) || '|' || SUM(grosz) FROM pack_spends WHERE session_id = '{sid}' GROUP BY user_package_id")
share = lambda m: (2 * round(PRICE * 100) * m + PMIN) // (2 * PMIN)  # цена отыгранных m минут — как сервер
want_g = share(PMIN) - share(PMIN - LEFT)
check(f"сессия час: 30 мин за монеты, {LEFT} мин пакета, остаток — кошелёк; расход пакета записан: {LEFT} мин, "
      f"{want_g / 100:.2f} zł — доля цены последних {LEFT} минут",
      pa and rd.get("id") and int(S["coin"]) == 30 and int(S["pack"]) == LEFT and int(S["money"]) == int(S["billed"]) - 30 - LEFT
      and spends == [f"{pa}|{LEFT}|{want_g}"], J([S, spends, want_g]))
E1 = earned()
d = lambda a, b, k: round((b.get(k) or 0) - (a.get(k) or 0), 2)
check(f"«Отработанная выручка» +{want_g / 100:.2f} (пакет) + {int(S['charged']) / 100:.2f} (кошелёк, списано за сессию); "
      f"«отдано временем» +30 мин и +{30 * float(S['base']) / 60:.2f} zł по тарифу зоны — в выручку не вошло",
      near(d(E0, E1, "packs_pln"), want_g / 100) and d(E0, E1, "packs_minutes") == LEFT and near(d(E0, E1, "wallet_pln"), int(S["charged"]) / 100)
      and near(d(E0, E1, "pln"), (want_g + int(S["charged"])) / 100) and d(E0, E1, "goods_pln") == 0 and d(E0, E1, "kitchen_pln") == 0
      and (E1.get("given") or {}).get("minutes", 0) - (E0.get("given") or {}).get("minutes", 0) == 30
      and near((E1.get("given") or {}).get("pln", 0) - (E0.get("given") or {}).get("pln", 0), 30 * float(S["base"]) / 60) and sum_ok(E1),
      J({"было": as_api(E0), "стало": as_api(E1), "сессия": S}))

section("товары и кухня")
sell = lambda method, **kw: api("POST", "/admin/sales", {"good_id": good, "qty": kw.pop("qty", 1), "method": method, **kw}, "boss")
s_card = sell("card", qty=2)
s_gift = sell("comp", comp_reason=f"{MARK}: проверка")
s_void = (sell("card")[1].get("sale") or {}).get("id")
vv = api("POST", f"/admin/sales/{s_void}/void", {"refund_method": "none"}, "boss")[0]
ko = (api("POST", "/admin/kitchen/orders", {"guest_id": G44, "good_id": good, "qty": 1, "method": "card"}, "boss")[1].get("order") or {}).get("id")
E2 = earned()
done_ = api("POST", f"/admin/kitchen/orders/{ko}/status", {"status": "done"}, "boss")[0]
E3 = earned()
check("продажа 2 × 7 картой — товары +14,00; подарок и отменённая — 0; заказ кухни, оплаченный картой, — 0, пока не выдан "
      "(его продажа — не «товары»), выдан — кухня +7,00",
      s_card[0] == 201 and s_gift[0] == 201 and vv == 200 and ko and done_ == 200
      and near(d(E1, E2, "goods_pln"), 14) and d(E1, E2, "kitchen_pln") == 0 and near(d(E2, E3, "kitchen_pln"), 7) and d(E2, E3, "goods_pln") == 0
      and near(d(E1, E3, "pln"), 21) and sum_ok(E3), J({"E1": as_api(E1), "E2": as_api(E2), "E3": as_api(E3)}))

section("до журнала — оценка (Д7)")
sql(f"DELETE FROM pack_spends WHERE session_id = '{sid}'")  # так выглядят минуты пакета до З12-и2: только итог на сессии
E4 = earned()
avg = Decimal(sql("SELECT SUM(price_pln * 100) / NULLIF(SUM(minutes_total), 0) FROM user_packages WHERE method <> 'comp'") or 0)
est = int(Decimal(E4.get("estimate_minutes") or 0) * avg + Decimal("0.5"))  # оценка — от всех минут без журнала за сутки, округление — на итоге
check(f"без строк расхода: пакеты −{want_g / 100:.2f} и −{LEFT} мин, без журнала +{LEFT} мин; оценка — все минуты без журнала "
      f"за сутки ({E4.get('estimate_minutes')}) × средняя цена минуты пакета ({float(avg) / 100:.4f} zł) = {est / 100:.2f}",
      near(d(E3, E4, "packs_pln"), -want_g / 100) and d(E3, E4, "packs_minutes") == -LEFT and d(E3, E4, "estimate_minutes") == LEFT
      and g(E4.get("estimate_pln")) == est and near(E4.get("pack_minute_pln", 0), round(float(avg) / 100, 4), 0.00005) and sum_ok(E4),
      J({"было": as_api(E3), "стало": as_api(E4), "avg": float(avg)}))
ok, bad = same_as_sql(E4)
check("после всего — снова как прямой SQL до гроша", ok, J(bad))
y = earned("yesterday")
ok, bad = same_as_sql(y, -1)
check("вчера — тоже как прямой SQL за вчерашние сутки: части — по моменту списания, продажи и выдачи, сессии — по началу",
      ok and y and sum_ok(y), J(bad))
done()
