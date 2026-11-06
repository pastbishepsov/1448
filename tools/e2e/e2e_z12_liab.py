#!/usr/bin/env python3
"""З12-и1 «Обязательства клуба и сгоревшее» (№49; З-Р9; решение основателя 03.10 — сгоревшие монеты тоже). GET
/admin/reports/money и GET /admin/cash/book (только владельцу) отдают liabilities — снимок «сколько клуб должен гостям
сейчас»: кошельки, пакеты (неотыгранный остаток по доле цены, подаренные — 0 zł), монеты на руках (по курсу погашения),
минуты за монеты и компенсации (по тарифу выдачи), минус дебиторка; и burned — «Сгорело за период»: просроченные
пакеты (остаток по доле цены, у подаренного — 0 zł) по моменту срока, монеты, сгоревшие у неактивных, по курсу
погашения, и (З18-и1а) минуты, сожжённые при удалении зоны, по тарифу выдачи — по моменту сгорания. Каждая часть сходится с прямым SQL до гроша и с «Кошельками гостей», «Долгом временем», «Монетами → на
руках», «должны временем» и «Дебиторкой сейчас»; оба отчёта отдают одно и то же. На приращениях пополнение, пакет,
подарок пакета, погашение монет временем, просроченные пакеты, сгоревшие монеты и долг двигают ровно свои строки.
Данные: гость Guest43 (монеты докладываются до 1000; за PC-09 — продажа в долг, сессия в конце завершается); позиция
«z12l Долг» (5 zł) заводится каждый прогон и в конце удаляется; самый дешёвый пакет каталога. Пополнение, пакеты,
погашение и продажа по ходу проверки или в уборке отменяются; сгорание монет — строкой coin_burns (SQL-ом, как её
пишет прогон таяния; метка — idle_days = 0 и pct = 0) и в конце удаляется вместе с возвратом монет."""
from _z import *

NAME, MARK = "z12l Долг", "z12l"
G43 = uid("Guest43")
PACK = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
PRICE = float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{PACK}'") or 0)
PMIN = int(sql(f"SELECT minutes FROM time_packages WHERE id = '{PACK}'") or 0)
STD = sql("SELECT id FROM zones WHERE name = 'Standard'")
BURN = 300  # монет сгорает в проверке


def gone():
    end_all("Guest43")
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE user_id = '{G43}' AND voided_at IS NULL"):
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for rid in rows(f"SELECT id FROM coin_redemptions WHERE user_id = '{G43}' AND voided_at IS NULL"):
        api("POST", f"/admin/coin-redemptions/{rid}/void", {}, "boss")
    for bid, n in (r.split("|") for r in rows(f"SELECT id, coins FROM coin_burns WHERE user_id = '{G43}' AND idle_days = 0 AND pct = 0")):
        sql(f"DELETE FROM coin_burns WHERE id = '{bid}'; UPDATE users SET coins_balance = coins_balance + {n} WHERE id = '{G43}'")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
at_end(gone)
sql(f"UPDATE users SET coins_balance = GREATEST(coins_balance, 1000) WHERE id = '{G43}'")
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z12", "price_pln": 5, "stock": 5}, "boss")[1].get("good") or {}).get("id")


def money(preset="today"):
    return api("GET", f"/admin/reports/money?preset={preset}", None, "boss")[1]


def snap():
    d = money()
    return d.get("liabilities") or {}, d.get("burned") or {}, d


g = lambda v: round((v or 0) * 100)  # злотые → гроши
part = lambda L, k, f="pln": (L.get(k) or {}).get(f, 0)
DAY0, DAY1 = (at_club(club_day(i), sql("SELECT lpad(COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '8'), 2, '0') || ':00'"))
              for i in (0, 1))


