#!/usr/bin/env python3
"""Ж0-и1/и2 (Ж-Р1, Ж-Р2): инвариант цели и дневной потолок компенсаций по инцидентам.
Живой e2e против стенда: admin2/Admin12345, boss/Boss12345, гости GuestNN/Guest12345.
Идемпотентен: потолки возвращаются в 0, тестовые деньги/минуты снимаются."""
import os, json, subprocess, sys, time, urllib.request, urllib.error

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr)
    return r.stdout.strip()
def api(m, p, b=None, t=None):
    req = urllib.request.Request(API + p, data=json.dumps(b).encode() if b is not None else None, method=m)
    req.add_header("Content-Type", "application/json")
    if t: req.add_header("Authorization", "Bearer " + t)
    try:
        with urllib.request.urlopen(req) as r: return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try: return e.code, json.loads(e.read() or b"{}")
        except Exception: return e.code, {}
    finally: time.sleep(0.13)  # рейт-лимитер ~10 rps
def login(n, p): return api("POST", "/auth/login", {"login": n, "password": p})[1]["access_token"]
ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    ok += cond; fail += (not cond)
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))

at = login("admin2", "Admin12345"); ot = login("boss", "Boss12345")
me = sql("SELECT id FROM users WHERE nickname='admin2'"); boss = sql("SELECT id FROM users WHERE nickname='boss'")
admin1 = sql("SELECT id FROM users WHERE nickname='admin1'")
g24 = sql("SELECT id FROM users WHERE nickname='Guest24'"); g25 = sql("SELECT id FROM users WHERE nickname='Guest25'")
# чистый старт: потолки выкл, чужих компенсаций за сегодня от admin2 нет
sql("DELETE FROM settings WHERE key IN ('admin_day_comp_cap_pln','admin_day_comp_cap_min')")
sql(f"DELETE FROM incidents WHERE created_by='{me}' OR resolved_by='{me}' OR user_id IN ('{me}','{boss}','{admin1}')")
w0 = {u: sql(f"SELECT wallet_grosz||'|'||coin_minutes FROM users WHERE id='{u}'") for u in (me, boss, admin1, g24, g25)}

