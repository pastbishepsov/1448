#!/usr/bin/env python3
"""Ж1 (Ж-Р4): отмена депозита. Живой e2e против стенда (критерий Ж1 в ADMIN.md):
нетронутый депозит возвращается целиком; после списания — остаток и unrecovered;
чужой/вчерашний отменяет только владелец; повтор — 409; выручка дня = живые депозиты;
уведомление и журнал кошелька у гостя; дневной лимит админа отменённым не съедается.
Идемпотентен: кошелёк и монеты Guest27 возвращает, настройки — тоже."""
import os, json, subprocess, sys, time, urllib.request, urllib.error

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr)
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
def near(a, b): return abs(float(a) - float(b)) < 0.005

a1 = login("admin1", "Admin12345"); a2 = login("admin2", "Admin12345"); ot = login("boss", "Boss12345")
gt = login("Guest27", "Guest12345")
g = sql("SELECT id FROM users WHERE nickname='Guest27'")
sql("DELETE FROM settings WHERE key='admin_day_deposit_cap_pln'")
wallet = lambda: int(sql(f"SELECT wallet_grosz FROM users WHERE id='{g}'"))
coins = lambda: int(sql(f"SELECT coins_balance FROM users WHERE id='{g}'"))
w0, c0 = wallet(), coins()
print(f"Guest27: кошелёк {w0} гр, монет {c0}")
def deposit(amount, tok=a1):
    s, d = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": amount, "method": "cash"}, tok)
    assert s == 201, (s, d)
    return d
def void(dep_id, tok): return api("POST", f"/admin/deposits/{dep_id}/void", {}, tok)

# ── A: нетронутый депозит — всё назад ──
d1 = deposit(50)
given = d1["coins_granted"] + d1["bonus_coins"]
check(f"депозит 50 zł: кошелёк +5000 гр, монет +{given}", wallet() == w0 + 5000 and coins() == c0 + given)
s, v = void(d1["deposit_id"], a1)
check("отмена своего свежего депозита админом → 200", s == 200, f"{s} {v}")
check(f"ответ: refund 50, unrecovered 0, coins_taken {given}",
      near(v.get("refund_pln"), 50) and near(v.get("unrecovered_pln"), 0) and v.get("coins_taken") == given, f"{v}")
check("кошелёк и монеты вернулись к исходным", wallet() == w0 and coins() == c0, f"{wallet()}/{w0} {coins()}/{c0}")
s2, v2 = void(d1["deposit_id"], a1)
check("повтор отмены → 409 already_void", s2 == 409 and v2.get("code") == "already_void", f"{s2} {v2.get('code')}")
row = sql(f"SELECT voided_at IS NOT NULL, refund_grosz, coins_taken, voided_by=(SELECT id FROM users WHERE nickname='admin1') FROM deposits WHERE id='{d1['deposit_id']}'")
check("строка депозита: voided_at, refund_grosz=5000, coins_taken, voided_by=admin1", row == f"t|5000|{given}|t", row)
tx = sql(f"SELECT kind, amount_grosz, balance_after FROM wallet_transactions WHERE ref_id='{d1['deposit_id']}' AND kind='deposit_void'")
check("журнал кошелька: deposit_void −5000 с балансом после", tx == f"deposit_void|-5000|{w0}", tx)
inv = sql(f"SELECT COALESCE(SUM(amount_grosz),0) = (SELECT wallet_grosz FROM users WHERE id='{g}') FROM wallet_transactions WHERE user_id='{g}'")
check("инвариант кошелька: сумма журнала = баланс", inv == "t", inv)
s, n = api("GET", "/me/notifications?client=app", None, gt)
nv = [x for x in n.get("items", []) if x.get("type") == "deposit_void"]
pl = nv[-1]["payload"] if nv else {}
check("гостю пришло уведомление deposit_void с суммой, возвратом и остатком кошелька",
      len(nv) >= 1 and near(pl.get("amount_pln", 0), 50) and near(pl.get("refund_pln", 0), 50) and near(pl.get("wallet_pln", -1), w0 / 100),
      f"{s} {nv[-1] if nv else n}")
s, wd = api("GET", "/me/wallet", None, gt)
check("в /me/wallet первая операция — deposit_void", s == 200 and wd.get("transactions", [{}])[0].get("kind") == "deposit_void", f"{s}")
s, md = api("GET", "/me/deposits", None, gt)
mine = [x for x in md.get("deposits", []) if x["id"] == d1["deposit_id"]]
check("в /me/deposits депозит помечен voided_at", bool(mine) and mine[0].get("voided_at"), f"{s}")

