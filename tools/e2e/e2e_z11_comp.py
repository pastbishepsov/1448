#!/usr/bin/env python3
"""З11-и5 «Подарено» (№18; З-Р12, Д6; решение основателя 30.09). Продажа у стойки, выдача пакета и заказ кухни от стойки
— способ comp: подарок не выручка и не ящик, товар уходит со склада, пакет даёт минуты. Причина обязательна (решение
основателя 30.09): без неё — 400 comp_reason, длиннее 200 символов — 400 comp_reason_long; она в продаже, пакете, журнале
и в «Деньгах». Админ дарит в пределах дневного потолка компенсаций (admin_day_comp_cap_pln — один на компенсации и
подарки, Д6), сверх — 403 day_cap, и компенсация тоже упирается в подаренное; владелец — без лимита. «Деньги»: выручка,
маржа и «Что покупают» подарков не видят; «Подарено» — по цене и по себестоимости партий, список с причинами. Отмена
подарка — денег не было: только «не возвращали» (и у владельца), в «Возвратах» подарков нет; товар вернулся на склад.
Данные: временный админ z11ct (на смене; ящик заводится, если его нет); гость Guest41; потолок компенсаций на время
проверки — 25 zł сверх уже выданного z11ct за эти клубные сутки (настройки возвращаются как были); позиция «z11c Подарок» (8 zł) и её поставка 10 шт по 3 zł (расход
поставки в конце сторнируется) заводятся каждый прогон и в конце удаляются; самый дешёвый пакет каталога. Все подарки по
ходу отменяются; наличные не трогаются — «должно быть в ящике» в конце как в начале."""
from _z import *

NAME, MARK = "z11c Подарок", "z11c подарок"
G41 = uid("Guest41")
WHY = "победа в турнире"


def gone():
    for oid in rows(f"SELECT id FROM kitchen_orders WHERE name = '{NAME}' AND status IN ('new','accepted','preparing','delivering')"):
        api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE method = 'comp' AND voided_at IS NULL AND comp_reason LIKE '{MARK}%'"):
        api("POST", f"/admin/user-packages/{pid}/void", {}, "boss")
    for eid in rows(f"SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%{NAME}%'"):  # расход поставки: «поставка от …: z11c Подарок ×10»
        api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
hire_temp("z11ct")
tok("z11ct")  # вход в админку открывает смену
drawer_ready("z11ct")
at_end(gone)
# выданное z11ct за эти клубные сутки до проверки (компенсация прогона на HEAD, где подарков нет, остаётся) — потолок сверх него
DAY0 = at_club(club_day(), sql("SELECT lpad(COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '0'), 2, '0') || ':00'"))
CT = uid("z11ct")
USED0 = int(sql(f"""SELECT COALESCE((SELECT SUM(comp_grosz) FROM incidents WHERE resolved_by = '{CT}' AND compensated_at >= {DAY0}), 0)
  + COALESCE((SELECT SUM(ROUND(total_pln * 100)) FROM sales WHERE method = 'comp' AND voided_at IS NULL AND created_by = '{CT}' AND created_at >= {DAY0}), 0)
  + COALESCE((SELECT SUM(ROUND(price_pln * 100)) FROM user_packages WHERE method = 'comp' AND voided_at IS NULL AND created_by = '{CT}' AND created_at >= {DAY0}), 0)""") or 0)
