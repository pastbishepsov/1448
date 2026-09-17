#!/usr/bin/env python3
"""Ж0-и5/и7: тикет живого канала и валидация депозита. Живой e2e против стенда."""
import os, json, subprocess, sys, time, urllib.request, urllib.error, socket, base64, os

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
    finally: time.sleep(0.13)
def login(n, p): return api("POST", "/auth/login", {"login": n, "password": p})[1]["access_token"]
def ws_status(query):
    """Сырой HTTP-апгрейд: только код ответа (101 = сокет открыт)."""
    s = socket.create_connection((HOST, PORT), timeout=5)
    key = base64.b64encode(os.urandom(16)).decode()
    s.sendall((f"GET /api/v1/ws/admin?{query} HTTP/1.1\r\nHost: {HOST}:{PORT}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
               f"Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
    head = s.recv(4096).decode(errors="replace"); s.close(); time.sleep(0.13)
    return int(head.split(" ")[1]) if head.startswith("HTTP/1.1 ") else -1
ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    ok += cond; fail += (not cond)
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))

at = login("admin1", "Admin12345"); gt = login("Guest26", "Guest12345")
# ── и5: тикет ──
s, d = api("POST", "/admin/ws-ticket", {}, at)
check("POST /admin/ws-ticket админу → 200 с тикетом", s == 200 and len(d.get("ticket", "")) >= 20 and d.get("expires_in") == 30, f"{s} {d}")
tk = d.get("ticket", "")
s2, d2 = api("POST", "/admin/ws-ticket", {}, gt)
check("гостю тикет не выдаётся → 403", s2 == 403, f"{s2} {d2.get('code')}")
check("сокет по тикету → 101", ws_status("ticket=" + tk) == 101)
check("тот же тикет второй раз → 401", ws_status("ticket=" + tk) == 401)
check("access-токен в query больше не принимается → 401", ws_status("token=" + at) == 401)
check("без тикета → 401", ws_status("") == 401)
if SRV_LOG and os.path.exists(SRV_LOG):
    log = open(SRV_LOG, encoding="utf-8", errors="replace").read()
    ws_lines = [l for l in log.splitlines() if "/ws/admin" in l]
    check(f"строк /ws/admin в логе сервера: {len(ws_lines)} (логгер молчит)", len(ws_lines) == 0)
    check("access-токен админа в логе не встречается", at[:40] not in log)
else:
    print("skip проверки лога сервера — задай E2E_SRV_LOG=путь к логу сервера")
# ── и7: депозит ──
g26 = sql("SELECT id FROM users WHERE nickname='Guest26'")
w0 = int(sql(f"SELECT wallet_grosz FROM users WHERE id='{g26}'"))
s, d = api("POST", f"/admin/users/{g26}/deposit", {"amount_pln": 0.001}, at)
check("депозит 0,001 zł → 400 bad_amount (было 500)", s == 400 and d.get("code") == "bad_amount", f"{s} {d.get('code')}")
s, d = api("POST", f"/admin/users/{g26}/deposit", {"amount_pln": 0.004}, at)
check("депозит 0,004 zł → 400 (округляется в 0 грошей)", s == 400 and d.get("code") == "bad_amount", f"{s} {d.get('code')}")
s, d = api("POST", f"/admin/users/{g26}/deposit", {"amount_pln": 10.005}, at)
w1 = int(sql(f"SELECT wallet_grosz FROM users WHERE id='{g26}'"))
rec = sql(f"SELECT amount_pln FROM deposits WHERE user_id='{g26}' ORDER BY created_at DESC LIMIT 1")
check(f"депозит 10,005 → 201, запись {rec} zł, кошелёк +{w1-w0} грошей — совпадают", s == 201 and w1 - w0 == 1001 and rec == "10.01" and d.get("amount_pln") == 10.01, f"{s} {d.get('amount_pln')}")
s, d = api("POST", f"/admin/users/{g26}/deposit", {"amount_pln": 0}, at)
check("депозит 0 → 400", s == 400, f"{s} {d.get('code')}")
sql(f"UPDATE users SET wallet_grosz={w0} WHERE id='{g26}'")
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
