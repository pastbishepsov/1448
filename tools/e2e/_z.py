"""Помощники живых проверок трека З (OWNER.md): на них e2e_z<N>_*.py пишется в 30–60 строк.

    from _z import *
    ensure_wallet("Guest31", 30)                         # докладывает недостающее через API
    sid = seat("Guest31", "PC-01", minutes_ago=90)       # посадка от admin1 и сдвиг начала назад
    s, d = end(sid)
    check("сессия закрыта", s == 200, J(d))
    done()                                               # уборка из at_end() и «Итого: ok=… fail=…»

Всё, что проверка меняет в общем стенде, возвращается в конце: set_settings(), hire_temp() и
agent_start() сами регистрируют уборку; своё — at_end(fn). Уборка идёт и при падении скрипта.
Стенд задаётся окружением (_env.py, README): адрес API, строка базы, куда писать логи."""
import atexit, base64, json, os, re, socket, struct, subprocess, sys, time, urllib.error, urllib.parse, urllib.request

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log

__all__ = ["API", "BASE", "DB", "SRV_LOG", "TZ", "GUEST_PASS", "STAFF_PASS", "sql", "rows", "uid", "login", "tok",
           "tokens", "password_of", "jwt_claims", "api", "J", "check", "near", "section", "at_end", "done", "club_day", "at_club", "settings",
           "set_settings", "restore_settings", "wallet", "topup", "ensure_wallet", "pcs", "seat", "end", "end_all", "free_pcs",
           "hire_temp", "dismiss", "drawer_ready", "audit", "ws_admin", "ws_read", "agent_start", "agent_stop",
           "drop_good", "stock_to"]

TZ = "Europe/Warsaw"  # часовой пояс клуба; сессия psql может жить в UTC — время клуба только через AT TIME ZONE
GUEST_PASS, STAFF_PASS = "Guest12345", "Admin12345"
PASSWORDS = {"boss": "Boss12345", "admin1": STAFF_PASS, "admin2": STAFF_PASS}
PAUSE = 0.12  # лимитер сервера — 10 запросов в секунду с одного IP

def password_of(nick):
    """Пароль стенда: персонал сида и временные z<N>… — STAFF_PASS (кроме boss), гости — GUEST_PASS."""
    return PASSWORDS.get(nick) or (STAFF_PASS if re.match(r"z\d", nick) else GUEST_PASS)


# ── база ─────────────────────────────────────────────────────────────────────────────────────────
def rows(q):
    """Все строки результата psql (-tA, поля через «|»). Ошибка SQL — выход с текстом запроса."""
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode:
        raise SystemExit("SQL: " + r.stderr.strip() + "\n" + q)
    return [x for x in r.stdout.splitlines() if x]

def sql(q):
    """Первая строка результата или ''."""
    r = rows(q)
    return r[0] if r else ""

def uid(nick):
    return sql(f"SELECT id FROM users WHERE nickname='{nick}'")


# ── API ──────────────────────────────────────────────────────────────────────────────────────────
def _call(method, path, body=None, token=None, raw=False):
    req = urllib.request.Request(API + path, data=json.dumps(body).encode() if body is not None else None, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req) as r:
            status, data = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, data = e.code, e.read()
    finally:
        time.sleep(PAUSE)
    if raw:
        return status, data.decode(errors="replace")
    try:
        return status, json.loads(data or b"{}")
    except ValueError:
        return status, {"_text": data.decode(errors="replace")[:300]}

def login(nick, password=None, **extra):
    """POST /auth/login как есть: (status, ответ). Кэш не трогает — для проверок самого входа."""
    return _call("POST", "/auth/login", {"login": nick, "password": password or password_of(nick), **extra})

_cache = {}  # ник → {"access", "refresh", "exp"}

def jwt_claims(token):
    """Полезная нагрузка JWT без проверки подписи: {"sub", "role", "iat", "exp", …}; мусор — {}."""
    try:
        part = token.split(".")[1]
        return json.loads(base64.urlsafe_b64decode(part + "=" * (-len(part) % 4)))
    except Exception:
        return {}

def _exp(token):
    return jwt_claims(token).get("exp", 0)

def tokens(nick, password=None, fresh=False):
    """Пара токенов из кэша процесса; нет, истекает через минуту или fresh — вход заново.
    Входит как админка (client="admin"): такой вход открывает админу смену (Е3-и1, З3-и1) — поэтому
    кэш, а не вход на каждый вызов. Вход киоска или PWA (без client) — через login()."""
    t = _cache.get(nick)
    if fresh or not t or t["exp"] - time.time() < 60:
        s, d = login(nick, password, client="admin")
        if s != 200:
            raise SystemExit(f"вход {nick}: {s} {J(d)}")
        t = _cache[nick] = {"access": d["access_token"], "refresh": d["refresh_token"], "exp": _exp(d["access_token"])}
    return t

