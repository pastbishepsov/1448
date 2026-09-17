#!/usr/bin/env python3
"""Ж2 (Ж-Р3): минуты за монеты привязаны к зоне. Живой e2e против стенда (критерий Ж2 в ADMIN.md):
30 мин Standard не тратятся на VIP (платит кошелёк), тратятся на Standard; старый coin_minutes
тратится везде и после зонных; отмена выдачи после 10 из 30 отыгранных минут возвращает 2/3 монет;
компенсация с ПК получает его зону, без ПК — пул; порог старта считает только минуты своей зоны;
/me/sessions, /me/packages, карточка и отчёт «Монеты» отдают запас по зонам.
Расчёт минут — через finishSession (тот же settleSessionMinutes, что у тика) + один живой тик.
Идемпотентен: Guest29 приводится к нулю в начале и в конце."""
import os, json, subprocess, sys, time, urllib.request, urllib.error

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

a1 = login("admin1", "Admin12345"); ot = login("boss", "Boss12345"); gt = login("Guest29", "Guest12345")
g = sql("SELECT id FROM users WHERE nickname='Guest29'")
club = sql("SELECT id FROM clubs LIMIT 1")
zones = {z["name"]: z for z in api("GET", "/admin/zones", None, a1)[1]["zones"]}
STD, VIP = zones["Standard"], zones["VIP"]
pcs = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
PC_STD, PC_VIP = pcs["PC-08"], pcs["VIP-02"]
wallet = lambda: int(sql(f"SELECT wallet_grosz FROM users WHERE id='{g}'"))
pool = lambda: int(sql(f"SELECT coin_minutes FROM users WHERE id='{g}'"))
coins = lambda: int(sql(f"SELECT coins_balance FROM users WHERE id='{g}'"))
grants = lambda: sql(f"SELECT COALESCE(string_agg(zone_name || ':' || minutes_left, ',' ORDER BY created_at), '') FROM minute_grants WHERE user_id='{g}'")

def reset():
    for sid in sql(f"SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='{g}' AND status='active'").split():
        api("POST", f"/admin/sessions/{sid}/end", {}, a1)
    sql(f"DELETE FROM minute_grants WHERE user_id='{g}'")
    sql(f"UPDATE users SET coin_minutes=0, coins_balance=5000, wallet_grosz=0 WHERE id='{g}'")
    sql(f"DELETE FROM wallet_transactions WHERE user_id='{g}'")  # чистый журнал: инвариант «сумма = баланс» держим сами
    sql(f"UPDATE incidents SET user_id=NULL WHERE user_id='{g}'")
reset()
sql("DELETE FROM settings WHERE key IN ('booking_min_level')")

def seat(pc, back_min):
    """Посадить Guest29 и снять окно [Готов!]; back_min > 0 — сдвинуть старт так, чтобы финальный
    расчёт (ceil прошедшего) насчитал ровно back_min минут: (back_min − 1) мин 10 с назад."""
    s, d = api("POST", f"/admin/computers/{pc['id']}/session", {"guest": "Guest29"}, a1)
    assert s == 201, (s, d)
    sid = d["session_id"]
    shift = f"now() - interval '{back_min - 1} minutes 10 seconds'" if back_min > 0 else "now()"
    sql(f"UPDATE sessions SET started_at = {shift}, ready_at = {shift} WHERE id='{sid}'")
    return sid
def end(sid):
    s, d = api("POST", f"/admin/sessions/{sid}/end", {}, a1)
    assert s == 200, (s, d)
    return sql(f"SELECT billed_minutes || '|' || coin_minutes_used || '|' || money_minutes || '|' || charged_grosz FROM sessions WHERE id='{sid}'")

# ── A: выдача — зона обязательна и означает зону ──
s, d = api("POST", f"/admin/users/{g}/redeem", {"zone_id": "", "minutes": 30}, a1)
check("выдача без зоны и без сессии → 400 zone_required", s == 400 and d.get("code") == "zone_required", f"{s} {d.get('code')} {d.get('message')}")
s, r1 = api("POST", f"/admin/users/{g}/redeem", {"zone_id": STD["id"], "minutes": 30}, a1)
check("выдача 30 мин Standard → 200, зона в ответе", s == 200 and r1.get("zone") == "Standard", f"{s} {r1}")
check("грант: Standard:30, source redeem, ref → выдача; пул без зоны не тронут", grants() == "Standard:30" and pool() == 0
      and sql(f"SELECT source || '|' || (ref_id = '{r1['id']}')::text FROM minute_grants WHERE user_id='{g}'") == "redeem|true", f"{grants()} pool={pool()}")