def direct():
    """Снимок и сгоревшее за сегодня прямым SQL, в грошах — независимо от сервера."""
    live = "voided_at IS NULL AND minutes_left > 0 AND (expires_at IS NULL OR expires_at > now())"
    burnt = f"voided_at IS NULL AND minutes_left > 0 AND expires_at >= {DAY0} AND expires_at < {DAY1} AND expires_at <= now()"
    rate = f"(SELECT COALESCE((SELECT value::numeric FROM settings WHERE key = 'coins_per_pln_spend'), 20))"
    q = f"""SELECT concat_ws('|',
      (SELECT COALESCE(SUM(wallet_grosz), 0) FROM users),
      (SELECT COALESCE(SUM(ROUND(price_pln * 100 * minutes_left / minutes_total)) FILTER (WHERE method <> 'comp'), 0) FROM user_packages WHERE {live}),
      (SELECT COALESCE(SUM(minutes_left), 0) FROM user_packages WHERE {live}),
      (SELECT COALESCE(SUM(coins_balance), 0) FROM users),
      (SELECT ROUND(COALESCE(SUM(coins_balance), 0) * 100 / {rate}) FROM users),
      (SELECT ROUND((COALESCE((SELECT SUM(minutes_left * rate_pln / 60.0) FROM minute_grants WHERE minutes_left > 0), 0)
        + COALESCE((SELECT SUM(coin_minutes) FROM users), 0) * (SELECT base_rate_pln FROM clubs ORDER BY created_at LIMIT 1) / 60.0) * 100)),
      (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales WHERE user_id IS NOT NULL AND paid_at IS NULL AND voided_at IS NULL AND write_off_id IS NULL)
        + (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM kitchen_orders WHERE status = 'done' AND paid_at IS NULL AND write_off_id IS NULL),
      (SELECT COALESCE(SUM(CASE WHEN method = 'comp' THEN 0 ELSE ROUND(price_pln * 100 * minutes_left / minutes_total) END), 0) FROM user_packages WHERE {burnt}),
      (SELECT count(*) FROM user_packages WHERE {burnt}),
      (SELECT COALESCE(SUM(coins), 0) FROM coin_burns WHERE created_at >= {DAY0} AND created_at < {DAY1}),
      (SELECT ROUND(COALESCE(SUM(coins), 0) * 100 / {rate}) FROM coin_burns WHERE created_at >= {DAY0} AND created_at < {DAY1}),
      (SELECT COALESCE(SUM(ROUND(burned_minutes * rate_pln * 100 / 60.0)), 0) FROM minute_grants
        WHERE burned_minutes > 0 AND burned_at >= {DAY0} AND burned_at < {DAY1}))"""
    k = ["wallets", "packs", "pack_min", "coins", "coins_g", "min_g", "rcv", "burn_packs", "burn_count", "burn_coins", "burn_coins_g",
         "burn_min_g"]
    return dict(zip(k, (int(float(x)) for x in sql(q).split("|"))))


def as_api(L, B):
    return {"wallets": g(part(L, "wallets")), "packs": g(part(L, "packages")), "pack_min": part(L, "packages", "minutes"),
            "coins": part(L, "coins", "coins"), "coins_g": g(part(L, "coins")), "min_g": g(part(L, "minutes")),
            "rcv": g(part(L, "receivable")), "burn_packs": g(part(B, "packages")), "burn_count": part(B, "packages", "count"),
            "burn_coins": part(B, "coins", "coins"), "burn_coins_g": g(part(B, "coins")), "burn_min_g": g(part(B, "minutes"))}


def sums_ok(L, B):
    """Одна цифра — сумма своих частей: клуб должен = кошельки + пакеты + монеты + минуты, обязательства = должен −
    дебиторка; сгорело = пакеты + минуты зоны (З18-и1а) + монеты."""
    owed = sum(g(part(L, k)) for k in ("wallets", "packages", "coins", "minutes"))
    return g(L.get("owed_pln")) == owed and g(L.get("net_pln")) == owed - g(part(L, "receivable")) \
        and g(B.get("pln")) == g(part(B, "packages")) + g(part(B, "minutes")) + g(part(B, "coins"))


def same_as_sql(L, B):
    a, d = as_api(L, B), direct()
    bad = {k: (a[k], d[k]) for k in d if abs(a[k] - d[k]) > (1 if k in ("coins_g", "min_g", "burn_coins_g") else 0)}
    return not bad, bad


section("снимок: прямой SQL, свои части, оба отчёта")
L0, B0, M0 = snap()
ok, bad = same_as_sql(L0, B0)
check("«Обязательства клуба» и «Сгорело» за сегодня — как прямой SQL до гроша: кошельки, пакеты (минуты тоже), монеты, минуты, "
      "дебиторка, сгоревшие пакеты, минуты зоны и монеты", ok and L0 and B0, J(bad))
check("одна цифра — сумма частей: клуб должен = кошельки + пакеты + монеты + минуты, обязательства = должен − дебиторка; "
      "сгорело = пакеты + минуты зоны + монеты", sums_ok(L0, B0), J({k: L0.get(k) for k in ("owed_pln", "net_pln")}))
book = api("GET", "/admin/cash/book?preset=today", None, "boss")[1]
check("«Бухгалтерия → Касса» отдаёт то же: liabilities и burned за тот же период",
      book.get("liabilities") == L0 and (book.get("burned") or {}).get("pln") == B0.get("pln")
      and (book.get("burned") or {}).get("items") == B0.get("items"), J([book.get("liabilities", {}).get("net_pln"), L0.get("net_pln")]))