def tok(nick, fresh=False):
    return tokens(nick, fresh=fresh)["access"]

def api(method, path, body=None, who=None, token=None, raw=False):
    """(status, data). who — ник: токен из кэша, на 401 — один перевход. token — явный токен без
    перевхода (проверки отзыва). Ни того ни другого — без авторизации."""
    if token or not who:
        return _call(method, path, body, token, raw)
    s, d = _call(method, path, body, tok(who), raw)
    if s == 401:
        s, d = _call(method, path, body, tok(who, fresh=True), raw)
    return s, d


# ── итог ─────────────────────────────────────────────────────────────────────────────────────────
_n = {"ok": 0, "fail": 0}
_cleanups = []

def J(x, n=300):
    return json.dumps(x, ensure_ascii=False, default=str)[:n]

def check(name, cond, detail=""):
    cond = bool(cond)
    _n["ok" if cond else "fail"] += 1
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail not in ("", None) else ""))
    return cond

def near(a, b, eps=0.005):
    return abs(float(a) - float(b)) < eps

def section(title):
    print(f"\n── {title}")

def at_end(fn):
    """Уборка в конце (в обратном порядке), в том числе при падении скрипта."""
    _cleanups.append(fn)
    return fn

def _cleanup():
    while _cleanups:
        fn = _cleanups.pop()
        try:
            fn()
        except BaseException as e:  # уборка не должна прятать итог
            print(f"уборка: {fn.__name__ if hasattr(fn, '__name__') else fn}: {e!r}")

atexit.register(_cleanup)

def done():
    _cleanup()
    print(f"\nИтого: ok={_n['ok']} fail={_n['fail']}")
    sys.exit(1 if _n["fail"] else 0)


# ── клубное время ────────────────────────────────────────────────────────────────────────────────
def _report_hour():
    # З18-и4б: строки нет — сутки с полуночи, как у сервера (reportHourDef = 0)
    return int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')"))

def club_day(offset=0):
    """Клубные сутки 'ГГГГ-ММ-ДД' (от report_hour по часам клуба), как их считает сервер; offset — ±дни."""
    return sql(f"SELECT (((now() AT TIME ZONE '{TZ}') - interval '{_report_hour()} hours')::date + {offset})::text")

def at_club(day, hhmm):
    """SQL-выражение timestamptz «день day, чч:мм по часам клуба» — для сдвига created_at и т. п."""
    return f"(('{day}'::date + time '{hhmm}') AT TIME ZONE '{TZ}')"


# ── настройки ────────────────────────────────────────────────────────────────────────────────────
_orig_settings = {}

def settings():
    return api("GET", "/admin/settings", who="boss")[1]["settings"]

def set_settings(**kv):
    """PUT /admin/settings от владельца; строки settings этих ключей вернутся как были (at_end)."""
    fresh = [k for k in kv if k not in _orig_settings]
    for k in fresh:
        r = rows(f"SELECT value FROM settings WHERE key='{k}'")
        _orig_settings[k] = r[0] if r else None
    if fresh and len(_orig_settings) == len(fresh):
        at_end(restore_settings)
    return api("PUT", "/admin/settings", kv, "boss")

def restore_settings():
    """Вернуть настройки, тронутые set_settings (сама зовётся в конце; можно и раньше)."""
    for k, v in _orig_settings.items():
        if v is None:
            sql(f"DELETE FROM settings WHERE key='{k}'")
        else:
            sql(f"INSERT INTO settings (key, value) VALUES ('{k}', '{v}') ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value")
    _orig_settings.clear()


# ── деньги и посадки ─────────────────────────────────────────────────────────────────────────────
def wallet(nick):
    """Кошелёк гостя в грошах."""
    return int(sql(f"SELECT wallet_grosz FROM users WHERE nickname='{nick}'"))

def topup(nick, pln, method="card", who="boss"):
    """Пополнение через API (как у стойки): id депозита. Владелец — без дневных потолков админа."""
    s, d = api("POST", f"/admin/users/{uid(nick)}/deposit", {"amount_pln": pln, "method": method}, who)
    if s != 201:
        raise SystemExit(f"пополнение {nick} {pln} zł: {s} {J(d)}")
    return d["deposit_id"]

def ensure_wallet(nick, pln, method="card"):
    """Докладывает недостающее до pln zł (повторный прогон не копит деньги)."""
    need = round(pln - wallet(nick) / 100, 2)
    return topup(nick, need, method) if need > 0 else None

