#!/usr/bin/env python3
"""З11-и3 «Возвраты за прошлый месяц» (№17; З-Р12; решение основателя 30.09). GET /admin/reports/returns — только владельцу:
отмены с деньгами по дню отмены (возврат — операция своего дня), как бы давно ни платили; без month — прошлый месяц по
клубным суткам, month=ГГГГ-ММ — любой не позже текущего. Итоги — живыми деньгами (наличными, на карту, по BLIK), на кошелёк,
«не возвращали», «по операциям прошлых месяцев»; сумма — как у ящика: депозит — снятое с кошелька, продажа — цена, пакет —
доля за неотыгранное. Неоплаченный долг и отменённый подарок (З11-и5) — не возврат. Итоги сходятся с прямым SQL до гроша. Кассовая книга: у дня — список
возвратов наличными (кто, кому, за что, когда платили), сумма списка — «возвраты» дня.
Данные: временный админ z11rt (на смене; ящик заводится, если его нет); гость Guest41 (кошелёк докладывается до цены пакета
+ 10 zł); гость Guest42 (кошелёк докладывается до 10 zł; за PC-08 — продажа в долг, отменяется до расчёта; сессия
завершается); позиция «z11r Возврат» (12 zł) заводится каждый прогон и в конце удаляется вместе с поставкой; самый дешёвый
пакет каталога. Продажа A картой сдвигается SQL-ом на 15-е прошлого месяца и отменяется владельцем наличными (−12 в ящике),
продажа B налом отменяется «не возвращали» (+12 остаются) — ящик в конце как в начале; продажа G картой отменяется картой и
сдвигается целиком в прошлый месяц (оплачена 10-го, отменена в последнюю минуту месяца — в списке она свежая, сколько бы
отмен в месяце ни было) — строка блока по умолчанию. Всё, что проверка продаёт,
пополняет и выдаёт, по ходу отменяется."""
from urllib.parse import quote

from _z import *

NAME = "z11r Возврат"
G41 = uid("Guest41")


def gone():
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
hire_temp("z11rt")
tok("z11rt")  # вход в админку открывает смену
drawer_ready("z11rt")
at_end(gone)
deps, packs = [], []
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in deps if sql(f"SELECT voided_at IS NULL FROM deposits WHERE id = '{d}'") == "t"])
at_end(lambda: [api("POST", f"/admin/user-packages/{p}/void", {}, "boss") for p in packs if sql(f"SELECT voided_at IS NULL FROM user_packages WHERE id = '{p}'") == "t"])
pack = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
price = float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{pack}'") or 0)
ensure_wallet("Guest41", price + 10)
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z11", "price_pln": 12, "stock": 10}, "boss")[1].get("good") or {}).get("id")
exp = lambda: api("GET", "/admin/work/closeout", None, "z11rt")[1].get("expected_pln", 0)
E0 = exp()
month = club_day()[:7]  # клубный месяц сейчас
prev = sql(f"SELECT to_char('{month}-01'::date - interval '1 month', 'YYYY-MM')")
nxt = sql(f"SELECT to_char('{month}-01'::date + interval '1 month', 'YYYY-MM')")
RH = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')"))


def sell(method, **kw):
    return (api("POST", "/admin/sales", {"good_id": good, "qty": 1, "method": method, **kw}, "z11rt")[1].get("sale") or {}).get("id")


def void(sid, who="z11rt", how=None):
    s, d = api("POST", f"/admin/sales/{sid}/void", {} if how is None else {"refund_method": how}, who)
    return s, d.get("code") or d.get("refund_method")


section("отмены этого месяца")
sa = sell("card", guest_id=G41)
sql(f"UPDATE sales SET created_at = {at_club(prev + '-15', '12:00')}, paid_at = {at_club(prev + '-15', '12:00')} WHERE id = '{sa}'")
sb = sell("cash")
dep = topup("Guest41", 7, method="blik", who="z11rt")
deps.append(dep)
up = (api("POST", f"/admin/users/{G41}/packages", {"package_id": pack, "method": "wallet"}, "z11rt")[1].get("package") or {}).get("id")
packs.append(up)
done_ = [void(sa, "z11rt"), void(sa, "boss", "cash"), void(sb, "z11rt", "none"),
         (lambda r: (r[0], r[1].get("refund_method")))(api("POST", f"/admin/deposits/{dep}/void", {}, "z11rt")),
         (lambda r: (r[0], r[1].get("refund_method")))(api("POST", f"/admin/user-packages/{up}/void", {}, "z11rt"))]
check("A — картой, оплата сдвинута на 15-е прошлого месяца: админу отмена не своей смены — 403, владелец вернул наличными; B налом — "
      "«не возвращали»; депозит 7 zł по BLIK и пакет с кошелька — как платили",
      done_ == [(403, "too_old"), (200, "cash"), (200, "none"), (200, "blik"), (200, "wallet")], J(done_))
