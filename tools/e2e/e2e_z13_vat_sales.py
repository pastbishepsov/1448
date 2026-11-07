#!/usr/bin/env python3
"""З13-и3 «Продажи по ставкам VAT» (№72 «а»; решения основателя 04.10, З-Р37). GET /admin/reports/vat — облагаемое за
период по настройкам «Бухгалтерии», по ставкам и по дням: брутто, нетто, VAT. Ручной расчёт по шагам в обоих режимах:
«при трате» (по умолчанию) — время с кошелька по ставке зоны, пакеты при выдаче, товары и кухня при оплате (и с
кошелька), пополнения без VAT; «при пополнении» — пополнения по ставке пополнения, пакеты, товары и кухня за живые
деньги, с кошелька — без VAT. Долг — не продажа, пока его не рассчитали; возвраты — минусом в день отмены по ставке, с
которой брали VAT («при пополнении» оплаченное с кошелька и возвращённое деньгами — по ставке пополнения), «не
возвращали» — ничего; подарки — по себестоимости, если так настроено; освобождённый от VAT — всё «zw». VAT дня по
ставке — из брутто дня, за период — сумма дней; «при пополнении» брутто = «Доходы» − «Возвраты» «Прибыли»; «при трате» —
как прямой SQL по тем же правилам. Только владельцу.
Данные: гость Guest45 — пополнение 100 zł картой (отменяется по ходу с возвратом на карту); сессия за PC-09 на полчаса
назад (кошелёк) и продажа в долг, рассчитанная картой; пакеты — с кошелька (отменяется с возвратом наличными) и картой
(отменяется «не возвращали»); заказы кухни от стойки — с кошелька (отменяется с возвратом на кошелёк) и картой (с
возвратом на карту); подарок; позиции «z13s Напиток» (23 %) и «z13s Кухня» (8 %) заводятся каждый прогон и в конце
удаляются `drop_good`; самому дешёвому пакету на время проверки — ставка 5 %; настройки «Бухгалтерии» возвращаются."""
from _z import *

