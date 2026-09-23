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
import atexit, base64, json, re, subprocess, sys, time, urllib.error, urllib.parse, urllib.request

from _env import API, BASE, DB, SRV_LOG, MOCK_AGENT, agent_log

__all__ = ["API", "BASE", "DB", "SRV_LOG", "TZ", "GUEST_PASS", "STAFF_PASS", "sql", "rows", "uid", "login", "tok",
           "tokens", "password_of", "api", "J", "check", "near", "section", "at_end", "done", "club_day", "at_club", "settings",
           "set_settings", "restore_settings", "wallet", "topup", "ensure_wallet", "pcs", "seat", "end", "end_all", "free_pcs",
           "hire_temp", "dismiss", "audit", "agent_start", "agent_stop"]

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

def _exp(token):
    try:
        part = token.split(".")[1]
        return json.loads(base64.urlsafe_b64decode(part + "=" * (-len(part) % 4))).get("exp", 0)
    except Exception:
        return 0

def tokens(nick, password=None, fresh=False):
    """Пара токенов из кэша процесса; нет, истекает через минуту или fresh — вход заново.
    Вход админа открывает ему смену (Е3-и1) — поэтому кэш, а не вход на каждый вызов."""
    t = _cache.get(nick)
    if fresh or not t or t["exp"] - time.time() < 60:
        s, d = login(nick, password)
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
    return int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))

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


# ── персонал ─────────────────────────────────────────────────────────────────────────────────────
def hire_temp(nick, keep=False, **profile):
    """Временный сотрудник z<N>adm с паролем STAFF_PASS, нанят 30 дней назад. Удалить аккаунт нельзя
    (FK), поэтому повторный прогон возвращает его: роль admin, пароль как у admin1, карточка без
    увольнения. keep=False — в конце уволить (at_end)."""
    hired = club_day(-30)
    if not uid(nick):
        s, d = api("POST", "/admin/staff/hire", {"nickname": nick, "password": STAFF_PASS, "hired_at": hired, **profile}, "boss")
        if s != 201:
            raise SystemExit(f"наём {nick}: {s} {J(d)}")
    else:
        sql(f"UPDATE users SET role='admin', status='active', password_hash=(SELECT password_hash FROM users "
            f"WHERE nickname='admin1') WHERE nickname='{nick}'")
        sql(f"UPDATE staff_profiles SET dismissed_at=NULL, hired_at='{hired}' WHERE user_id='{uid(nick)}'")
    _cache.pop(nick, None)
    if not keep:
        at_end(lambda: _retire(nick))
    return uid(nick)

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