# ── B: часть отыграна — остаток назад, разница в unrecovered ──
d2 = deposit(50)
given2 = d2["coins_granted"] + d2["bonus_coins"]
# «биллинг»: 38 zł ушло за время, монеты потрачены до 120
sql(f"UPDATE users SET wallet_grosz = wallet_grosz - 3800, coins_balance = 120 WHERE id='{g}'")
sql(f"INSERT INTO wallet_transactions(user_id, kind, amount_grosz, balance_after, ref_type) VALUES ('{g}', 'session_spend', -3800, (SELECT wallet_grosz FROM users WHERE id='{g}'), 'session')")
s, v = void(d2["deposit_id"], a1)
check("отмена после списания → 200: refund 12, unrecovered 38, coins_taken 120",
      s == 200 and near(v.get("refund_pln"), 12) and near(v.get("unrecovered_pln"), 38) and v.get("coins_taken") == 120, f"{s} {v}")
check("кошелёк = исходный (долга нет), монеты = 0 (не ниже нуля)", wallet() == w0 and coins() == 0, f"{wallet()}/{w0} {coins()}")
row = sql(f"SELECT refund_grosz, coins_taken FROM deposits WHERE id='{d2['deposit_id']}'")
check("строка: refund_grosz=1200, coins_taken=120", row == "1200|120", row)
sql(f"UPDATE users SET coins_balance = {c0} WHERE id='{g}'")

# ── C: права ──
d3 = deposit(20)
s, v = void(d3["deposit_id"], a2)
check("чужой депозит другим админом → 403 not_yours", s == 403 and v.get("code") == "not_yours", f"{s} {v.get('code')} {v.get('message')}")
s, v = void(d3["deposit_id"], ot)
check("чужой депозит владельцем → 200, вернулось 20", s == 200 and near(v.get("refund_pln"), 20), f"{s} {v}")
d4 = deposit(20)
sql(f"UPDATE deposits SET created_at = created_at - interval '2 days' WHERE id='{d4['deposit_id']}'")
s, v = void(d4["deposit_id"], a1)
check("свой, но позавчерашний → 403 too_old", s == 403 and v.get("code") == "too_old", f"{s} {v.get('code')}")
s, v = void(d4["deposit_id"], ot)
check("позавчерашний владельцем → 200", s == 200, f"{s} {v}")
s, v = void(d4["deposit_id"], gt)
check("гость → 403", s == 403, f"{s} {v.get('code')}")
s, v = void("00000000-0000-0000-0000-000000000000", a1)
check("неизвестный id → 404 deposit_not_found", s == 404 and v.get("code") == "deposit_not_found", f"{s} {v.get('code')}")
check("после C кошелёк и монеты исходные", wallet() == w0 and coins() == c0, f"{wallet()}/{w0} {coins()}/{c0}")

# ── D: выручка и отчёты ──
s, ov0 = api("GET", "/admin/overview", None, a1)
s, rm0 = api("GET", "/admin/reports/money?preset=today", None, ot)
s, sh0 = api("GET", "/admin/stats/shift", None, ot)
d5 = deposit(30)
s, ov1 = api("GET", "/admin/overview", None, a1)
check("overview: выручка дня +30 после депозита", near(ov1["revenue_today_pln"] - ov0["revenue_today_pln"], 30), f"{ov0['revenue_today_pln']} → {ov1['revenue_today_pln']}")
s, v = void(d5["deposit_id"], a1)
s, ov2 = api("GET", "/admin/overview", None, a1)
s, rm2 = api("GET", "/admin/reports/money?preset=today", None, ot)
s, sh2 = api("GET", "/admin/stats/shift", None, ot)
check("overview: после отмены выручка дня вернулась", near(ov2["revenue_today_pln"], ov0["revenue_today_pln"]), f"{ov2['revenue_today_pln']} vs {ov0['revenue_today_pln']}")
check("reports/money: пополнения за сегодня не изменились", near(rm2["totals"]["deposits_pln"], rm0["totals"]["deposits_pln"]) and rm2["totals"]["deposits"] == rm0["totals"]["deposits"],
      f"{rm0['totals']['deposits_pln']}/{rm0['totals']['deposits']} → {rm2['totals']['deposits_pln']}/{rm2['totals']['deposits']}")