_pcs = {}

def pcs():
    """ПК стенда по имени: {"PC-01": {...}, …, "VIP-03": {...}}."""
    if not _pcs:
        _pcs.update({p["name"]: p for p in api("GET", "/admin/computers", who="admin1")[1]["computers"]})
    return _pcs

def seat(nick, pc="PC-01", who="admin1", minutes_ago=0, **extra):
    """Посадка через стойку → session_id; minutes_ago — сдвинуть начало (и готовность) назад SQL."""
    s, d = api("POST", f"/admin/computers/{pcs()[pc]['id']}/session", {"guest": nick, **extra}, who)
    if s != 201:
        raise SystemExit(f"посадка {nick} за {pc}: {s} {J(d)}")
    sid = d["session_id"]
    if minutes_ago:
        sql(f"UPDATE sessions SET started_at = now() - interval '{minutes_ago} minutes', "
            f"ready_at = now() - interval '{minutes_ago} minutes' WHERE id='{sid}'")
    return sid

def end(sid, who="admin1"):
    return api("POST", f"/admin/sessions/{sid}/end", {}, who)

def end_all(*nicks):
    """Завершить все живые сессии этих гостей (уборка) и освободить их ПК."""
    ids = ",".join(f"'{u}'" for u in map(uid, nicks) if u) or "NULL"
    for sid in rows(f"SELECT id FROM sessions WHERE status='active' AND user_id IN ({ids})"):
        end(sid)
    free_pcs()

def free_pcs():
    """ПК «занят/бронь» без живой сессии — снова свободны (хвосты упавших прогонов)."""
    sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') "
        "AND id NOT IN (SELECT computer_id FROM sessions WHERE status='active')")


# ── склад ────────────────────────────────────────────────────────────────────────────────────────
def drop_good(gid):
    """Временная позиция стенда — SQL-ом вместе с движениями, партиями и её поставками (З10-и3; удаления через API
    у позиции с историей нет). Продажи и заказы кухни остаются без позиции (good_id → NULL)."""
    if not gid:
        return
    sql(f"""CREATE TEMP TABLE _dg AS SELECT DISTINCT supply_id FROM stock_lots WHERE good_id = '{gid}' AND supply_id IS NOT NULL;
        DELETE FROM lot_moves WHERE lot_id IN (SELECT id FROM stock_lots WHERE good_id = '{gid}');
        DELETE FROM stock_moves WHERE good_id = '{gid}';
        DELETE FROM stock_lots WHERE good_id = '{gid}';
        DELETE FROM supplies s WHERE s.id IN (SELECT supply_id FROM _dg) AND NOT EXISTS (SELECT 1 FROM stock_lots l WHERE l.supply_id = s.id);
        DELETE FROM goods WHERE id = '{gid}'""")

def stock_to(gid, target, note, who="boss"):
    """Остаток позиции — к target корректировкой через API, а не SQL: движение и партии остаются честными (З10-и3)."""
    cur = int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'") or 0)
    if cur != target:
        return api("POST", f"/admin/goods/{gid}/stock", {"delta": target - cur, "reason": "adjust", "note": note}, who)


# ── персонал ─────────────────────────────────────────────────────────────────────────────────────
def hire_temp(nick, keep=False, **profile):
    """Временный сотрудник z<N>adm с паролем STAFF_PASS, нанят 30 дней назад. Удалить аккаунт нельзя
    (FK), поэтому повторный прогон возвращает его: роль admin, пароль как у admin1, карточка без
    увольнения. keep=False — в конце уволить (at_end). Наём с З14-и3 требует сменить пароль при первом
    входе — временным сотрудникам проверок это не нужно (это проверяет e2e_z14_password): флаг снят SQL-ом."""
    hired = club_day(-30)
    if not uid(nick):
        s, d = api("POST", "/admin/staff/hire", {"nickname": nick, "password": STAFF_PASS, "hired_at": hired, **profile}, "boss")
        if s != 201:
            raise SystemExit(f"наём {nick}: {s} {J(d)}")
    else:
        sql(f"UPDATE users SET role='admin', status='active', password_hash=(SELECT password_hash FROM users "
            f"WHERE nickname='admin1') WHERE nickname='{nick}'")
        sql(f"UPDATE staff_profiles SET dismissed_at=NULL, hired_at='{hired}' WHERE user_id='{uid(nick)}'")
    sql(f"UPDATE users SET must_change_password=false WHERE nickname='{nick}'")
    _cache.pop(nick, None)
    if not keep:
        at_end(lambda: _retire(nick))
    return uid(nick)

