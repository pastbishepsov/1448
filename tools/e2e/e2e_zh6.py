#!/usr/bin/env python3
"""Ж6 «Касса» (Ж-Р8, №16, 17, 26, 32): постоплата товаров как кухня, долг гостя = кухня + бар,
«Рассчитать» одной операцией, корзина/чек с receipt_id, заказ кухни от стойки, закрытие смены с
пересчётом наличных. Живой e2e против стенда с моком агента.
Идемпотентен: сессии Guest29 снимает, продажи/заказы отменяет или рассчитывает, смену admin2 закрывает.
На долгоживущем стенде не ломается (З7-и5): товар под проверку докладывает сама корректировкой (З18-и4ж: не приходом —
приход ждёт цен у владельца), новые сообщения канала персонала ищет по id — канал отдаёт последние 100."""
import os, json, subprocess, sys, time, os, urllib.request, urllib.error, datetime as dt

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr + "\n" + q)
    return r.stdout.strip().splitlines()[0] if r.stdout.strip() else ""
def api(m, p, b=None, t=None):
    req = urllib.request.Request(API + p, data=json.dumps(b).encode() if b is not None else None, method=m)
    req.add_header("Content-Type", "application/json")
    if t: req.add_header("Authorization", "Bearer " + t)
    try:
        with urllib.request.urlopen(req) as r: return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try: return e.code, json.loads(e.read() or b"{}")
        except Exception: return e.code, {}
    finally: time.sleep(0.13)
def login(n, p): return api("POST", "/auth/login", {"login": n, "password": p})[1]["access_token"]
ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    cond = bool(cond); ok += cond; fail += (not cond)
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))
def near(a, b, eps=0.005): return abs(float(a) - float(b)) < eps