end_all("Guest42")
at_end(lambda: end_all("Guest42"))
ensure_wallet("Guest42", 10)  # посадке нужен запас на старт
seat("Guest42", "PC-08")
sd = sell("later", guest_id=uid("Guest42"))
rd = void(sd)  # долг: без поля — «не возвращали»
end_all("Guest42")
s, cur = api("GET", f"/admin/reports/returns?month={month}", None, "boss")
by = {x.get("id"): x for x in cur.get("items") or []}
A, B, D, P = (by.get(i, {}) for i in (sa, sb, dep, up))
check(f"месяц {month}: окно — клубные сутки месяца, все четыре отмены в списке, свежие сверху",
      s == 200 and cur.get("month") == month and (cur.get("period") or {}).get("from") == month + "-01" and all((A, B, D, P))
      and [x.get("id") for x in (cur.get("items") or [])[:4]] == [up, dep, sb, sa], J([s, cur.get("period"), [x.get("id") for x in (cur.get("items") or [])[:4]]]))
check("отмена старой продажи — в месяце отмены: оплачено 12 zł картой 15-го прошлого месяца («из прошлых месяцев»), вернули 12 zł "
      "наличными, гость Guest41, отменил boss",
      A.get("kind") == "sale" and A.get("name") == NAME and A.get("paid_method") == "card" and near(A.get("paid_pln", 0), 12)
      and str(A.get("paid_at", "")).startswith(prev + "-15") and A.get("earlier") is True and A.get("refund_method") == "cash"
      and near(A.get("refund_pln", 0), 12) and A.get("guest") == "Guest41" and A.get("voided_by") == "boss", J(A))
check("«не возвращали» — вернули 0, оплачено 12 налом; депозит — 7 zł по BLIK; пакет с кошелька — на кошелёк вся цена (минуты целы); "
      "свои — не «из прошлых месяцев», отменил z11rt",
      B.get("refund_method") == "none" and near(B.get("refund_pln", -1), 0) and near(B.get("paid_pln", 0), 12) and B.get("paid_method") == "cash"
      and D.get("kind") == "deposit" and D.get("refund_method") == "blik" and near(D.get("refund_pln", 0), 7) and D.get("guest") == "Guest41"
      and P.get("kind") == "package" and P.get("refund_method") == "wallet" and near(P.get("refund_pln", 0), price) and P.get("paid_method") == "wallet"
      and not any(x.get("earlier") for x in (B, D, P)) and {x.get("voided_by") for x in (B, D, P)} == {"z11rt"}, J([B, D, P]))


def direct(mon, nextmon):
    """Итоги месяца прямым SQL, в грошах — независимо от сервера; окно — клубные сутки месяца; подарки (З11-и5) — не возврат."""
    f, t = at_club(mon + "-01", f"{RH:02d}:00"), at_club(nextmon + "-01", f"{RH:02d}:00")
    live = "('cash','card','blik','wallet')"
    q = f"""WITH r AS (
      SELECT COALESCE(refund_method, method) m, refund_grosz g, ROUND(amount_pln * 100)::bigint p, created_at pa FROM deposits WHERE voided_at >= {f} AND voided_at < {t}
      UNION ALL SELECT COALESCE(refund_method, method), ROUND(total_pln * 100)::bigint, ROUND(total_pln * 100)::bigint, paid_at FROM sales
        WHERE paid_at IS NOT NULL AND method <> 'comp' AND voided_at >= {f} AND voided_at < {t}
      UNION ALL SELECT COALESCE(refund_method, method), refund_grosz, ROUND(price_pln * 100)::bigint, created_at FROM user_packages
        WHERE method <> 'comp' AND voided_at >= {f} AND voided_at < {t})
    SELECT concat_ws('|', count(*),
      COALESCE(SUM(g) FILTER (WHERE m IN ('cash','card','blik')), 0), count(*) FILTER (WHERE m IN ('cash','card','blik')),
      COALESCE(SUM(g) FILTER (WHERE m = 'cash'), 0), count(*) FILTER (WHERE m = 'cash'),
      COALESCE(SUM(g) FILTER (WHERE m = 'card'), 0), count(*) FILTER (WHERE m = 'card'),
      COALESCE(SUM(g) FILTER (WHERE m = 'blik'), 0), count(*) FILTER (WHERE m = 'blik'),
      COALESCE(SUM(g) FILTER (WHERE m = 'wallet'), 0), count(*) FILTER (WHERE m = 'wallet'),
      COALESCE(SUM(p) FILTER (WHERE m NOT IN {live}), 0), count(*) FILTER (WHERE m NOT IN {live}),
      COALESCE(SUM(CASE WHEN m IN {live} THEN g ELSE 0 END) FILTER (WHERE pa < {f}), 0), count(*) FILTER (WHERE pa < {f}),
      (SELECT count(*) FROM sales WHERE paid_at IS NULL AND voided_at >= {f} AND voided_at < {t})) FROM r"""
    return [int(x) for x in sql(q).split("|")]