set_settings(admin_day_comp_cap_pln=25 + -(-USED0 // 100))
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z11", "price_pln": 8}, "boss")[1].get("good") or {}).get("id")
sup = (api("POST", "/admin/supplies", {"lines": [{"good_id": good, "qty": 10}], "note": f"{MARK}: приход"}, "z11ct")[1].get("supply") or {}).get("id")
priced = api("PUT", f"/admin/supplies/{sup}/pricing", {"lines": [{"good_id": good, "prices": [{"qty": 10, "price_pln": 3}]}], "method": "bank"}, "boss")[0]
pack = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
price = float(sql(f"SELECT price_pln FROM time_packages WHERE id = '{pack}'") or 0)
stock = lambda: int(sql(f"SELECT stock FROM goods WHERE id = '{good}'") or -1)
exp = lambda: api("GET", "/admin/work/closeout", None, "z11ct")[1].get("expected_pln", 0)
money = lambda: api("GET", "/admin/reports/money?preset=today", None, "boss")[1]
gift = lambda d, i: next((x for x in (d.get("gifts") or {}).get("items") or [] if x.get("id") == i), {})
E0, M0 = exp(), money()
T0, GM0, GF0, PK0 = M0.get("totals") or {}, M0.get("goods_margin") or {}, M0.get("gifts") or {}, M0.get("packages") or {}
check("данные: поставка 10 шт по 3 zł оценена, на складе 10", priced == 200 and stock() == 10, J([priced, stock()]))

section("подарок у стойки: товар")
sell = lambda body, who="z11ct": api("POST", "/admin/sales", {"good_id": good, "qty": 2, "method": "comp", "guest_id": G41, **body}, who)
bad = [sell({}), sell({"comp_reason": "   "}), sell({"comp_reason": "я" * 201})]
check("без причины — 400 comp_reason (и из одних пробелов), длиннее 200 символов — 400 comp_reason_long; склад не тронут",
      [(s, d.get("code")) for s, d in bad] == [(400, "comp_reason"), (400, "comp_reason"), (400, "comp_reason_long")] and stock() == 10,
      J([(s, d.get("code")) for s, d in bad]))
s1, d1 = sell({"comp_reason": f"  {MARK}: {WHY}  "})
sa = (d1.get("sale") or {}).get("id")
check("подарок 2 × 8 zł с причиной: 201, «подарено» (comp), причина — обрезана и лежит в продаже; склад −2",
      s1 == 201 and (d1.get("sale") or {}).get("method") == "comp" and (d1.get("sale") or {}).get("comp_reason") == f"{MARK}: {WHY}"
      and (d1.get("receipt") or {}).get("comp") is True and stock() == 8, J([s1, d1.get("sale"), d1.get("receipt", {}).get("comp")]))

section("подарок: заказ кухни от стойки")
kb = api("POST", "/admin/kitchen/orders", {"guest_id": G41, "good_id": good, "qty": 1, "method": "comp"}, "z11ct")
kk = api("POST", "/admin/kitchen/orders", {"guest_id": G41, "good_id": good, "qty": 1, "method": "comp", "comp_reason": f"{MARK}: извинение за PC-05"}, "z11ct")
ko, ks = (kk[1].get("order") or {}).get("id"), kk[1].get("sale_id")
check("заказ кухни «подарено» без причины — 400; с причиной — 201, заказ оплачен подарком, продажа с той же причиной; склад −1",
      (kb[0], kb[1].get("code")) == (400, "comp_reason") and kk[0] == 201 and (kk[1].get("order") or {}).get("pay_method") == "comp"
      and sql(f"SELECT method || '|' || comp_reason FROM sales WHERE id = '{ks}'") == f"comp|{MARK}: извинение за PC-05" and stock() == 7,
      J([kb[0], kb[1].get("code"), kk[0], kk[1].get("order")]))

section("не выручка, не ящик, не маржа — «Подарено»")
M1 = money()
T1, GM1, GF1 = M1.get("totals") or {}, M1.get("goods_margin") or {}, M1.get("gifts") or {}
ga, gk = gift(M1, sa), gift(M1, ks)
check("«Деньги»: выручка, товары, «Продано штук», «Продано товаров на» — как до подарков; в «Что покупают» позиции нет",
      (T1.get("revenue_pln"), T1.get("goods_pln"), T1.get("goods_items"), GM1.get("sold_pln")) == (T0.get("revenue_pln"), T0.get("goods_pln"), T0.get("goods_items"), GM0.get("sold_pln"))
      and not any(g.get("name") == NAME for g in M1.get("top_goods") or []), J([T0.get("revenue_pln"), T1.get("revenue_pln"), GM0.get("sold_pln"), GM1.get("sold_pln")]))
check("«Подарено»: +24,00 zł по цене (товары 16 + кухня 8), по себестоимости +9,00 (3 шт по 3 zł из партии), +2 подарка; в списке — "
      "с причинами, гостем и кто подарил",
      near(GF1.get("pln", 0) - GF0.get("pln", 0), 24) and near(GF1.get("goods_pln", 0) - GF0.get("goods_pln", 0), 24)
      and near((GF1.get("cost_pln") or 0) - (GF0.get("cost_pln") or 0), 9) and GF1.get("goods_units", 0) - GF0.get("goods_units", 0) == 3
      and GF1.get("count", 0) - GF0.get("count", 0) == 2
      and (ga.get("reason"), ga.get("guest"), ga.get("by"), ga.get("price_pln"), ga.get("qty")) == (f"{MARK}: {WHY}", "Guest41", "z11ct", 16, 2)
      and (gk.get("reason"), gk.get("price_pln")) == (f"{MARK}: извинение за PC-05", 8), J([{k: GF1.get(k) for k in ("pln", "goods_pln", "cost_pln", "goods_units", "count")}, ga, gk]))
feed = api("GET", "/admin/sales", None, "z11ct")[1]
fr = next((x for x in feed.get("sales") or [] if x.get("id") == sa), {})
check("«должно быть в ящике» не изменилось; лента продаж: подарок с причиной, в итогах — «подарено», не выручка",
      near(exp(), E0) and fr.get("method") == "comp" and fr.get("comp_reason") == f"{MARK}: {WHY}" and feed.get("comp_pln", 0) >= 24,
      J([E0, exp(), fr.get("method"), feed.get("comp_pln")]))
aud = [x.get("text", "") for x in audit("boss", kind="sale", limit=30)]
check("журнал: «… — 16.00 zł (подарено: <причина>)»", any(t == f"{NAME} ×2 — 16.00 zł (подарено: {MARK}: {WHY})" for t in aud), J(aud[:4]))

section("потолок админа (Д6) и владелец")
rp = api("POST", f"/admin/users/{G41}/packages", {"package_id": pack, "method": "comp", "comp_reason": f"{MARK}: постоянный гость"}, "z11ct")
ri = api("POST", "/admin/incidents", {"user_id": G41, "kind": "other", "comp_pln": 2}, "z11ct")
check(f"потолок 25 zł сверх выданного раньше за сутки: подарено 24 — пакет за {price:.2f} сверх (403 day_cap, «подарков и компенсаций»), и компенсация 2 zł тоже (потолок общий)",
      rp[0] == 403 and rp[1].get("code") == "day_cap" and "подарков и компенсаций" in rp[1].get("message", "")
      and ri[0] == 403 and ri[1].get("code") == "day_cap", J([rp, ri]))
lia0 = (money().get("packages") or {})
ro = api("POST", f"/admin/users/{G41}/packages", {"package_id": pack, "method": "comp", "comp_reason": f"{MARK}: постоянный гость"}, "boss")
up = (ro[1].get("package") or {}).get("id")
M2 = money()
lia1, T2, GF2 = M2.get("packages") or {}, M2.get("totals") or {}, M2.get("gifts") or {}
check(f"владелец без лимита: пакет «подарено» — 201; выручка и «Пакеты» как были, «Подарено» +{price:.2f} (пакеты), «Долг временем» в злотых "
      f"тот же, минут не отыграно больше",
      ro[0] == 201 and (ro[1].get("package") or {}).get("method") == "comp" and (ro[1].get("package") or {}).get("comp_reason") == f"{MARK}: постоянный гость"
      and (T2.get("revenue_pln"), T2.get("packs_pln")) == (T0.get("revenue_pln"), T0.get("packs_pln"))
      and near(GF2.get("packs_pln", 0) - GF1.get("packs_pln", 0), price) and near(lia1.get("liability_pln", 0), lia0.get("liability_pln", 0))
      and lia1.get("minutes_left", 0) > lia0.get("minutes_left", 0), J([ro[0], ro[1].get("package"), lia0, lia1, GF2.get("packs_pln")]))

section("отмена подарка")
vs = [api("POST", f"/admin/sales/{sa}/void", {"refund_method": "cash"}, who) for who in ("z11ct", "boss")] + [api("POST", f"/admin/sales/{sa}/void", {}, "z11ct")]
check("вернуть подарок деньгами нельзя — 400 refund_unpaid (и владельцу); без способа — отменён, «не возвращали», товар на складе",
      [(s, d.get("code") or d.get("refund_method")) for s, d in vs] == [(400, "refund_unpaid"), (400, "refund_unpaid"), (200, "none")]
      and vs[2][1].get("comp") is True and stock() == 9, J([(s, d.get("code") or d.get("refund_method")) for s, d in vs]))
kc = api("POST", f"/admin/kitchen/orders/{ko}/cancel", {}, "z11ct")
pv = api("POST", f"/admin/user-packages/{up}/void", {}, "boss")
ret = api("GET", "/admin/reports/returns?month=" + club_day()[:7], None, "boss")[1]
check("заказ кухни отменён — деньги не возвращаем, товар на складе; пакет отменён — возврат 0; в «Возвратах» подарков нет",
      kc[0] == 200 and not (kc[1].get("refund") or {}).get("pln") and pv[0] == 200 and pv[1].get("refund_method") == "none" and not pv[1].get("refund_pln")
      and stock() == 10 and not {sa, ks, up} & {x.get("id") for x in ret.get("items") or []}, J([kc, pv[1].get("refund_method"), stock()]))
GF3 = money().get("gifts") or {}
check("отменённые подарки из «Подарено» ушли; «должно быть в ящике» как в начале",
      near(GF3.get("pln", 0), GF0.get("pln", 0)) and GF3.get("count") == GF0.get("count") and near(exp(), E0), J([GF0.get("pln"), GF3.get("pln"), E0, exp()]))
aud2 = [x.get("text", "") for x in audit("boss", kind="sale_void", limit=30)]
check("журнал отмены: «(подарено) · подарок отменён, денег не было»",
      any(t == f"отмена: {NAME} ×2 — 16.00 zł (подарено) · подарок отменён, денег не было" for t in aud2), J(aud2[:3]))
done()