coins_rep = api("GET", "/admin/reports/coins?preset=today", None, "boss")[1]
check("те же цифры, что уже на экранах: «Кошельки гостей», «Долг временем», «Дебиторка сейчас» в «Деньгах», «на руках» и "
      "«должны временем» в «Монетах»",
      near(part(L0, "wallets"), (M0.get("wallet") or {}).get("liability_pln", -1)) and near(part(L0, "packages"), (M0.get("packages") or {}).get("liability_pln", -1))
      and near(part(L0, "receivable"), ((M0.get("debts") or {}).get("receivable") or {}).get("total_pln", -1))
      and near(part(L0, "coins"), (coins_rep.get("on_hand") or {}).get("value_pln", -1)) and part(L0, "coins", "coins") == (coins_rep.get("on_hand") or {}).get("coins")
      and near(part(L0, "minutes"), (coins_rep.get("minutes_owed") or {}).get("value_pln", -1)),
      J([L0.get("wallets"), (M0.get("wallet") or {}).get("liability_pln"), L0.get("coins"), coins_rep.get("on_hand")]))
adm = [api("GET", u, None, "admin1")[0] for u in ("/admin/reports/money?preset=today", "/admin/cash/book?preset=today")]
check("админу — 403: обязательства видит только владелец", adm == [403, 403], J(adm))


def step(title, act, want):
    """act() → что сделали; want(L0, B0, L1, B1, r) — ожидание на приращениях; плюс одна цифра = сумма частей."""
    La, Ba, _ = snap()
    r = act()
    Lb, Bb, _ = snap()
    check(title, want(La, Ba, Lb, Bb, r) and sums_ok(Lb, Bb), J({"было": as_api(La, Ba), "стало": as_api(Lb, Bb), "r": r}))
    return r


d = lambda La, Lb, k, f="pln": round(part(Lb, k, f) - part(La, k, f), 2)

section("приращения: деньги, пакеты, монеты, минуты")
dep = step("пополнение 20 zł картой: кошельки +20,00, монеты — на начисленное по курсу погашения, остальное не тронуто",
           lambda: (lambda i: (i, int(sql(f"SELECT coins_granted + bonus_coins FROM deposits WHERE id = '{i}'") or 0)))(topup("Guest43", 20)),
           lambda La, Ba, Lb, Bb, r: d(La, Lb, "wallets") == 20 and d(La, Lb, "coins", "coins") == r[1] and r[1] > 0
           and near(d(La, Lb, "coins"), r[1] / part(Lb, "coins", "rate"), 0.011)
           and d(La, Lb, "packages") == 0 and d(La, Lb, "minutes") == 0 and d(La, Lb, "receivable") == 0)
at_end(lambda: api("POST", f"/admin/deposits/{dep[0]}/void", {}, "boss"))
pa = step(f"пакет за {PRICE:.2f} картой: пакеты +{PRICE:.2f} zł и +{PMIN} мин, кошельки и монеты не тронуты",
          lambda: (api("POST", f"/admin/users/{G43}/packages", {"package_id": PACK, "method": "card"}, "boss")[1].get("package") or {}).get("id"),
          lambda La, Ba, Lb, Bb, r: r and near(d(La, Lb, "packages"), PRICE) and d(La, Lb, "packages", "minutes") == PMIN
          and d(La, Lb, "wallets") == 0 and d(La, Lb, "coins") == 0)
pb = step(f"подарок пакета: пакеты +{PMIN} мин и +0,00 zł — денег за него не было (как в «Долге временем»)",
          lambda: (api("POST", f"/admin/users/{G43}/packages", {"package_id": PACK, "method": "comp", "comp_reason": f"{MARK}: проверка обязательств"}, "boss")[1]
                   .get("package") or {}).get("id"),
          lambda La, Ba, Lb, Bb, r: r and d(La, Lb, "packages") == 0 and d(La, Lb, "packages", "minutes") == PMIN)
rd = step("время за монеты, 60 мин в Standard: монеты −400 (−20,00 zł), минуты +20,00 zł — клуб должен столько же",
          lambda: api("POST", f"/admin/users/{G43}/redeem", {"zone_id": STD, "minutes": 60}, "boss")[1],
          lambda La, Ba, Lb, Bb, r: r.get("coins") and d(La, Lb, "coins", "coins") == -r["coins"] and near(d(La, Lb, "coins"), -r["coins"] / part(Lb, "coins", "rate"))
          and near(d(La, Lb, "minutes"), r.get("value_pln", 0)) and d(La, Lb, "minutes", "minutes") == 60
          and near(Lb.get("owed_pln", 0) - La.get("owed_pln", 0), d(La, Lb, "coins") + d(La, Lb, "minutes")))