coins_after_r1 = coins()
check(f"монеты списаны: {r1.get('coins')} (30 мин × 20 zł/ч × 20 монет/zł = 200)", r1.get("coins") == 200 and coins_after_r1 == 5000 - 200, f"{coins_after_r1}")
check("в ответе zone_minutes по зонам", any(z.get("zone") == "Standard" and z.get("minutes") == 30 for z in r1.get("zone_minutes", [])), f"{r1.get('zone_minutes')}")
s, pk = api("GET", "/me/packages", None, gt)
check("/me/packages: minutes_by_zone = [Standard 30]", [(z["zone"], z["minutes"]) for z in pk.get("minutes_by_zone", [])] == [("Standard", 30)], f"{pk.get('minutes_by_zone')}")

# ── B: Standard-минуты не тратятся на VIP — платит кошелёк ──
api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 20, "method": "cash"}, a1)  # 20 zł на кошельке
sid = seat(PC_VIP, 10)
s, ss = api("GET", "/me/sessions", None, gt)
check("/me/sessions на VIP: zone_minutes 0 (Standard тут не годны), zone=VIP", ss.get("zone_minutes") == 0 and ss.get("zone") == "VIP", f"{ss.get('zone_minutes')} {ss.get('zone')}")
row = end(sid)
check(f"10 мин на VIP: грант Standard цел, платит кошелёк — сессия {row} (ожидалось 10|0|10|500)", row == "10|0|10|500" and grants() == "Standard:30" and wallet() == 1500, f"grants={grants()} wallet={wallet()}")

# ── C: на Standard — тратятся зонные минуты ──
sid = seat(PC_STD, 10)
s, ss = api("GET", "/me/sessions", None, gt)
check("/me/sessions на Standard: zone_minutes 30, zone=Standard", ss.get("zone_minutes") == 30 and ss.get("zone") == "Standard", f"{ss.get('zone_minutes')} {ss.get('zone')}")
row = end(sid)
check(f"10 мин на Standard: грант 30→20, кошелёк не тронут — сессия {row} (ожидалось 10|10|0|0)", row == "10|10|0|0" and grants() == "Standard:20" and wallet() == 1500, f"grants={grants()} wallet={wallet()}")

# ── D: старый пул без зоны тратится везде и ПОСЛЕ зонных ──
sql(f"UPDATE users SET coin_minutes=12 WHERE id='{g}'")
sid = seat(PC_VIP, 5)
row = end(sid)
check(f"5 мин на VIP при пуле 12: пул 12→7, грант Standard цел, денег не взято — {row}", row == "5|5|0|0" and pool() == 7 and grants() == "Standard:20" and wallet() == 1500, f"pool={pool()} grants={grants()}")
sid = seat(PC_STD, 25)
row = end(sid)
check(f"25 мин на Standard: сперва грант 20→0, потом пул 7→2, кошелёк цел — {row}", row == "25|25|0|0" and pool() == 2 and grants() == "Standard:0" and wallet() == 1500, f"pool={pool()} grants={grants()}")

# ── E: отмена выдачи после 10 из 30 отыгранных — 2/3 монет ──
s, r2 = api("POST", f"/admin/users/{g}/redeem", {"zone_id": STD["id"], "minutes": 30}, a1)
sid = seat(PC_STD, 10)
row = end(sid)
check(f"10 мин на новой выдаче: грант2 30→20 — {row}", row == "10|10|0|0" and grants() == "Standard:0,Standard:20", grants())
c_before = coins()  # после сессии: игра сама начисляет монеты за минуты, их в возврат не считаем
s, v = api("POST", f"/admin/coin-redemptions/{r2['id']}/void", {}, a1)
check("отмена выдачи → 200: забрано 20 мин, вернулось 133 монет (200 × 20/30)", s == 200 and v.get("minutes_taken") == 20 and v.get("coins_returned") == 133, f"{s} {v}")
check("грант2 обнулён, монеты +133", grants() == "Standard:0,Standard:0" and coins() == c_before + 133, f"grants={grants()} coins={coins()} было {c_before}")
s, v2 = api("POST", f"/admin/coin-redemptions/{r2['id']}/void", {}, a1)
check("повтор отмены → 409", s == 409, f"{s} {v2.get('code')}")

# ── E2: выдача без зоны при активной сессии — зона ПК ──
sql(f"UPDATE users SET coin_minutes=2 WHERE id='{g}'")
sid = seat(PC_VIP, 0)
s, r3 = api("POST", f"/admin/users/{g}/redeem", {"zone_id": "", "minutes": 10}, a1)
check("выдача без zone_id при сессии на VIP → зона VIP", s == 200 and r3.get("zone") == "VIP" and r3.get("coins") == 100, f"{s} {r3.get('zone')} {r3.get('coins')}")
s, card = api("GET", f"/admin/users/{g}", None, a1)
check("карточка: active_session = VIP-02 / VIP, reserve содержит VIP 10 и пул 2",
      (card.get("active_session") or {}).get("zone") == "VIP" and (card.get("active_session") or {}).get("computer") == "VIP-02"
      and any(r.get("zone") == "VIP" and r.get("minutes") == 10 for r in card.get("reserve", []))
      and any(r.get("from_pool") and r.get("minutes") == 2 for r in card.get("reserve", [])), f"{card.get('active_session')} {card.get('reserve')}")
