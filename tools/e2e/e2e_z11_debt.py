#!/usr/bin/env python3
"""З11-и4 «Списание долга и дебиторка» (№15; З-Р12; решения основателя 30.09). Долг гостя — бар «в долг» и выданный заказ
кухни «при выходе», только гостю за ПК. Владелец списывает весь долг одной кнопкой, без причины: POST
/admin/users/:id/debt/write-off (админу — 403); склад не трогаем, выручки нет; списанное не висит ни в карточке, ни в
расчёте, ни в «ждут оплаты» у кухни, ни в ленте продаж, ни у гостя; рассчитать, отменить продажу или чек, оплатить заказ,
пока списание стоит, нельзя (409 written_off). «Потери → долги» — по дню списания; «Дебиторка сейчас» — у кого, сколько, с
какого времени, за ПК или ушёл. Владелец возвращает списанный долг — POST /admin/debt-write-offs/:id/cancel: долг снова
открыт, рассчитывается как обычно (выручка — в день расчёта), отменённое списание — не потеря.
Данные: временный админ z11dt (на смене); гость Guest42 (кошелёк докладывается до 10 zł; за PC-08 — продажа в долг ×2 и
заказ кухни при выходе, выдан; сессия завершается — гость «ушёл, не рассчитавшись»); позиция «z11d Долг» (6 zł) заводится
каждый прогон и в конце удаляется вместе с поставкой. В конце долг рассчитан картой (ящик не трогаем), продажа бара
отменена с возвратом на карту; заказ кухни остаётся рассчитанным. Хвосты упавшего прогона — списания Guest42 возвращаются,
долг рассчитывается картой."""
from _z import *

NAME = "z11d Долг"
G42 = uid("Guest42")


def gone():
    for w in rows(f"SELECT id FROM debt_write_offs WHERE user_id = '{G42}' AND cancelled_at IS NULL"):
        api("POST", f"/admin/debt-write-offs/{w}/cancel", {}, "boss")
    end_all("Guest42")
    if float(api("GET", f"/admin/users/{G42}/debt", None, "boss")[1].get("debt", {}).get("total_pln") or 0) > 0:
        api("POST", f"/admin/users/{G42}/settle", {"method": "card"}, "boss")
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL AND paid_at IS NOT NULL "
                    f"AND id NOT IN (SELECT sale_id FROM kitchen_orders WHERE sale_id IS NOT NULL)"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "card"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
hire_temp("z11dt")
tok("z11dt")  # вход в админку открывает смену
at_end(gone)
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z11", "price_pln": 6, "stock": 20}, "boss")[1].get("good") or {}).get("id")
stock = lambda: int(sql(f"SELECT stock FROM goods WHERE id = '{good}'") or -1)
debt = lambda who="boss": api("GET", f"/admin/users/{G42}/debt", None, who)[1]
card = lambda who="boss": api("GET", f"/admin/users/{G42}", None, who)[1].get("stats") or {}
money = lambda: api("GET", "/admin/reports/money?preset=today", None, "boss")[1]
recv = lambda: next((x for x in (money().get("debts", {}).get("receivable", {}).get("items") or []) if x.get("user_id") == G42), None)

section("долг гостя за ПК")
ensure_wallet("Guest42", 10)  # посадке нужен запас на старт
sid = seat("Guest42", "PC-08")
s1 = api("POST", "/admin/sales", {"good_id": good, "qty": 2, "method": "later", "guest_id": G42}, "z11dt")
sale, rid = (s1[1].get("sale") or {}).get("id"), (s1[1].get("receipt") or {}).get("id")
ko = (api("POST", "/admin/kitchen/orders", {"guest_id": G42, "good_id": good, "qty": 1, "method": "later"}, "z11dt")[1].get("order") or {}).get("id")
st = api("POST", f"/admin/kitchen/orders/{ko}/status", {"status": "done"}, "z11dt")[0]
d0, r0 = debt(), recv()
check("бар в долг 2 × 6 zł и выданный заказ кухни 6 zł: должен 18,00 (кухня 6,00 · бар 12,00); склад −3; в «Дебиторке» — за PC-08",
      s1[0] == 201 and st == 200 and (d0.get("debt") or {}).get("total_pln") == 18 and (d0.get("debt") or {}).get("kitchen_pln") == 6
      and (d0.get("debt") or {}).get("bar_pln") == 12 and stock() == 17 and r0 and r0.get("computer") == "PC-08" and r0.get("total_pln") == 18,
      J([s1[0], st, d0.get("debt"), stock(), r0]))