agents = {}
def agent_start(pc_id, name):
    log = agent_log(name)
    if os.path.exists(log): os.remove(log)
    p = subprocess.Popen(["node", MOCK_AGENT, pc_id, log], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
    agents[name] = (p, log)
    for _ in range(30):
        if os.path.exists(log) and '"open"' in open(log).read(): break
        time.sleep(0.1)
    time.sleep(0.4)
def agent_stop(name):
    if name in agents:
        agents[name][0].terminate()
        try: agents[name][0].wait(3)
        except Exception: pass
        del agents[name]
        time.sleep(0.6)

a1 = login("admin1", "Admin12345"); a2 = login("admin2", "Admin12345"); ot = login("boss", "Boss12345"); gt = login("Guest29", "Guest12345")
g29 = sql("SELECT id FROM users WHERE nickname='Guest29'"); g28 = sql("SELECT id FROM users WHERE nickname='Guest28'")
adm2 = sql("SELECT id FROM users WHERE nickname='admin2'")
pcs = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
P9 = pcs["PC-09"]
# трек З, З7-и5: товар под проверку докладывает сама. Каждый прогон съедает несколько штук насовсем (расчёт
# долга — настоящая продажа), и на долгоживущем стенде остатка переставало хватать: проверка падала на «нужно ≥3
# позиции с остатком ≥6», хотя касса была ни при чём. З18-и4ж: корректировкой, а не приходом — с З10-и4 приход это
# поставка, она ждёт цен у владельца, и «Закупки» стенда копили по две-три такие за прогон
act = [x for x in api("GET", "/admin/goods", None, a1)[1]["goods"] if x["active"]]
assert len(act) >= 3, "нужно ≥3 активных позиции на стенде"
for x in act[:3]:
    if x["stock"] < 30:
        s, d = api("POST", f"/admin/goods/{x['id']}/stock", {"delta": 30 - x["stock"], "reason": "adjust", "note": "e2e_zh6: доложено под проверку"}, ot)
        assert s == 200, f"доложить {x['name']}: {s} {d}"
goods = [x for x in api("GET", "/admin/goods", None, a1)[1]["goods"] if x["active"] and x["stock"] >= 6]
GA, GB, GC = goods[0], goods[1], goods[2]
stock = lambda gid: int(sql(f"SELECT stock FROM goods WHERE id='{gid}'"))
wallet = lambda uid: int(sql(f"SELECT wallet_grosz FROM users WHERE id='{uid}'"))

def reset():
    for name in list(agents): agent_stop(name)
    for sid in sql(f"SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id IN ('{g29}','{g28}') AND status='active'").split():
        api("POST", f"/admin/sessions/{sid}/end", {}, a1)
    # долги тестовых гостей закрываем расчётом (не SQL — иначе рвётся склад/выручка), потом ничего не висит
    for uid in (g29, g28):
        api("POST", f"/admin/users/{uid}/settle", {"method": "card"}, ot)
    sql("UPDATE computers SET status='available' WHERE name='PC-09' AND id NOT IN (SELECT computer_id FROM sessions WHERE status='active')")
    sql(f"UPDATE users SET wallet_grosz=3000, coin_minutes=0 WHERE id='{g29}'"); sql(f"DELETE FROM wallet_transactions WHERE user_id='{g29}'")
reset()
# трек З, З8-и1: касса считает ящик клуба; на стенде без истории ящика первая сдача — нулевая точка, а проверке
# нужны ожидаемое и расхождения — ящик заводит admin2 сдачей на 500 zł
api("POST", "/admin/work/start", {}, a2)
if api("GET", "/admin/work/closeout", None, a2)[1].get("baseline"):
    api("POST", "/admin/work/stop", {"counted_pln": 500}, a2)

def revenue_today():
    s, d = api("GET", "/admin/reports/money?preset=today", None, ot)
    return d["totals"]["goods_pln"], d["totals"]["revenue_pln"]
def sales_today(tok=a1):
    return api("GET", "/admin/sales", None, tok)[1]
def card_stats(uid): return api("GET", f"/admin/users/{uid}", None, a1)[1]["stats"]
def fresh(after, before):
    """новые сообщения канала персонала — по id, а не по длине списка: канал отдаёт последние 100, и на
    долгоживущем стенде длина перестаёт расти (трек З, З7-и5)"""
    seen = {m["id"] for m in before}
    return [m for m in after if m["id"] not in seen]

# ── A: постоплата (и1) ──
s, d = api("POST", "/admin/sales", {"good_id": GA["id"], "method": "later", "guest_id": g29}, a1)
check("в долг гостю без сессии → 409 need_session", s == 409 and d.get("code") == "need_session", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/sales", {"good_id": GA["id"], "method": "later"}, a1)
check("в долг без гостя → 400 need_guest", s == 400 and d.get("code") == "need_guest", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/sales", {"good_id": GA["id"], "method": "wallet"}, a1)
check("method=wallet для товаров → 400 bad_method (Ж-Р8: кошельком не платятся)", s == 400 and d.get("code") == "bad_method", f"{s} {d.get('code')}")

agent_start(P9["id"], "pc09")
s, d = api("POST", f"/admin/computers/{P9['id']}/session", {"guest_id": g29, "planned_min": 60}, a1)
check("Guest29 за PC-09", s == 201, str(s)); api("POST", "/me/sessions/ready", {}, gt)
goods0, rev0 = revenue_today(); st0 = sales_today(); stockA0 = stock(GA["id"]); w0 = wallet(g29)
s, d = api("POST", "/admin/sales", {"good_id": GA["id"], "qty": 2, "method": "later", "guest_id": g29}, a1)
check("в долг гостю за ПК → 201, sale.unpaid, postpaid, receipt.unpaid, debt.bar_pln = 2×цена",
      s == 201 and d["sale"]["unpaid"] and d["sale"]["postpaid"] and d["receipt"]["unpaid"] and near(d["debt"]["bar_pln"], 2 * GA["price_pln"]) and d["sale"].get("paid_at") is None,
      f"{s} {json.dumps({k: d.get(k) for k in ('debt',)}, ensure_ascii=False)}")
later1 = d["sale"]["id"]; rid_later = d["receipt"]["id"]
check("склад списан сразу (товар выдан)", stock(GA["id"]) == stockA0 - 2, f"{stock(GA['id'])} vs {stockA0 - 2}")
goods1, rev1 = revenue_today(); st1 = sales_today()
check("выручка не выросла: goods_pln и revenue_pln как до продажи в долг", near(goods1, goods0) and near(rev1, rev0), f"{goods0}→{goods1}")
check("/admin/sales: total_pln не вырос, unpaid=1, unpaid_pln = сумма долга, строка с unpaid=true и receipt_id",
      near(st1["total_pln"], st0["total_pln"]) and st1["unpaid"] == 1 and near(st1["unpaid_pln"], 2 * GA["price_pln"])
      and any(x["id"] == later1 and x["unpaid"] and x.get("receipt_id") == rid_later for x in st1["sales"]), f"{st1['total_pln']} unpaid={st1.get('unpaid')}")
stc = card_stats(g29)
check("карточка: stats.unpaid_pln = бар, bar_unpaid_count 1, кухни нет", near(stc.get("unpaid_pln", 0), 2 * GA["price_pln"]) and stc.get("bar_unpaid_count") == 1 and not stc.get("kitchen_unpaid_count"), json.dumps(stc, ensure_ascii=False)[:200])
s, d = api("GET", f"/admin/users/{g29}/debt", None, a1)
check("GET /debt: items — одна позиция bar с receipt_id, debt.total = бар", s == 200 and len(d["items"]) == 1 and d["items"][0]["kind"] == "bar" and d["items"][0].get("receipt_id") == rid_later and near(d["debt"]["total_pln"], 2 * GA["price_pln"]), f"{s}")
s, d = api("GET", "/me/kitchen/orders", None, gt)
check("гость: /me/kitchen/orders → bar_unpaid_pln и unpaid_total_pln", near(d.get("bar_unpaid_pln", 0), 2 * GA["price_pln"]) and near(d.get("unpaid_total_pln", 0), 2 * GA["price_pln"]), json.dumps({k: d.get(k) for k in ("bar_unpaid_pln", "unpaid_total_pln", "unpaid_pln")}))

# ── и3: заказ кухни от стойки (в долг → кухня, потом выдан) ──
stockB0 = stock(GB["id"])
s, d = api("POST", "/admin/kitchen/orders", {"guest": "+48500120029", "good_id": GB["id"], "qty": 1}, a1)
check("заказ кухни от стойки по телефону, при выходе → 201, статус accepted, computer PC-09, debt.bar как было",
      s == 201 and d["order"]["status"] == "accepted" and d["order"].get("computer") == "PC-09" and d.get("nickname") == "Guest29" and near(d["debt"]["bar_pln"], 2 * GA["price_pln"]) and d["debt"]["kitchen_count"] == 0,
      f"{s} {d.get('code')} {json.dumps(d.get('order'), ensure_ascii=False)[:160]}")
kid = d["order"]["id"]
check("склад кухни списан", stock(GB["id"]) == stockB0 - 1, "")
s, d = api("POST", "/admin/kitchen/orders", {"guest": "Guest2", "good_id": GB["id"]}, a1)
check("заказ «Guest2» → 409 guest_ambiguous с кандидатами", s == 409 and d.get("code") == "guest_ambiguous" and len(d.get("candidates") or []) >= 2, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/kitchen/orders", {"guest_id": g28, "good_id": GB["id"]}, a1)
check("заказ при выходе гостю без сессии → 409 need_session", s == 409 and d.get("code") == "need_session", f"{s} {d.get('code')}")
goods_k0, _ = revenue_today()
s, d = api("POST", "/admin/kitchen/orders", {"guest_id": g28, "good_id": GB["id"], "method": "card"}, a1)
check("заказ без сессии с оплатой сразу (карта) → 201, sale_id, выручка +цена", s == 201 and d.get("sale_id") and near(revenue_today()[0], goods_k0 + GB["price_pln"]), f"{s} {d.get('code')}")
paid_kitchen_sale = d.get("sale_id"); kid28 = d["order"]["id"]
s, d = api("POST", f"/admin/kitchen/orders/{kid}/status", {"status": "done"}, a1)
check("заказ Guest29 выдан → ждёт оплаты", s == 200, str(s))
stc = card_stats(g29)
check("карточка: долг = бар + кухня, kitchen_unpaid_count 1", near(stc.get("unpaid_pln", 0), 2 * GA["price_pln"] + GB["price_pln"]) and stc.get("kitchen_unpaid_count") == 1 and stc.get("bar_unpaid_count") == 1, json.dumps(stc, ensure_ascii=False)[:220])

# ── завершение сессии: чек и уведомление с полным долгом ──
api("GET", "/me/notifications", None, gt)  # очистить старые
sid = sql(f"SELECT id FROM sessions WHERE user_id='{g29}' AND status='active'")
s, d = api("POST", f"/admin/sessions/{sid}/end", {}, a1)
rc = d.get("receipt") or {}
check("чек завершения: unpaid_pln = кухня + бар, bar_unpaid_pln, kitchen_unpaid_pln",
      near(rc.get("unpaid_pln", 0), 2 * GA["price_pln"] + GB["price_pln"]) and near(rc.get("bar_unpaid_pln", 0), 2 * GA["price_pln"]) and near(rc.get("kitchen_unpaid_pln", 0), GB["price_pln"]), json.dumps(rc))
nts = api("GET", "/me/notifications", None, gt)[1]["items"]
un = [n for n in nts if n["type"] == "kitchen_unpaid"]
check("гостю уведомление kitchen_unpaid с total_pln = весь долг и bar_pln", un and near(un[-1]["payload"]["total_pln"], 2 * GA["price_pln"] + GB["price_pln"]) and near(un[-1]["payload"].get("bar_pln", 0), 2 * GA["price_pln"]), json.dumps(un[-1]["payload"] if un else nts)[:200])
ss = api("GET", "/me/sessions", None, gt)[1]
check("/me/sessions receipt после завершения — тот же долг", near((ss.get("receipt") or {}).get("unpaid_pln", -1), 2 * GA["price_pln"] + GB["price_pln"]), json.dumps(ss.get("receipt")))

# ── «Рассчитать» одной операцией ──
s, d = api("POST", f"/admin/users/{g29}/settle", {"method": "wallet"}, a1)
check("расчёт кошельком → 400 bad_method (Ж-Р8)", s == 400 and d.get("code") == "bad_method", f"{s} {d.get('code')}")
w_settle = wallet(g29)  # сессия уже списала своё — сравниваем с моментом перед расчётом
goods2, rev2 = revenue_today(); st2 = sales_today()
prev_cash = api("GET", "/admin/work/closeout", None, a1)[1]
s, d = api("POST", f"/admin/users/{g29}/settle", {"method": "cash"}, a1)
total = 2 * GA["price_pln"] + GB["price_pln"]
check("settle cash → 200: paid_pln = кухня + бар, kitchen.count 1, bar.count 1, debt.total 0",
      s == 200 and near(d["paid_pln"], total) and d["kitchen"]["count"] == 1 and d["bar"]["count"] == 1 and near(d["debt"]["total_pln"], 0), f"{s} {json.dumps(d, ensure_ascii=False)[:200]}")
row = sql(f"SELECT method || '|' || postpaid::text || '|' || (paid_at IS NOT NULL)::text || '|' || (paid_by='{sql('SELECT id FROM users WHERE nickname=%s' % chr(39)+'admin1'+chr(39))}')::text FROM sales WHERE id='{later1}'")
check("строка бара: method=cash, postpaid=true, paid_at есть, paid_by=admin1", row == "cash|true|true|true", row)
krow = sql(f"SELECT (paid_at IS NOT NULL)::text || '|' || COALESCE(pay_method,'') || '|' || (sale_id IS NOT NULL)::text FROM kitchen_orders WHERE id='{kid}'")
check("заказ кухни: paid_at, pay_method=cash, sale_id", krow == "true|cash|true", krow)
ksale = sql(f"SELECT method || '|' || postpaid::text || '|' || (paid_at IS NOT NULL)::text FROM sales WHERE id=(SELECT sale_id FROM kitchen_orders WHERE id='{kid}')")
check("продажа кухни создана расчётом: cash, postpaid, paid_at", ksale == "cash|true|true", ksale)
goods3, rev3 = revenue_today(); st3 = sales_today()
check("выручка выросла ровно на сумму расчёта (goods_pln и revenue_pln)", near(goods3, goods2 + total) and near(rev3, rev2 + total), f"{goods2}→{goods3} (+{total})")
check("/admin/sales: total_pln +сумма, unpaid 0, строка бара уже не unpaid и postpaid", near(st3["total_pln"], st2["total_pln"] + total) and st3["unpaid"] == 0
      and any(x["id"] == later1 and not x["unpaid"] and x["postpaid"] and x["method"] == "cash" for x in st3["sales"]), f"{st2['total_pln']}→{st3['total_pln']}")
cash_after = api("GET", "/admin/work/closeout", None, a1)[1]
check("ожидаемое в ящике (глазами admin1) выросло на сумму расчёта налом", near(cash_after["expected_pln"], prev_cash["expected_pln"] + total) and near(cash_after["goods_pln"], prev_cash["goods_pln"] + total), f"{prev_cash['expected_pln']}→{cash_after['expected_pln']}")
s, d = api("POST", f"/admin/users/{g29}/settle", {"method": "cash"}, a1)
check("повторный расчёт → 409 no_debt", s == 409 and d.get("code") == "no_debt", f"{s} {d.get('code')}")
check("кошелёк гостя не тронут расчётом", wallet(g29) == w_settle, f"{wallet(g29)} vs {w_settle}")
aud = api("GET", "/admin/audit?kind=sale_paid,settle&limit=20", None, a1)[1]["items"]
check("журнал: строки sale_paid (расчёт: …) и settle (рассчитался у стойки)", any(i["kind"] == "sale_paid" and "расчёт:" in i["text"] for i in aud) and any(i["kind"] == "settle" and "кухня" in i["text"] and "бар" in i["text"] for i in aud), json.dumps([(i['kind'], i['text']) for i in aud[:4]], ensure_ascii=False)[:300])

# ── отмена неоплаченной продажи: склад назад, денег не брали (З11-и4: «долг списан» — это списание владельцем) ──
s, d = api("POST", f"/admin/computers/{P9['id']}/session", {"guest_id": g29, "planned_min": 60}, a1); api("POST", "/me/sessions/ready", {}, gt)
stockC0 = stock(GC["id"])
s, d = api("POST", "/admin/sales", {"good_id": GC["id"], "method": "later", "guest_id": g29}, a1)
later2 = d["sale"]["id"]
s, d = api("POST", f"/admin/sales/{later2}/void", {}, a1)
check("отмена продажи в долг → 200 unpaid=true, склад вернулся, долг 0, в кошелёк ничего не «вернулось»", s == 200 and d.get("unpaid") is True and stock(GC["id"]) == stockC0 and near(card_stats(g29).get("unpaid_pln", 0), 0)
      and sql(f"SELECT count(*) FROM wallet_transactions WHERE ref_id='{later2}'") == "0", f"{s} {d}")
aud = api("GET", "/admin/audit?kind=sale_void&limit=5", None, a1)[1]["items"]
check("журнал: отмена продажи в долг помечена «денег не брали, товар вернулся на склад»", any("денег не брали, товар вернулся на склад" in i["text"] for i in aud), json.dumps([i['text'] for i in aud[:2]], ensure_ascii=False)[:200])

# ── B: корзина / чек (и2) ──
sA, sB, sC = stock(GA["id"]), stock(GB["id"]), stock(GC["id"])
goods4, _ = revenue_today()
s, d = api("POST", "/admin/sales", {"items": [{"good_id": GA["id"], "qty": 2}, {"good_id": GB["id"]}, {"good_id": GC["id"], "qty": 1}, {"good_id": GA["id"], "qty": 1}], "method": "cash", "guest_id": g29}, a1)
rc = d.get("receipt") or {}
exp_total = 3 * GA["price_pln"] + GB["price_pln"] + GC["price_pln"]
check("чек из 4 строк (A×2 + A×1 сложились) → 201: receipt.count 3, total = A×3 + B + C, у всех один receipt_id, paid_at есть",
      s == 201 and rc.get("count") == 3 and near(rc.get("total_pln", 0), exp_total) and len({x["receipt_id"] for x in rc["items"]}) == 1 and all(x.get("paid_at") for x in rc["items"]) and not rc.get("unpaid"), f"{s} {json.dumps(rc, ensure_ascii=False)[:200]}")
rid = rc.get("id")
check("склад: A −3, B −1, C −1", stock(GA["id"]) == sA - 3 and stock(GB["id"]) == sB - 1 and stock(GC["id"]) == sC - 1, "")
check("выручка +сумма чека", near(revenue_today()[0], goods4 + exp_total), "")
aud = api("GET", "/admin/audit?kind=sale&limit=5", None, a1)[1]["items"]
check("журнал: чек одной строкой «чек: A ×3, B, C — сумма (cash)» с id receipt:", any(i["id"] == "receipt:" + rid and i["text"].startswith("чек:") and (" ×3" in i["text"]) for i in aud), json.dumps([(i['id'][:12], i['text']) for i in aud[:2]], ensure_ascii=False)[:300])
sa, sb2, sc2 = stock(GA["id"]), stock(GB["id"]), stock(GC["id"])
s, d = api("POST", "/admin/sales", {"items": [{"good_id": GA["id"], "qty": 1}, {"good_id": GB["id"], "qty": 99}], "method": "cash"}, a1)
check("чек с нехваткой одной позиции → 409 out_of_stock, ничего не списано (атомарно)", s == 409 and d.get("code") == "out_of_stock" and stock(GA["id"]) == sa and stock(GB["id"]) == sb2, f"{s} {d.get('code')} {d.get('message')}")
big = max(goods, key=lambda x: x["stock"])  # у кого больше всего на складе — 21 штука туда влезет
s, d = api("POST", "/admin/sales", {"items": [{"good_id": big["id"], "qty": 1}] * 21, "method": "cash"}, a1)
check("чек из 21 одинаковой строки → сложились в одну (qty 21) → 201", s == 201 and d["receipt"]["count"] == 1 and d["sale"]["qty"] == 21, f"{s} {d.get('code')} {d.get('message')} (склад {big['stock']})")
if s == 201: api("POST", f"/admin/receipts/{d['receipt']['id']}/void", {}, a1)
s, d = api("POST", "/admin/sales", {"items": [{"good_id": GA["id"] if i % 2 else GB["id"], "qty": 1} for i in range(2)] + [{"good_id": f"00000000-0000-0000-0000-0000000000{i:02d}", "qty": 1} for i in range(19)], "method": "cash"}, a1)
check("чек из 21 разных позиций → 400 too_many_items", s == 400 and d.get("code") == "too_many_items", f"{s} {d.get('code')}")
# отмена одной позиции чека
pos_b = next(x for x in rc["items"] if x["name"] == GB["name"])
s, d = api("POST", f"/admin/sales/{pos_b['id']}/void", {}, a1)
check("отмена одной позиции чека → 200, вернулась только она (B +1)", s == 200 and stock(GB["id"]) == sb2 + 1 and stock(GA["id"]) == sa and stock(GC["id"]) == sc2, f"{s}")
st = sales_today()
grp = [x for x in st["sales"] if x.get("receipt_id") == rid]
check("/admin/sales: у чека 3 строки с общим receipt_id, одна voided", len(grp) == 3 and sum(1 for x in grp if x["voided"]) == 1, str(len(grp)))
s, d = api("POST", f"/admin/receipts/{rid}/void", {}, a1)
check("отмена чека целиком → 200: остальные 2 позиции отменены, склад A и C вернулся", s == 200 and len(d.get("voided", [])) == 2 and stock(GA["id"]) == sA and stock(GC["id"]) == sC and stock(GB["id"]) == sB, f"{s} {d}")
s, d = api("POST", f"/admin/receipts/{rid}/void", {}, a1)
check("повторная отмена чека → 409 already_void", s == 409 and d.get("code") == "already_void", f"{s} {d.get('code')}")
check("выручка вернулась к значению до чека", near(revenue_today()[0], goods4), f"{revenue_today()[0]} vs {goods4}")
s, d = api("POST", "/admin/receipts/00000000-0000-0000-0000-000000000000/void", {}, a1)
check("неизвестный чек → 404", s == 404, str(s))
# чек в долг и «Отменить чек»
s, d = api("POST", "/admin/sales", {"items": [{"good_id": GA["id"]}, {"good_id": GC["id"]}], "method": "later", "guest_id": g29}, a1)
check("чек из 2 позиций в долг → receipt.unpaid, debt.bar_count 2", s == 201 and d["receipt"]["unpaid"] and d["debt"]["bar_count"] == 2, f"{s} {d.get('code')}")
s, v = api("POST", f"/admin/receipts/{d['receipt']['id']}/void", {}, a1)
check("отмена чека в долг → 200, долг 0, склад как был", s == 200 and near(card_stats(g29).get("unpaid_pln", 0), 0) and stock(GA["id"]) == sA and stock(GC["id"]) == sC, f"{s}")

# ── сводка смены: unpaid_pln (долг за смену) ──
s, d = api("POST", "/admin/sales", {"good_id": GA["id"], "method": "later", "guest_id": g29}, a1)
sh = api("GET", "/admin/stats/shift", None, ot)[1]
check("сводка смены (owner): revenue.unpaid_pln ≥ цена A, unpaid ≥ 1", sh["revenue"].get("unpaid_pln", 0) >= GA["price_pln"] - 0.005 and sh["revenue"].get("unpaid", 0) >= 1, json.dumps({k: sh['revenue'].get(k) for k in ('unpaid_pln', 'unpaid')}))
api("POST", f"/admin/sales/{d['sale']['id']}/void", {}, a1)

# ── C: закрытие смены с кассой (и4), admin2 ──
api("POST", "/admin/work/start", {}, a2)  # уже открыта после логина — 409 и ладно
pv0 = api("GET", "/admin/work/closeout", None, a2)[1]
mine1 = api("GET", "/admin/work/closeout", None, a1)[1].get("mine") or {}
# трек З, З8-и1: предпросмотр — ящик клуба: на начало, пришло, ушло, из чего и «из них провёл ты»
check("preview кассы admin2: on_shift, ящик — expected, opening, in/out, deposits/goods/packs/refunds, mine", pv0.get("on_shift") is True and all(k in pv0 for k in ("expected_pln", "opening_pln", "in_pln", "out_pln", "deposits_pln", "goods_pln", "packs_pln", "refunds_pln", "mine", "from")), json.dumps(pv0))
s, dep = api("POST", f"/admin/users/{g28}/deposit", {"amount_pln": 20, "method": "cash"}, a2)
check("admin2: депозит 20 zł налом → 201", s == 201, f"{s} {dep.get('code')}")
s, sale2 = api("POST", "/admin/sales", {"good_id": GA["id"], "method": "cash"}, a2)
s2, sale3 = api("POST", "/admin/sales", {"good_id": GB["id"], "method": "card"}, a2)
pv1 = api("GET", "/admin/work/closeout", None, a2)[1]
check("ожидаемая наличность admin2 выросла на 20 + цена A (карта не считается)", near(pv1["expected_pln"], pv0["expected_pln"] + 20 + GA["price_pln"]) and near(pv1["deposits_pln"], pv0["deposits_pln"] + 20) and near(pv1["goods_pln"], pv0["goods_pln"] + GA["price_pln"]), f"{pv0['expected_pln']}→{pv1['expected_pln']}")
pa1 = api("GET", "/admin/work/closeout", None, a1)[1]
check("ящик один на клуб: у admin1 «должно быть» то же, что у admin2, а его «из них провёл ты» от операций admin2 не меняется",
      near(pa1["expected_pln"], pv1["expected_pln"]) and near((pa1.get("mine") or {}).get("in_pln", -1), mine1.get("in_pln", -2)), f"{pa1['expected_pln']} / {pv1['expected_pln']}")
# отмена своей продажи налом — из плюса уходит сама
api("POST", f"/admin/sales/{sale2['sale']['id']}/void", {}, a2)
pv2 = api("GET", "/admin/work/closeout", None, a2)[1]
check("отменил свою продажу налом → ожидаемая сумма вернулась (−цена A), отмена — строкой «возвраты налом»", near(pv2["expected_pln"], pv1["expected_pln"] - GA["price_pln"]) and near(pv2.get("refunds_pln", 0), pv1.get("refunds_pln", 0) + GA["price_pln"]), f"{pv1['expected_pln']}→{pv2['expected_pln']}")
staff0 = api("GET", "/admin/staff-chat", None, ot)[1]["messages"]
s, d = api("POST", "/admin/work/stop", {"counted_pln": round(pv2["expected_pln"] - 20, 2), "note": "сдал Ане"}, a2)
co = d.get("closeout") or {}
check("уход с пересчётом (−20) → 200: closeout.expected = preview, counted, diff_pln −20, note, skipped false",
      s == 200 and near(co.get("expected_pln", -1), pv2["expected_pln"]) and near(co.get("counted_pln", -1), pv2["expected_pln"] - 20) and near(co.get("diff_pln", 0), -20) and co.get("note") == "сдал Ане" and co.get("skipped") is False, f"{s} {json.dumps(co, ensure_ascii=False)}")
row = sql(f"SELECT diff_grosz || '|' || skipped::text || '|' || (work_entry_id IS NOT NULL)::text || '|' || note FROM shift_closeouts WHERE user_id='{adm2}' ORDER BY created_at DESC LIMIT 1")
check("shift_closeouts: diff −2000 гр, skipped=false, work_entry_id, note", row == "-2000|false|true|сдал Ане", row)
staff1 = api("GET", "/admin/staff-chat", None, ot)[1]["messages"]
new_msgs = fresh(staff1, staff0)
# трек З, З1-и2 (Д4): деньги — служебной строкой «только владельцу», заметка — от admin2 всем
check("владельцу в канал персонала: служебная строка «admin2 … расхождение −20.00 zł» и заметка admin2 «Передал смену: сдал Ане»",
      any(m.get("kind") == "system" and m.get("visible_to") == "owner" and "admin2" in m["text"] and "расхождение −20.00 zł" in m["text"] for m in new_msgs)
      and any(m.get("nickname") == "admin2" and m["text"] == "Передал смену: сдал Ане" for m in new_msgs), json.dumps([m['text'] for m in new_msgs], ensure_ascii=False)[:300])
# трек З, З19-и7: сдача — в сводке дня своей записи табеля; ночью по графику это день начала ночной смены, а не «сегодня»
sh = api("GET", f"/admin/stats/shift?date={co.get('date', '')}", None, ot)[1]
co_sh = [c for c in sh.get("closeouts", []) if c["nickname"] == "admin2"]
check("сводка смены: closeouts содержит admin2 с diff_pln −20", co_sh and near(co_sh[-1]["diff_pln"], -20) and near(co_sh[-1]["counted_pln"], pv2["expected_pln"] - 20), json.dumps(co_sh[-1] if co_sh else sh.get('closeouts'), ensure_ascii=False)[:200])
ts = api("GET", f"/admin/work?from={co['date']}&to={co['date']}" if co.get("date") else "/admin/work?preset=today", None, ot)[1]
ent = [e for e in ts["entries"] if e["nickname"] == "admin2" and e.get("closeout")]
check("табель: у записи admin2 closeout с diff_pln −20", ent and near(ent[0]["closeout"]["diff_pln"], -20), str(len(ent)))
aud = api("GET", "/admin/audit?kind=shift_close&limit=5", None, ot)[1]["items"]
check("журнал: shift_close с «расхождение -20.00 zł»", any("расхождение -20.00" in i["text"] for i in aud), json.dumps([i['text'] for i in aud[:2]], ensure_ascii=False)[:200])
# без пересчёта
api("POST", "/admin/work/start", {}, a2)
staff2 = api("GET", "/admin/staff-chat", None, ot)[1]["messages"]
s, d = api("POST", "/admin/work/stop", {}, a2)
co = d.get("closeout") or {}
check("уход без тела → closeout.skipped=true, counted нет", s == 200 and co.get("skipped") is True and "counted_pln" not in co, f"{s} {json.dumps(co, ensure_ascii=False)[:160]}")
staff3 = api("GET", "/admin/staff-chat", None, ot)[1]["messages"]
check("владельцу: «Сдал смену без пересчёта кассы»", any("без пересчёта кассы" in m["text"] for m in fresh(staff3, staff2)), json.dumps([m['text'] for m in fresh(staff3, staff2)], ensure_ascii=False)[:200])
# касса сошлась — владельцу строка с суммой (трек З, З1-и1, №1 «а»: раньше молчали — «сошлось»
# было неотличимо от «вбил ожидаемую цифру»)
api("POST", "/admin/work/start", {}, a2)
pv3 = api("GET", "/admin/work/closeout", None, a2)[1]
staff4 = api("GET", "/admin/staff-chat", None, ot)[1]["messages"]
s, d = api("POST", "/admin/work/stop", {"counted_pln": pv3["expected_pln"]}, a2)
staff5 = api("GET", "/admin/staff-chat", None, ot)[1]["messages"]
check("касса сошлась → diff 0, владельцу строка «касса сошлась»", s == 200 and near(d["closeout"]["diff_pln"], 0)
      and any("касса сошлась" in m["text"] for m in fresh(staff5, staff4)), f"{s} {d.get('closeout', {}).get('diff_pln')} {json.dumps([m['text'] for m in fresh(staff5, staff4)], ensure_ascii=False)[:160]}")
s, d = api("POST", "/admin/work/stop", {"counted_pln": -5}, a2)
check("stop без открытой смены / отрицательная сумма → 400 или 409", s in (400, 409), f"{s} {d.get('code')}")
api("POST", "/admin/work/start", {}, a2)
s, d = api("POST", "/admin/work/stop", {"counted_pln": -5}, a2)
check("отрицательная сумма → 400 bad_amount", s == 400 and d.get("code") == "bad_amount", f"{s} {d.get('code')}")
api("POST", "/admin/work/stop", {"counted_pln": api("GET", "/admin/work/closeout", None, a2)[1]["expected_pln"]}, a2)

# уборка
api("POST", f"/admin/sales/{sale3['sale']['id']}/void", {}, a2)
api("POST", f"/admin/deposits/{dep.get('deposit_id')}/void", {}, a2)
api("POST", f"/admin/sales/{paid_kitchen_sale}/void", {}, a1)
api("POST", f"/admin/kitchen/orders/{kid28}/cancel", {}, a1)
reset()
print(f"\nИтого: ok={ok} fail={fail}")
sys.exit(1 if fail else 0)