check("stats/shift: выручка смены не изменилась", near(sh2["revenue"]["deposits_pln"], sh0["revenue"]["deposits_pln"]), f"{sh0['revenue']['deposits_pln']} → {sh2['revenue']['deposits_pln']}")
# Ж3-и2: KPI = отчёт «Деньги» за клубные сутки (пополнения + товары + пакеты, без отменённых)
check(f"overview.revenue_today = итог отчёта «Деньги» за сегодня ({rm2['totals']['revenue_pln']})", near(ov2["revenue_today_pln"], rm2["totals"]["revenue_pln"]), f"{ov2['revenue_today_pln']}")
loss = rm2.get("losses", {})
check("losses: отыграно по ошибочным депозитам ≥ 38 zł, отмен ≥ 5", float(loss.get("unrecovered_pln", 0)) >= 38 and int(loss.get("deposit_voids", 0)) >= 5, f"{loss}")
s, cr = api("GET", "/admin/reports/coins?preset=today", None, ot)
# начало клубных суток — по часам клуба и report_hour, как у сервера: сессия psql стенда живёт в UTC,
# и прежнее «date_trunc('day', now()) + 8 ч» давало 10:00 по Варшаве (трек З, З2-и1)
rh = "make_interval(hours => COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')::int)"
dbc = sql(f"SELECT COALESCE(SUM(coins_granted + bonus_coins - coins_taken),0) FROM deposits WHERE created_at >= "
          f"((((now() AT TIME ZONE 'Europe/Warsaw') - {rh})::date + {rh}) AT TIME ZONE 'Europe/Warsaw')")
check("reports/coins: монеты за депозиты = выдано − забрано при отмене", s == 200 and int(cr["totals"]["deposits"]) == int(float(dbc)), f"{cr.get('totals', {}).get('deposits')} vs {dbc}")
s, au = api("GET", "/admin/audit", None, a1)
kinds = [x.get("kind") for x in au.get("items", [])]
check("журнал: есть строка deposit_void", "deposit_void" in kinds, f"{kinds[:8]}")
dv = [x for x in au.get("items", []) if x.get("kind") == "deposit_void"][0]
check("текст строки: «отмена: … снято … отыграно …»", "отмена" in dv.get("text", "") and "снято с кошелька" in dv.get("text", ""), dv.get("text"))
s, card = api("GET", f"/admin/users/{g}", None, a1)
cd = [x for x in card.get("deposits", []) if x["id"] == d1["deposit_id"]]
check("карточка гостя: депозит с voided_at и refund_grosz", bool(cd) and cd[0].get("voided_at") and cd[0].get("refund_grosz") == 5000, f"{cd[0] if cd else card.get('deposits')}")

# ── E: дневной лимит админа отменённым не съедается ──
# живые депозиты admin1 за клубные сутки (Ж3: от report_hour, а не с полуночи) — лимит ставим на 50 zł выше
used = float(sql(f"SELECT COALESCE(SUM(amount_pln),0) FROM deposits WHERE created_by=(SELECT id FROM users WHERE nickname='admin1') AND voided_at IS NULL AND created_at >= '{ov2['day_from']}'"))
cap = int(used) + 50
api("PUT", "/admin/settings", {"admin_day_deposit_cap_pln": cap}, ot)
s, d6 = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 40, "method": "cash"}, a1)
check(f"под лимит {cap} (живых {used}) → 201", s == 201, f"{s} {d6.get('code')} {d6.get('message')}")
s, d7 = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 20, "method": "cash"}, a1)
check("следующие 20 zł сверх лимита → 403 day_cap", s == 403 and d7.get("code") == "day_cap", f"{s} {d7.get('code')}")
if s == 201: void(d7["deposit_id"], ot)
if "deposit_id" in d6: void(d6["deposit_id"], a1)
s, d8 = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 20, "method": "cash"}, a1)
check("после отмены лимит свободен → 201", s == 201, f"{s} {d8.get('code')} {d8.get('message')}")
if s == 201: void(d8["deposit_id"], a1)
sql("DELETE FROM settings WHERE key='admin_day_deposit_cap_pln'")
check("итог: кошелёк и монеты Guest27 исходные", wallet() == w0 and coins() == c0, f"{wallet()}/{w0} {coins()}/{c0}")
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