end(sid)
r1, m1 = recv(), money().get("debts", {}).get("receivable", {})
check("гость ушёл, не рассчитавшись: в «Дебиторке» — без ПК, с какого времени — с первой позиции; «ушли, не рассчитавшись» ≥ 18,00, "
      "ушедшие — в списке первыми",
      r1 and r1.get("computer") == "" and str(r1.get("since", ""))[:16] == str(s1[1].get("sale", {}).get("created_at", ""))[:16]
      and m1.get("gone_pln", 0) >= 18 and m1.get("gone_guests", 0) >= 1
      and [x.get("computer") for x in m1.get("items") or []] == sorted([x.get("computer") for x in m1.get("items") or []], key=lambda c: c != ""),
      J([r1, {k: m1.get(k) for k in ("total_pln", "guests", "gone_pln", "gone_guests")}]))

section("списание")
wo0 = money().get("debts", {}).get("written_off", {})
rw = api("POST", f"/admin/users/{G42}/debt/write-off", {}, "z11dt")
check("админу списать — 403, долг на месте", rw[0] == 403 and (debt().get("debt") or {}).get("total_pln") == 18, J(rw))
rw = api("POST", f"/admin/users/{G42}/debt/write-off", None, "boss")
w = rw[1].get("write_off") or {}
check("владелец — одной кнопкой, без причины: 200, списано 18,00 (кухня 1 · 6,00, бар 1 · 12,00), кто — boss; долг — 0",
      rw[0] == 200 and w.get("total_pln") == 18 and (w.get("kitchen_count"), w.get("kitchen_pln"), w.get("bar_count"), w.get("bar_pln")) == (1, 6, 1, 12)
      and w.get("by") == "boss" and (rw[1].get("debt") or {}).get("total_pln") == 0, J(rw))
cs = card()
check("карточка: «Должен» больше нет, «Долг списан» — 18,00 от boss; склад не тронут (−3 так и есть)",
      not cs.get("unpaid_pln") and [(x.get("id"), x.get("total_pln"), x.get("by")) for x in cs.get("debt_written_off") or []] == [(w.get("id"), 18, "boss")]
      and stock() == 17, J([cs.get("unpaid_pln"), cs.get("debt_written_off"), stock()]))
blocked = [api("POST", f"/admin/users/{G42}/debt/write-off", None, "boss"), api("POST", f"/admin/users/{G42}/settle", {"method": "card"}, "z11dt"),
           api("POST", f"/admin/kitchen/orders/{ko}/pay", {"method": "cash"}, "z11dt"), api("POST", f"/admin/sales/{sale}/void", {}, "boss"),
           api("POST", f"/admin/receipts/{rid}/void", {}, "boss")]
check("списанное не трогается: второе списание и расчёт — 409 no_debt; оплата заказа, отмена продажи и чека — 409 written_off",
      [(s, d.get("code")) for s, d in blocked] == [(409, "no_debt"), (409, "no_debt"), (409, "written_off"), (409, "written_off"), (409, "written_off")],
      J([(s, d.get("code")) for s, d in blocked]))
kit = api("GET", "/admin/kitchen/orders", None, "z11dt")[1]
kk = next((o for o in kit.get("orders") or [] if o.get("id") == ko), {})
feed = api("GET", "/admin/sales", None, "z11dt")[1]
ss = next((x for x in feed.get("sales") or [] if x.get("id") == sale), {})
me = api("GET", "/me/kitchen/orders", None, "Guest42")[1]
check("кухня: заказ не «ждёт оплаты» — закрыт с пометкой «списан»; лента продаж: не «в долг», а «списан»; гость у себя долга не видит",
      kk.get("unpaid") is False and kk.get("written_off") is True and ss.get("unpaid") is False and ss.get("written_off") is True
      and me.get("unpaid_total_pln") == 0, J([kk, {k: ss.get(k) for k in ("unpaid", "written_off")}, me.get("unpaid_total_pln")]))