at_end(lambda: api("POST", f"/admin/coin-redemptions/{rd.get('id')}/void", {}, "boss"))

section("сгорело: просроченные пакеты и монеты у неактивных")
pc = (api("POST", f"/admin/users/{G43}/packages", {"package_id": PACK, "method": "card"}, "boss")[1].get("package") or {}).get("id")


def expire():
    sql(f"UPDATE user_packages SET expires_at = now() - interval '1 second' WHERE id IN ('{pc}', '{pb}')")
    return [pc, pb]


step(f"срок вышел у пакета за {PRICE:.2f} и у подаренного: пакеты −{PRICE:.2f} zł и −{2 * PMIN} мин, «Сгорело» +{PRICE:.2f} zł "
     f"(пакеты: +2, подаренный — 0 zł, его {PMIN} мин — «подаренные»)", expire,
     lambda La, Ba, Lb, Bb, r: near(d(La, Lb, "packages"), -PRICE) and d(La, Lb, "packages", "minutes") == -2 * PMIN
     and near(d(Ba, Bb, "packages"), PRICE) and d(Ba, Bb, "packages", "count") == 2 and d(Ba, Bb, "packages", "minutes") == 2 * PMIN
     and d(Ba, Bb, "packages", "gift_minutes") == PMIN and near(Bb.get("pln", 0) - Ba.get("pln", 0), PRICE))


def burn():
    left = int(sql(f"SELECT coins_balance FROM users WHERE id = '{G43}'"))
    sql(f"""UPDATE users SET coins_balance = coins_balance - {BURN} WHERE id = '{G43}';
        INSERT INTO coin_burns (user_id, coins, balance_after, idle_days, pct, manual) VALUES ('{G43}', {BURN}, {left - BURN}, 0, 0, true)""")
    return BURN


step(f"сгорело {BURN} монет у Guest43: монеты на руках −{BURN} (−{BURN / 20:.2f} zł при 20 = 1 zł), «Сгорело» → монеты +{BURN}",
     burn, lambda La, Ba, Lb, Bb, r: d(La, Lb, "coins", "coins") == -BURN and near(d(La, Lb, "coins"), -BURN / part(Lb, "coins", "rate"))
     and d(Ba, Bb, "coins", "coins") == BURN and d(Ba, Bb, "coins", "count") == 1 and near(d(Ba, Bb, "coins"), BURN / part(Bb, "coins", "rate")))
L1, B1, _ = snap()
items = {x.get("id"): x for x in B1.get("items") or []}
xb = next((x for x in B1.get("items") or [] if x.get("kind") == "coins" and x.get("guest") == "Guest43" and x.get("qty") == BURN), {})
check("в списке «Сгорело» — оба пакета (название, гость, минуты, по доле цены; подаренный — 0 zł и «подарок») и монеты Guest43, "
      "свежие сверху",
      (items.get(pc) or {}).get("guest") == "Guest43" and (items.get(pc) or {}).get("qty") == PMIN and near((items.get(pc) or {}).get("pln", 0), PRICE)
      and (items.get(pc) or {}).get("gift") is False and (items.get(pb) or {}).get("gift") is True and (items.get(pb) or {}).get("pln") == 0
      and xb and near(xb.get("pln", 0), BURN / part(B1, "coins", "rate"))
      and [x.get("at") for x in B1.get("items") or []] == sorted([x.get("at") for x in B1.get("items") or []], reverse=True),
      J([items.get(pc), items.get(pb), xb]))
ok, bad = same_as_sql(L1, B1)
check("после всего — снова как прямой SQL до гроша", ok, J(bad))
y = money("yesterday").get("burned") or {}
check("за вчера сегодняшнего сгоревшего нет: «Сгорело» — по моменту срока и сгорания", not ({pc, pb, xb.get("id")} & {x.get("id") for x in y.get("items") or []}),
      J(y.get("pln")))

section("дебиторка вычитается")
sid = seat("Guest43", "PC-09")
step("продажа в долг 5 zł гостю за PC-09: дебиторка +5,00, обязательства −5,00 (клуб должен — как был)",
     lambda: api("POST", "/admin/sales", {"good_id": good, "qty": 1, "method": "later", "guest_id": G43}, "boss")[0],
     lambda La, Ba, Lb, Bb, r: r == 201 and d(La, Lb, "receivable") == 5 and near(Lb.get("net_pln", 0) - La.get("net_pln", 0), -5 + (Lb.get("owed_pln", 0) - La.get("owed_pln", 0))))
done()