# ── и1: инвариант цели ──
s, d = api("POST", "/admin/incidents", {"user_id": me, "kind": "other", "comp_pln": 100}, at)
check("инцидент на себя → 403 cannot_touch_self", s == 403 and d.get("code") == "cannot_touch_self", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": boss, "kind": "power", "comp_pln": 9999}, at)
check("инцидент на владельца → 403 cannot_touch_staff", s == 403 and d.get("code") == "cannot_touch_staff", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": admin1, "kind": "other", "comp_minutes": 30}, at)
check("инцидент на другого админа → 403 cannot_touch_staff", s == 403 and d.get("code") == "cannot_touch_staff", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": me, "kind": "other", "note": "без компенсации"}, at)
check("запись на себя даже без компенсации → 403", s == 403, f"{s} {d.get('code')}")
check("кошельки персонала не тронуты", all(sql(f"SELECT wallet_grosz||'|'||coin_minutes FROM users WHERE id='{u}'") == w0[u] for u in (me, boss, admin1)))
s, d = api("POST", "/admin/incidents", {"user_id": g24, "kind": "other", "comp_pln": 5, "note": "e2e"}, at)
check("инцидент на гостя с компенсацией → 201", s == 201 and d.get("comp_pln") == 5, f"{s} {d.get('code')}")
inc_g24 = d.get("id")
# старая запись с персоналом в user_id (до фикса) — компенсировать нельзя
club = sql("SELECT id FROM clubs LIMIT 1")
old = sql(f"INSERT INTO incidents (club_id, user_id, kind, started_at, ended_at, minutes, created_by, created_at) VALUES ('{club}','{boss}','other',NOW(),NOW(),0,'{me}',NOW()) RETURNING id").splitlines()[0].strip()
s, d = api("POST", f"/admin/incidents/{old}/compensate", {"comp_pln": 50}, at)
check("compensate по старой записи на владельца → 403", s == 403 and d.get("code") == "cannot_touch_staff", f"{s} {d.get('code')}")
sql(f"DELETE FROM incidents WHERE id='{old}'")
# персонал за ПК: запись по ПК создаётся БЕЗ гостя
pcs = api("GET", "/admin/computers", None, at)[1]["computers"]; pc = [p for p in pcs if p["name"] == "PC-09"][0]
mt = login("admin2", "Admin12345")  # тот же аккаунт как «гость» за ПК
s, d = api("POST", "/me/sessions/start", {"computer_id": pc["id"]}, mt)
if s != 201:
    sql(f"UPDATE users SET wallet_grosz=5000 WHERE id='{me}'"); s, d = api("POST", "/me/sessions/start", {"computer_id": pc["id"]}, mt)
check("админ сел за PC-09 как гость (подготовка)", s == 201, f"{s} {d.get('code')} {d.get('message')}")
s, d = api("POST", "/admin/incidents", {"computer_id": pc["id"], "kind": "pc_hang", "minutes": 3}, at)
check("инцидент по ПК с персоналом за ним — без гостя", s == 201 and d.get("user_id") is None, f"{s} user_id={d.get('user_id')}")
s2, d2 = api("POST", f"/admin/incidents/{d.get('id')}/compensate", {"comp_pln": 10}, at)
check("компенсация по нему → 409 incident_no_guest", s2 == 409 and d2.get("code") == "incident_no_guest", f"{s2} {d2.get('code')}")
sid = sql(f"SELECT id FROM sessions WHERE user_id='{me}' AND status='active'")
if sid: api("POST", f"/admin/sessions/{sid}/end", {}, ot)
sql(f"UPDATE users SET wallet_grosz=0, coin_minutes=0 WHERE id='{me}'")
sql(f"DELETE FROM incidents WHERE created_by='{me}' AND user_id IS NULL")

# ── и2: потолки ──
s, d = api("PUT", "/admin/settings", {"admin_day_comp_cap_pln": 50, "admin_day_comp_cap_min": 60}, ot)
check("владелец ставит потолки 50 zł / 60 мин", s == 200 and d["settings"].get("admin_day_comp_cap_pln") == 50 and d["settings"].get("admin_day_comp_cap_min") == 60, f"{s}")
s, d = api("GET", "/admin/settings", None, ot)
check("GET /admin/settings отдаёт оба ключа", d["settings"].get("admin_day_comp_cap_pln") == 50 and d["settings"].get("admin_day_comp_cap_min") == 60)
sql(f"DELETE FROM incidents WHERE resolved_by='{me}' AND compensated_at >= date_trunc('day', now())")
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_pln": 40}, at)
check("40 zł при потолке 50 → 201", s == 201, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_pln": 20}, at)
check("ещё 20 zł (итого 60 > 50) → 403 day_cap", s == 403 and d.get("code") == "day_cap", f"{s} {d.get('code')} {d.get('message')}")
open_cnt = sql(f"SELECT count(*) FROM incidents WHERE created_by='{me}' AND resolved_at IS NULL")
check("отказ по потолку не оставил открытой записи", open_cnt == "0", f"открытых {open_cnt}")
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_pln": 10}, at)
check("10 zł (итого ровно 50) → 201", s == 201, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_minutes": 50}, at)
check("50 мин при потолке 60 → 201 (деньги не считаются)", s == 201, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_minutes": 20}, at)
check("ещё 20 мин (70 > 60) → 403 day_cap", s == 403 and d.get("code") == "day_cap", f"{s} {d.get('code')}")
# compensate-ручка тоже под потолком
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "note": "открытая"}, at)
oid = d.get("id")
s, d = api("POST", f"/admin/incidents/{oid}/compensate", {"comp_pln": 1}, at)
check("compensate сверх потолка → 403 day_cap", s == 403 and d.get("code") == "day_cap", f"{s} {d.get('code')}")
s, d = api("POST", f"/admin/incidents/{oid}/compensate", {"comp_pln": 5000}, ot)
check("владелец компенсирует 5000 zł без потолка → 200", s == 200 and d.get("comp_pln") == 5000, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_pln": 5000}, ot)
check("владелец создаёт с 5000 zł → 201", s == 201, f"{s} {d.get('code')}")
# потолки выкл → админ снова может
api("PUT", "/admin/settings", {"admin_day_comp_cap_pln": 0, "admin_day_comp_cap_min": 0}, ot)
s, d = api("POST", "/admin/incidents", {"user_id": g25, "kind": "other", "comp_pln": 200}, at)
check("потолки 0 → 200 zł проходят", s == 201, f"{s} {d.get('code')}")

# уборка: тестовые деньги/минуты и записи
sql(f"DELETE FROM incidents WHERE (created_by='{me}' OR resolved_by IN ('{me}','{boss}')) AND created_at >= now() - interval '10 minutes'")
for u in (g24, g25):
    w, m = w0[u].split("|"); sql(f"UPDATE users SET wallet_grosz={w}, coin_minutes={m} WHERE id='{u}'")
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