# живой тик: сессия с 12+ минутами назад — биллинг (floor прошедшего) снимет грант VIP 10 + пул 2,
# остальное — кошельком; тик раз в 30 с, поэтому минут может набежать 12 или 13
sql(f"UPDATE sessions SET started_at = now() - interval '12 minutes 10 seconds', ready_at = now() - interval '12 minutes 10 seconds' WHERE id='{sid}'")
deadline = time.time() + 45
while time.time() < deadline and int(sql(f"SELECT billed_minutes FROM sessions WHERE id='{sid}'")) < 12: time.sleep(2)
b, cm, mm, ch = (int(x) for x in sql(f"SELECT billed_minutes || '|' || coin_minutes_used || '|' || money_minutes || '|' || charged_grosz FROM sessions WHERE id='{sid}'").split("|"))
check(f"живой тик биллинга: {b} мин = грант VIP 10 + пул 2 ({cm}) + кошельком {mm} мин ({ch} гр)", b >= 12 and cm == 12 and mm == b - 12 and ch == mm * 50
      and pool() == 0 and sql(f"SELECT minutes_left FROM minute_grants WHERE ref_id='{r3['id']}'") == "0", f"pool={pool()}")
w_before_end = wallet()
api("POST", f"/admin/sessions/{sid}/end", {}, a1)

# ── F: компенсация — с ПК в зону ПК, без ПК — в пул ──
s, inc = api("POST", "/admin/incidents", {"computer_id": PC_VIP["id"], "user_id": g, "kind": "pc_hang", "comp_minutes": 15, "note": "e2e Ж2"}, a1)
check("компенсация 15 мин с ПК VIP-02 → грант VIP:15, пул не тронут", s == 201 and grants().endswith("VIP:15") and pool() == 0
      and sql(f"SELECT source FROM minute_grants WHERE ref_id='{inc.get('id', inc.get('incident_id', ''))}'") == "compensation", f"{s} {inc} grants={grants()}")
s, n = api("GET", "/me/notifications?client=app", None, gt)
comp = [x for x in n.get("items", []) if x.get("type") == "incident_compensation"]
check("гостю: incident_compensation с zone=VIP", bool(comp) and comp[-1]["payload"].get("zone") == "VIP", f"{comp[-1] if comp else n}")
s, inc2 = api("POST", "/admin/incidents", {"user_id": g, "kind": "other", "comp_minutes": 5, "note": "e2e Ж2 без ПК"}, a1)
check("компенсация 5 мин без ПК → в пул без зоны", s == 201 and pool() == 5, f"{s} pool={pool()}")

# ── G: отчёт «Монеты» и карточка ──
s, rep = api("GET", "/admin/reports/coins?preset=today", None, ot)
owed = rep.get("minutes_owed", {})
db_pool = int(sql("SELECT COALESCE(SUM(coin_minutes),0) FROM users")); db_zoned = int(sql("SELECT COALESCE(SUM(minutes_left),0) FROM minute_grants"))
check(f"reports/coins.minutes_owed: total {owed.get('total')} = пул {db_pool} + зонные {db_zoned}, по зонам есть VIP 15",
      owed.get("total") == db_pool + db_zoned and owed.get("pool") == db_pool and owed.get("zoned") == db_zoned
      and any(z.get("zone") == "VIP" and z.get("minutes") >= 15 for z in owed.get("by_zone", [])), f"{owed}")
s, pk = api("GET", "/me/packages", None, gt)
check("/me/packages: minutes_by_zone = VIP 15 + пул 5", sorted((z.get("zone", ""), z["minutes"]) for z in pk.get("minutes_by_zone", []) if z["minutes"] > 0) == [("", 5), ("VIP", 15)], f"{pk.get('minutes_by_zone')}")

# ── H: порог старта считает минуты только своей зоны ──
sql(f"UPDATE users SET coin_minutes=0 WHERE id='{g}'")
sql(f"UPDATE users SET wallet_grosz=0 WHERE id='{g}'"); sql(f"DELETE FROM wallet_transactions WHERE user_id='{g}'")
s, d = api("POST", f"/admin/computers/{PC_STD['id']}/session", {"guest": "Guest29"}, a1)
check("посадка на Standard с 15 мин VIP и пустым кошельком → 409 wallet_low", s == 409 and d.get("code") == "wallet_low", f"{s} {d.get('code')} {d.get('message')}")
s, d = api("POST", f"/admin/computers/{PC_VIP['id']}/session", {"guest": "Guest29"}, a1)
check("посадка на VIP с 15 мин VIP → 201 (порог 15 мин закрыт зонными минутами)", s == 201, f"{s} {d.get('code')} {d.get('message')}")
if s == 201: api("POST", f"/admin/sessions/{d['session_id']}/seat-cancel", {}, a1)

reset()
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