m2 = money().get("debts", {})
wo2 = m2.get("written_off", {})
check("«Деньги»: «Потери → долги» за сегодня выросли на 18,00 (+1 списание, в списке — Guest42 от boss); в «Дебиторке» Guest42 больше нет",
      near(wo2.get("pln", 0), wo0.get("pln", 0) + 18) and wo2.get("count") == wo0.get("count", 0) + 1
      and any(x.get("id") == w.get("id") and x.get("nickname") == "Guest42" and x.get("by") == "boss" for x in wo2.get("items") or [])
      and not any(x.get("user_id") == G42 for x in m2.get("receivable", {}).get("items") or []), J([wo0, {k: wo2.get(k) for k in ("pln", "count", "guests")}]))
aud = next((x for x in audit("boss", kind="debt_write_off", limit=20) if x.get("nickname") == "Guest42"), {})
check("журнал: «долг списан» у Guest42 в минуту списания — 18.00 zł · кухня 6.00 zł (1), бар 12.00 zł (1), кто — boss",
      aud.get("text") == "18.00 zł · кухня 6.00 zł (1), бар 12.00 zł (1)" and aud.get("admin") == "boss"
      and str(aud.get("created_at", ""))[:16] == str(w.get("created_at", ""))[:16], J([aud, w.get("created_at")]))

section("вернуть долг")
rc = [api("POST", f"/admin/debt-write-offs/{w.get('id')}/cancel", None, "z11dt"), api("POST", f"/admin/debt-write-offs/{w.get('id')}/cancel", None, "boss"),
      api("POST", f"/admin/debt-write-offs/{w.get('id')}/cancel", None, "boss")]
d3, cs3, wo3 = debt(), card(), money().get("debts", {}).get("written_off", {})
check("админу вернуть — 403; владелец вернул: долг снова 18,00 (кухня и бар), «Долг списан» ушёл из карточки; повторно — 409 already_cancelled; "
      "отменённое списание — не потеря: «Потери → долги» как до списания",
      [s for s, _ in rc] == [403, 200, 409] and rc[2][1].get("code") == "already_cancelled" and (d3.get("debt") or {}).get("total_pln") == 18
      and len(d3.get("items") or []) == 2 and cs3.get("unpaid_pln") == 18 and not cs3.get("debt_written_off")
      and near(wo3.get("pln", 0), wo0.get("pln", 0)) and wo3.get("count") == wo0.get("count"), J([[s for s, _ in rc], d3.get("debt"), wo3.get("pln")]))
rev0 = money().get("totals", {}).get("revenue_pln", 0)
rs = api("POST", f"/admin/users/{G42}/settle", {"method": "card"}, "z11dt")
rev1 = money().get("totals", {}).get("revenue_pln", 0)
wat = str(w.get("created_at", ""))  # «списан 30.09 19:30» — время этого списания
aud2 = next((x.get("text", "") for x in audit("boss", kind="debt_restore", limit=20) if x.get("nickname") == "Guest42"), "")
check("возвращённый долг рассчитывается как обычно: картой 18,00 — выручка сегодня +18,00; долга и строки в «Дебиторке» нет; журнал — «долг возвращён»",
      rs[0] == 200 and rs[1].get("paid_pln") == 18 and near(rev1, rev0 + 18) and (debt().get("debt") or {}).get("total_pln") == 0 and recv() is None
      and aud2 == f"вернули в долг 18.00 zł · списан {wat[8:10]}.{wat[5:7]} {wat[11:16]}", J([rs[0], rs[1].get("paid_pln"), rev0, rev1, aud2, wat]))
check("склад за всё время — только выдача: −3, списание и возврат его не трогали", stock() == 17, stock())
done()