def api_totals(t):
    g = lambda k: round((t.get(k) or 0) * 100)
    return [t.get("count"), g("live_pln"), t.get("live_count"), g("cash_pln"), t.get("cash_count"), g("card_pln"), t.get("card_count"),
            g("blik_pln"), t.get("blik_count"), g("wallet_pln"), t.get("wallet_count"), g("none_paid_pln"), t.get("none_count"),
            g("earlier_pln"), t.get("earlier_count")]


want = direct(month, nxt)
got = api_totals(cur.get("totals") or {})
check("итоги месяца — как прямой SQL до гроша: штук, живыми деньгами, наличными, на карту, по BLIK, на кошелёк, «не возвращали», "
      "по операциям прошлых месяцев; список — все отмены (или последние 300)",
      got == want[:15] and cur.get("items_total") == want[0] and len(cur.get("items") or []) == min(want[0], 300), J([got, want]))
check("неоплаченный долг — не возврат: продажу в долг Guest42 за PC-08 отменили до расчёта — денег не брали, в списке и итогах её нет",
      rd == (200, "none") and want[15] >= 1 and got[0] == want[0] and sd not in {x.get("id") for x in cur.get("items") or []}
      and all(x.get("paid_at") for x in cur.get("items") or []), J([rd, want[15], got[0]]))

section("прошлый месяц — по умолчанию")
sg = sell("card")
rg = void(sg, "z11rt", "card")
sql(f"UPDATE sales SET created_at = {at_club(prev + '-10', '12:00')}, paid_at = {at_club(prev + '-10', '12:00')}, "
    f"voided_at = {at_club(month + '-01', f'{RH:02d}:00')} - interval '1 minute' WHERE id = '{sg}'")
s, old = api("GET", "/admin/reports/returns", None, "boss")
G = next((x for x in old.get("items") or [] if x.get("id") == sg), {})
s2, cur2 = api("GET", f"/admin/reports/returns?month={month}", None, "boss")
check(f"без month — прошлый месяц {prev}: продажа, оплаченная 10-го и отменённая в последнюю минуту месяца, — в нём (картой, не «из прошлых месяцев»); "
      "в текущем месяце её нет — возврат живёт в дне отмены",
      rg == (200, "card") and s == 200 and old.get("month") == prev and (old.get("period") or {}).get("preset") == "prev_month"
      and G.get("refund_method") == "card" and near(G.get("refund_pln", 0), 12) and G.get("earlier") is False
      and sg not in {x.get("id") for x in cur2.get("items") or []},
      J([rg, s, old.get("month"), old.get("period"), G]))
check("итоги прошлого месяца — как прямой SQL", api_totals(old.get("totals") or {}) == direct(prev, month)[:15],
      J([api_totals(old.get("totals") or {}), direct(prev, month)]))

section("права и месяц")
bad = [api("GET", "/admin/reports/returns?month=" + quote(m), None, "boss")[0] for m in (nxt, month[:5] + "1", "август", month + "-01")]
check("админу — 403; будущий месяц, месяц без нуля, слово, дата — 400 bad_month",
      api("GET", "/admin/reports/returns", None, "z11rt")[0] == 403 and bad == [400, 400, 400, 400]
      and api("GET", f"/admin/reports/returns?month={nxt}", None, "boss")[1].get("code") == "bad_month", J(bad))

section("кассовая книга")
s, book = api("GET", "/admin/cash/book?preset=today", None, "boss")
day = (book.get("days") or [{}])[-1]
refs = {x.get("id"): x for x in day.get("refunds") or []}
a = refs.get(sa, {})
check("сегодня в книге — возврат наличными по A: 12 zł, позиция, гость Guest41, отдал boss, оплачено картой 15-го прошлого месяца "
      "(раньше этого дня); «не возвращали», BLIK и кошелёк — не из ящика, их в списке нет",
      s == 200 and near(a.get("refund_pln", 0), 12) and a.get("name") == NAME and a.get("guest") == "Guest41" and a.get("voided_by") == "boss"
      and a.get("paid_method") == "card" and a.get("earlier") is True and not {sb, dep, up} & set(refs), J([a, list(refs)[:5]]))
s, week = api("GET", "/admin/cash/book?preset=d7", None, "boss")
diff = [(d.get("date"), d.get("refunds_pln"), round(sum(x.get("refund_pln", 0) for x in d.get("refunds") or []), 2)) for d in week.get("days") or []]
check("каждый день недели: сумма списка возвратов наличными = «возвраты» дня; в списке — только наличные и не ноль",
      s == 200 and diff and all(near(x[1], x[2]) for x in diff)
      and all(x.get("refund_method") == "cash" and x.get("refund_pln", 0) > 0 for d in week.get("days") or [] for x in d.get("refunds") or []),
      J([x for x in diff if not near(x[1], x[2])] or diff[-2:]))
check("наличные туда и обратно сошлись: A −12 из ящика, B +12 остались — «должно быть в ящике» как в начале", near(exp(), E0), J([E0, exp()]))
done()