MARK = "z13s"
A_NAME, K_NAME = f"{MARK} Напиток", f"{MARK} Кухня"
G = uid("Guest45")
PC = "PC-09"
ZVAT = int(sql(f"SELECT COALESCE(z.vat_pct, 23) FROM computers c LEFT JOIN zones z ON z.id = c.zone_id WHERE c.name = '{PC}'") or 23)
PACK = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
PVAT0 = int(sql(f"SELECT vat_pct FROM time_packages WHERE id = '{PACK}'") or 23)
PRICE = round(float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{PACK}'") or 0) * 100)
KEYS = ("acc_vat_moment", "acc_vat_payer", "acc_gift_vat", "acc_topup_vat")
ORIG = {k: (rows(f"SELECT value FROM settings WHERE key = '{k}'") or [None])[0] for k in KEYS}
made = {"deps": []}


def put(key, value):
    """Настройка «Бухгалтерии» прямо в базе: отчёт читает её на каждом запросе, а журналу действий шум не нужен."""
    sql(f"INSERT INTO settings (key, value) VALUES ('{key}', '{value}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value")


def back():
    for k, v in ORIG.items():
        if v is None:
            sql(f"DELETE FROM settings WHERE key = '{k}'")
        else:
            put(k, v)


def gone():
    end_all("Guest45")
    for sid in rows(f"SELECT id FROM sales WHERE name LIKE '{MARK} %' AND voided_at IS NULL AND write_off_id IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for oid in rows(f"SELECT id FROM kitchen_orders WHERE name LIKE '{MARK} %' AND status IN ('new','accepted','preparing','delivering')"):
        api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE user_id = '{G}' AND voided_at IS NULL"):  # и хвосты упавшего прогона
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for rid in rows(f"SELECT id FROM coin_redemptions WHERE user_id = '{G}' AND voided_at IS NULL"):
        api("POST", f"/admin/coin-redemptions/{rid}/void", {}, "boss")
    for did in made["deps"]:
        api("POST", f"/admin/deposits/{did}/void", {"refund_method": "none"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{MARK} %'"):
        drop_good(gid)
    sql(f"UPDATE time_packages SET vat_pct = {PVAT0} WHERE id = '{PACK}'")
    back()


gone()
at_end(gone)
sql(f"UPDATE users SET coin_minutes = 0 WHERE id = '{G}'")  # сессия идёт с кошелька: ни монетных минут, ни пакетов
for k in KEYS:  # с известного начала: плательщик VAT, при трате, подарки без VAT, ставка пополнения 23 %
    sql(f"DELETE FROM settings WHERE key = '{k}'")
api("PATCH", f"/admin/packages/{PACK}", {"vat_pct": 5}, "boss")
new_good = lambda name, price, vat: (api("POST", "/admin/goods", {"name": name, "category": MARK, "price_pln": price, "stock": 20, "vat_pct": vat}, "boss")[1].get("good") or {}).get("id")
A, K = new_good(A_NAME, 6, 23), new_good(K_NAME, 10, 8)
sql(f"UPDATE stock_lots SET price_grosz = 250 WHERE good_id = '{A}'")  # себестоимость напитка — 2,50 zł за штуку
RH = sql("SELECT lpad(COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '0'), 2, '0') || ':00'")
D0, D1 = club_day(-1), club_day(0)  # вчера и сегодня: шаг у границы клубных суток не уходит за период
Q = f"from={D0}&to={D1}"
g = lambda v: round((v or 0) * 100)


def snap(moment=0, payer=1, gifts=0):
    """Отчёт за вчера и сегодня в режиме moment (0 — при трате, 1 — при пополнении): брутто по ставкам и видам, в грошах."""
    put("acc_vat_moment", moment); put("acc_vat_payer", payer); put("acc_gift_vat", gifts)
    s, r = api("GET", f"/admin/reports/vat?{Q}", None, "boss")
    if s != 200:  # до З13-и3 отчёта нет — шаги падают, но прогон доходит до конца
        return {"ok": False, "rates": {}, "kinds": {}, "raw": {}}
    return {"ok": True, "rates": {x["vat_pct"]: g(x["gross_pln"]) for x in r.get("rates") or []},
            "kinds": {k: g(v.get("pln")) for k, v in (r.get("kinds") or {}).items()}, "raw": r}


def both(**kw):
    return snap(0, **kw), snap(1, **kw)


def delta(a, b):
    """Что изменилось между снимками: ставки и виды с ненулевой разницей."""
    d = lambda x, y: {k: y.get(k, 0) - x.get(k, 0) for k in set(x) | set(y) if y.get(k, 0) != x.get(k, 0)}
    return {"rates": d(a["rates"], b["rates"]), "kinds": d(a["kinds"], b["kinds"])}


def step(name, before, after, want_m, want_s):
    """Шаг ручного расчёта: изменение отчёта в обоих режимах — ровно ожидаемое (ставка → гроши, вид → гроши)."""
    dm, ds = delta(before[0], after[0]), delta(before[1], after[1])
    ok = all(x["ok"] for x in (*before, *after))
    check(name, ok and dm == want_m and ds == want_s, J({"при трате": dm, "ждали": want_m, "при пополнении": ds, "ждали_": want_s, "отчёт": ok}, 900))


def formula_ok(r):
    """VAT дня по ставке — из брутто дня (до гроша, половина — от нуля), за период — сумма дней; нетто = брутто − VAT;
    итог — сумма ставок."""
    def vat(gross, rate):
        if rate <= 0:
            return 0
        n, d = 2 * gross * rate, 100 + rate
        return (n + d) // (2 * d) if n >= 0 else -((-n + d) // (2 * d))
    bad = []
    for x in r.get("rates") or []:
        days = [(g(c["gross_pln"]), g(c["vat_pln"])) for day in r.get("days") or [] for c in day.get("rates") or [] if c["vat_pct"] == x["vat_pct"]]
        want_vat = sum(vat(dg, x["vat_pct"]) for dg, _ in days)
        if g(x["vat_pln"]) != want_vat or sum(dg for dg, _ in days) != g(x["gross_pln"]) or any(dv != vat(dg, x["vat_pct"]) for dg, dv in days) \
                or g(x["net_pln"]) != g(x["gross_pln"]) - g(x["vat_pln"]):
            bad.append(x)
    t = r.get("total") or {}
    for f in ("gross_pln", "vat_pln", "net_pln"):
        if g(t.get(f)) != sum(g(x[f]) for x in r.get("rates") or []):
            bad.append({"итог": f})
    return not bad, bad


section("права и период")
sa = api("GET", f"/admin/reports/vat?{Q}", None, "admin1")[0]
sb = api("GET", "/admin/reports/vat?month=2026-1", None, "boss")
sm = api("GET", "/admin/reports/vat?preset=month", None, "boss")
check("отчёт — только владельцу (админу 403); месяц не «ГГГГ-ММ» — 400; «этот месяц» — по дням без дыр, у каждого дня — "
      "брутто, нетто и VAT по ставкам", sa == 403 and sb[0] == 400 and sm[0] == 200 and len(sm[1].get("days") or []) == (sm[1].get("period") or {}).get("days")
      and all({"date", "gross_pln", "net_pln", "vat_pln", "rates"} <= set(d) for d in sm[1].get("days") or []), J([sa, sb, (sm[1].get("period"))]))
ok, bad = formula_ok(sm[1])
check("«этот месяц»: VAT ставки — сумма VAT дней, VAT дня — из брутто дня (брутто × r / (100 + r), до гроша); нетто = "
      "брутто − VAT; итог — сумма ставок", ok and sm[1].get("rates") is not None, J(bad, 600))
r1 = set_settings(acc_gift_vat=2)[0]
r2 = set_settings(acc_gift_vat=1)[0]
st = settings()
check("настройка «Подарки в VAT»: 1 — да, 2 — 400; по умолчанию — нет (0)", r1 == 400 and r2 == 200 and st.get("acc_gift_vat") == 1, J([r1, r2, st.get("acc_gift_vat")]))
sql("DELETE FROM settings WHERE key = 'acc_gift_vat'")
check("без строки в настройках — 0: подарки без VAT", settings().get("acc_gift_vat") == 0, J(settings().get("acc_gift_vat")))

section("шаг за шагом: пополнение, время, долг, пакеты, кухня, подарок")
B = both()
dep = topup("Guest45", 100, "card")
made["deps"].append(dep)
A1 = both()
step("пополнение 100 zł картой: «при трате» — ничего, «при пополнении» — пополнения +100,00 по ставке пополнения 23 %",
     B, A1, {"rates": {}, "kinds": {}}, {"rates": {23: 10000}, "kinds": {"deposits": 10000}})
sid = seat("Guest45", PC, minutes_ago=30)
s_debt, d_debt = api("POST", "/admin/sales", {"good_id": A, "qty": 1, "method": "later", "guest_id": G}, "boss")
end(sid)
charged = int(sql(f"SELECT charged_grosz FROM sessions WHERE id = '{sid}'") or 0)
spent = int(sql(f"SELECT COALESCE(-SUM(amount_grosz), 0) FROM wallet_transactions WHERE ref_id = '{sid}' AND kind = 'session_spend'") or 0)
A2 = both()
check(f"сессия за {PC} полчаса с кошелька — списано {spent / 100:.2f} zł (как в сессии); продажа «{A_NAME}» в долг — принята",
      charged > 0 and spent == charged and s_debt == 201, J([charged, spent, s_debt, d_debt]))
step(f"время с кошелька: «при трате» — время +{spent / 100:.2f} по ставке зоны {ZVAT} %; долг — не продажа, пока не рассчитан; "
     "«при пополнении» — ничего", A1, A2, {"rates": {ZVAT: spent}, "kinds": {"time": spent}}, {"rates": {}, "kinds": {}})
st_ = api("POST", f"/admin/users/{G}/settle", {"method": "card"}, "boss")[0]
A3 = both()
step("долг рассчитали картой — в обоих режимах товары +6,00 по ставке позиции 23 % в момент оплаты", A2, A3,
     {"rates": {23: 600}, "kinds": {"sales": 600}}, {"rates": {23: 600}, "kinds": {"sales": 600}})
pw = (api("POST", f"/admin/users/{G}/packages", {"package_id": PACK, "method": "wallet"}, "boss")[1].get("package") or {}).get("id")
pc = (api("POST", f"/admin/users/{G}/packages", {"package_id": PACK, "method": "card"}, "boss")[1].get("package") or {}).get("id")
A4 = both()
step(f"пакеты по {PRICE / 100:.2f} zł со ставкой 5 %: с кошелька и картой — «при трате» оба при выдаче (+{2 * PRICE / 100:.2f}), "
     "«при пополнении» — только картой", A3, A4, {"rates": {5: 2 * PRICE}, "kinds": {"packs": 2 * PRICE}},
     {"rates": {5: PRICE}, "kinds": {"packs": PRICE}})
kw = api("POST", "/admin/kitchen/orders", {"guest_id": G, "good_id": K, "qty": 1, "method": "wallet"}, "admin1")[1]
kc = api("POST", "/admin/kitchen/orders", {"guest_id": G, "good_id": K, "qty": 1, "method": "card"}, "admin1")[1]
A5 = both()
step("кухня от стойки 10 zł со ставкой 8 %: с кошелька и картой — «при трате» обе (+20,00), «при пополнении» — только картой",
     A4, A5, {"rates": {8: 2000}, "kinds": {"sales": 2000}}, {"rates": {8: 1000}, "kinds": {"sales": 1000}})
gift = api("POST", "/admin/sales", {"good_id": A, "qty": 1, "method": "comp", "comp_reason": f"{MARK}: проверка VAT"}, "boss")[0]
A6 = both()
step("подарок «Напитка» при «Подарки без VAT» — ничего в обоих режимах", A5, A6, {"rates": {}, "kinds": {}}, {"rates": {}, "kinds": {}})
G6 = both(gifts=1)
step("«Подарки в VAT: да» — подарок по себестоимости 2,50 по ставке позиции 23 % в обоих режимах", A6, G6,
     {"rates": {23: 250}, "kinds": {"gifts": 250}}, {"rates": {23: 250}, "kinds": {"gifts": 250}})

section("возвраты — минусом в день отмены")
v1 = api("POST", f"/admin/sales/{kc.get('sale_id')}/void", {"refund_method": "card"}, "boss")[0]
R1 = both()
step("кухню картой вернули на карту — минус 10,00 по 8 % в обоих режимах", A6, R1,
     {"rates": {8: -1000}, "kinds": {"returns": -1000}}, {"rates": {8: -1000}, "kinds": {"returns": -1000}})
v2 = api("POST", f"/admin/sales/{kw.get('sale_id')}/void", {"refund_method": "wallet"}, "boss")[0]
R2 = both()
step("кухню с кошелька вернули на кошелёк — «при трате» минус 10,00 по 8 %, «при пополнении» — ничего", R1, R2,
     {"rates": {8: -1000}, "kinds": {"returns": -1000}}, {"rates": {}, "kinds": {}})
v3 = api("POST", f"/admin/user-packages/{pw}/void", {"refund_method": "cash"}, "boss")[0]
back_pw = int(sql(f"SELECT refund_grosz FROM user_packages WHERE id = '{pw}'") or 0)
R3 = both()
step(f"пакет с кошелька владелец вернул наличными ({back_pw / 100:.2f}) — «при трате» минус по ставке пакета 5 %, «при "
     "пополнении» — по ставке пополнения 23 %: VAT за него брали с пополнения", R2, R3,
     {"rates": {5: -back_pw}, "kinds": {"returns": -back_pw}}, {"rates": {23: -back_pw}, "kinds": {"returns": -back_pw}})
v4 = api("POST", f"/admin/user-packages/{pc}/void", {"refund_method": "none"}, "boss")[0]
R4 = both()
step("пакет картой отменили «не возвращали» — ничего в обоих режимах: деньги остались у клуба", R3, R4,
     {"rates": {}, "kinds": {}}, {"rates": {}, "kinds": {}})
v5 = api("POST", f"/admin/deposits/{dep}/void", {"refund_method": "card"}, "boss")[0]
back_dep = int(sql(f"SELECT refund_grosz FROM deposits WHERE id = '{dep}'") or 0)
R5 = both()
step(f"пополнение отменили с возвратом на карту ({back_dep / 100:.2f} — что снято с кошелька) — «при трате» ничего, «при "
     "пополнении» минус по его ставке 23 %", R4, R5, {"rates": {}, "kinds": {}}, {"rates": {23: -back_dep}, "kinds": {"returns": -back_dep}})
check("отмены прошли: кухня на карту и на кошелёк, пакет наличными и «не возвращали», пополнение на карту",
      [v1, v2, v3, v4, v5] == [200] * 5 and back_pw == PRICE and back_dep > 0, J([v1, v2, v3, v4, v5, back_pw, back_dep]))

section("сверки: «Прибыль», прямой SQL, «zw», формула")
m, s_ = R5[0]["raw"], R5[1]["raw"]
p = api("GET", f"/admin/reports/profit?{Q}", None, "boss")[1]
check("«при пополнении» брутто = «Доходы» − «Возвраты» «Прибыли» за тот же период — до гроша",
      g((s_.get("total") or {}).get("gross_pln")) == g((p.get("income") or {}).get("pln")) - g((p.get("returns") or {}).get("pln")),
      J([s_.get("total"), (p.get("income") or {}).get("pln"), (p.get("returns") or {}).get("pln")]))
W0, W1 = at_club(D0, RH), at_club(club_day(1), RH)
w = lambda col: f"{col} >= {W0} AND {col} < {W1}"
direct = dict(zip(("time", "packs", "sales", "returns"), map(int, sql(f"""SELECT concat_ws('|',
  (SELECT COALESCE(-SUM(amount_grosz), 0) FROM wallet_transactions WHERE kind = 'session_spend' AND amount_grosz < 0 AND {w('created_at')}),
  (SELECT COALESCE(SUM(ROUND(price_pln * 100)), 0) FROM user_packages WHERE method <> 'comp' AND {w('created_at')}),
  (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales WHERE method <> 'comp' AND {w('paid_at')}),
  -(SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales WHERE paid_at IS NOT NULL AND method <> 'comp' AND {w('voided_at')}
       AND COALESCE(refund_method, method) IN ('cash', 'card', 'blik', 'wallet'))
   - (SELECT COALESCE(SUM(refund_grosz), 0) FROM user_packages WHERE method <> 'comp' AND {w('voided_at')}
       AND COALESCE(refund_method, method) IN ('cash', 'card', 'blik', 'wallet')))""").split("|"))))
got = {k: g(((m.get("kinds") or {}).get(k) or {}).get("pln")) for k in direct}
check("«при трате» по видам — как прямой SQL до гроша: время с кошелька, пакеты при выдаче, товары и кухня при оплате, "
      "возвраты живыми и на кошелёк; пополнений нет", got == direct and g(((m.get("kinds") or {}).get("deposits") or {}).get("pln")) == 0,
      J([got, direct]))
for raw, name in ((m, "при трате"), (s_, "при пополнении")):
    ok, bad = formula_ok(raw)
    check(f"«{name}»: VAT по ставкам — из брутто дней, нетто = брутто − VAT, итог — сумма ставок", ok and raw.get("rates") is not None, J(bad, 600))
Z = snap(0, payer=0, gifts=1)["raw"]
zr = Z.get("rates") or []
check("освобождённый от VAT — одна ставка «zw»: брутто как «при трате» без подарков, VAT 0",
      len(zr) == 1 and zr[0].get("vat_pct") == -1 and zr[0].get("label") == "zw" and g(zr[0].get("gross_pln")) == g((m.get("total") or {}).get("gross_pln"))
      and g((Z.get("total") or {}).get("vat_pln")) == 0 and g(((Z.get("kinds") or {}).get("gifts") or {}).get("pln")) == 0
      and (Z.get("settings") or {}).get("payer") is False, J([zr, Z.get("total"), Z.get("settings")]))
check("как считали — в ответе: при трате / при пополнении, плательщик, подарки, ставка пополнения",
      (m.get("settings") or {}) == {"payer": True, "moment": "spend", "gifts": False, "topup_vat": 23}
      and (s_.get("settings") or {}).get("moment") == "topup", J([m.get("settings"), s_.get("settings")]))
done()
