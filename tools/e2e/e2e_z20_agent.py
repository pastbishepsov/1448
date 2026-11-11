#!/usr/bin/env python3
"""З20-и2 «Агент сообщает серверу» (№57 «Где не установлено»): связка настоящего агента ПК (cmd/agent, собранного под
Linux; путь — E2E_AGENT_BIN, без него проверка пропускается) и сервера стенда. Агент берёт каталог стенда, проверяет пути
и сразу после подключения шлёт apps_status: в pc_app_status — его снимок, тот же, что у его GET /apps-status (протокол
steam:// — доступен, файла нет — target_missing). Файл появился — агент замечает на своей проверке путей (раз в 5 с) и
шлёт новый снимок, не дожидаясь 10 минут. Агент остановлен — строки остаются (последний снимок), сокет — офлайн.
Временный ПК z20g-1, записи каталога z20g_proto и z20g_file удаляются в начале и в конце, конфиг агента и файл игры — в
$E2E_OUT."""
import json
import os
import subprocess
import time
import urllib.request

from _z import *

BIN = os.environ.get("E2E_AGENT_BIN", "")
OUT = os.environ.get("E2E_OUT") or "/tmp"
GAME = os.path.join(OUT, "z20g_game", "game.exe")
CFG = os.path.join(OUT, "z20g_agent.json")
LOG = os.path.join(OUT, "z20g_agent.log")
PORT = 14820


def wipe():
    sql("DELETE FROM computers WHERE name = 'z20g-1'")
    sql("DELETE FROM catalog_apps WHERE id IN ('z20g_proto', 'z20g_file')")
    if os.path.exists(GAME):
        os.remove(GAME)


if not BIN or not os.path.exists(BIN):
    print("skip — задай E2E_AGENT_BIN=путь к агенту, собранному под Linux (go build -o … ./cmd/agent)")
    done()

wipe()
at_end(wipe)
P = (api("POST", "/admin/computers", {"name": "z20g-1"}, "boss")[1] or {}).get("computer_id", "")
cat = [api("POST", "/admin/catalog", body, "boss")[0] for body in (
    {"id": "z20g_proto", "name": "z20g Протокол", "category": "game", "target": "steam://rungameid/730"},
    {"id": "z20g_file", "name": "z20g Файл", "category": "game", "target": GAME})]
os.makedirs(os.path.dirname(GAME), exist_ok=True)
with open(CFG, "w") as f:
    json.dump({"backend_ws": API.replace("http", "ws", 1) + "/ws/shell", "computer_id": P, "listen": f"127.0.0.1:{PORT}",
               "catalog_url": API + "/catalog", "lock_action": "none",
               "launch": {"check_interval_sec": 5, "check_timeout_sec": 2, "telemetry": False}}, f)
check("ПК z20g-1 и записи каталога z20g_proto (steam://) и z20g_file (файла пока нет) заведены",
      bool(P) and all(s in (200, 201) for s in cat), J([P, cat]))

agent = subprocess.Popen([BIN, "-config", CFG], stdout=open(LOG, "w"), stderr=subprocess.STDOUT, start_new_session=True)
at_end(lambda: agent.poll() is None and (agent.terminate(), agent.wait(5)))
state = lambda: rows(f"SELECT app_id || ' ' || available || ' ' || reason FROM pc_app_status WHERE computer_id = '{P}' "
                     "AND app_id LIKE 'z20g_%' ORDER BY app_id")


def wait_for(cond, sec):
    end = time.time() + sec
    while time.time() < end:
        if cond():
            return True
        time.sleep(0.5)
    return cond()


def agent_apps():
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{PORT}/apps-status", timeout=2) as r:
            apps = json.load(r).get("apps") or {}
        return sorted(f"{k} {str(v.get('available')).lower()} {v.get('reason', '')}" for k, v in apps.items() if k.startswith("z20g_"))
    except OSError:
        return []


want1 = ["z20g_file false target_missing", "z20g_proto true "]
ok1 = wait_for(lambda: state() == want1 and agent_apps() == want1, 25)
check("агент на связи: в pc_app_status — его снимок, как у его /apps-status: steam:// — доступен, файла нет — "
      "target_missing", ok1, J([state(), agent_apps(), open(LOG).read()[-400:]]))
t0 = time.time()
open(GAME, "w").close()
want2 = ["z20g_file true ", "z20g_proto true "]
ok2 = wait_for(lambda: state() == want2, 40)
check(f"файл игры появился — новый снимок у сервера через {time.time() - t0:.0f} с (проверка путей — 5 с, сверка "
      "снимка — 15 с), а не через 10 минут", ok2 and time.time() - t0 < 40, J([state(), agent_apps()]))
agent.terminate()
agent.wait(5)
time.sleep(1)
pc = next((p for p in api("GET", "/admin/computers", who="admin1")[1].get("computers") or [] if p.get("id") == P), {})
check("агент остановлен — строки остаются (последний снимок)", state() == want2, J([state(), pc.get("online")]))
done()