def drawer_ready(who):
    """Ящик заведён (З8-и1): на стенде без истории ящика первая сдача — нулевая точка, а проверке нужны
    ожидаемое и расхождения. Тогда who (на смене) сдаёт кассу пересчётом 500 zł — это начальный остаток — и
    снова выходит на смену. Возвращает предпросмотр кассы who."""
    p = api("GET", "/admin/work/closeout", None, who)[1]
    if p.get("baseline"):
        api("POST", "/admin/work/stop", {"counted_pln": 500}, who)
        api("POST", "/admin/work/start", {}, who)
        p = api("GET", "/admin/work/closeout", None, who)[1]
    return p

def _retire(nick):
    """Уборка временного сотрудника: уволить и снять незакрытую запись табеля (вход открывает смену)."""
    dismiss(nick)
    sql(f"DELETE FROM work_entries WHERE ended_at IS NULL AND user_id='{uid(nick)}'")

def dismiss(nick, **body):
    """POST /admin/staff/:id/dismiss от владельца (уже player — пропуск)."""
    if sql(f"SELECT role FROM users WHERE nickname='{nick}'") in ("", "player"):
        return 200, {}
    _cache.pop(nick, None)
    return api("POST", f"/admin/staff/{uid(nick)}/dismiss", body, "boss")


# ── live-канал админки ───────────────────────────────────────────────────────────────────────────
def ws_admin(ticket):
    """Сырой апгрейд /ws/admin по тикету (без библиотек): (сокет, код ответа; 101 — канал открыт).
    Тикет: api("POST", "/admin/ws-ticket", {}, who)[1]["ticket"]."""
    s = socket.create_connection((HOST, PORT), timeout=5)
    s.sendall((f"GET /api/v1/ws/admin?ticket={ticket} HTTP/1.1\r\nHost: {HOST}:{PORT}\r\nUpgrade: websocket\r\n"
               f"Connection: Upgrade\r\nSec-WebSocket-Key: {base64.b64encode(os.urandom(16)).decode()}\r\n"
               "Sec-WebSocket-Version: 13\r\n\r\n").encode())
    head = b""
    while b"\r\n\r\n" not in head and (ch := s.recv(1)):
        head += ch
    return s, int(head.split(b" ")[1]) if head.startswith(b"HTTP/1.1 ") else -1

def ws_read(s, wait=5):
    """Кадры сервера, пока не придёт close или wait секунд тишины: (события — JSON текстовых кадров,
    код close или None, если канал молчит или оборван без close)."""
    s.settimeout(wait)
    events = []
    def need(k):
        b = b""
        while len(b) < k:
            ch = s.recv(k - len(b))
            if not ch:
                raise OSError("eof")
            b += ch
        return b
    try:
        while True:
            b1, b2 = need(2)
            n = b2 & 0x7F
            n = struct.unpack(">H", need(2))[0] if n == 126 else struct.unpack(">Q", need(8))[0] if n == 127 else n
            data = need(n)
            if b1 & 0x0F == 8:
                return events, (struct.unpack(">H", data[:2])[0] if len(data) >= 2 else 1005)
            if b1 & 0x0F == 1:
                try:
                    events.append(json.loads(data))
                except ValueError:
                    pass
    except OSError:
        return events, None


# ── журнал и мок агента ──────────────────────────────────────────────────────────────────────────
def audit(who="boss", **params):
    """Строки /admin/audit (kind=…, day=…, limit=…)."""
    q = urllib.parse.urlencode(params)
    return api("GET", "/admin/audit" + (f"?{q}" if q else ""), who=who)[1].get("items", [])

_agents = {}

def agent_start(pc, name=None):
    """Мок агента ПК на /ws/shell (mock_agent.js); остановится сам в конце."""
    name = name or f"z_{pc}"
    log = agent_log(name)
    try:
        open(log, "w").close()
    except OSError:
        pass
    p = subprocess.Popen(["node", MOCK_AGENT, pcs()[pc]["id"], log], stdout=subprocess.DEVNULL,
                         stderr=subprocess.DEVNULL, start_new_session=True)
    if not _agents:
        at_end(lambda: [agent_stop(n) for n in list(_agents)])
    _agents[name] = p
    for _ in range(30):
        if '"open"' in open(log).read():
            break
        time.sleep(0.1)
    time.sleep(0.4)
    return name

def agent_stop(name):
    p = _agents.pop(name, None)
    if p:
        p.terminate()
        try:
            p.wait(3)
        except Exception:
            p.kill()
        time.sleep(0.6)
