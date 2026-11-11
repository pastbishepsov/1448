#!/usr/bin/env python3
"""З20-и1 «Сервер принимает статус приложений» (№57 «Где не установлено», обязательно). Агент шлёт по WS apps_status —
снимок доступности приложений каталога, как его GET /apps-status; сервер хранит последний по каждому ПК в pc_app_status
(миграция 096): доступно ли, почему нет, когда пришло. Новый снимок заменяет прежний целиком (пропавшее из снимка
уходит, пустой снимок — ни одной строки); приложение не из каталога пропускается; битый снимок отклоняется целиком —
остаётся прежний (в журнале сервера — «apps_status … отклонён»). Незнакомый тип агент шлёт без последствий: сокет жив,
на тик приходит state. Агент без нового типа — как раньше: подключён, state на тик, строк нет. ПК удалён — его строки
тоже. Мок-агент шлёт сообщение из файла (третий аргумент mock_agent.js). Временные ПК z20a-1, z20a-2 и записи каталога
z20a_cs2, z20a_gta5, z20a_new удаляются в начале и в конце."""
import json
import os
import time

import _z
from _z import *

APPS = ("z20a_cs2", "z20a_gta5", "z20a_new")
MSG = os.path.join(os.environ.get("E2E_OUT") or "/tmp", "z20a_msg.json")


def wipe():
    sql("DELETE FROM computers WHERE name LIKE 'z20a-%'")
    sql("DELETE FROM catalog_apps WHERE id IN ('z20a_cs2', 'z20a_gta5', 'z20a_new')")


wipe()
at_end(wipe)
at_end(lambda: os.path.exists(MSG) and os.remove(MSG))
has_table = sql("SELECT to_regclass('pc_app_status') IS NOT NULL") == "t"
state = lambda pc: rows(f"SELECT app_id || ' ' || available || ' ' || reason FROM pc_app_status WHERE computer_id = '{pc}' "
                        "ORDER BY app_id") if has_table else ["нет таблицы pc_app_status"]
at = lambda pc: sql(f"SELECT max(reported_at)::text FROM pc_app_status WHERE computer_id = '{pc}'") if has_table else ""


def send(msg):
    """Сообщение мок-агенту: он шлёт файл, как только тот поменялся (опрос 0,2 с)."""
    with open(MSG + ".tmp", "w") as f:
        f.write(json.dumps(msg, ensure_ascii=False))
    os.replace(MSG + ".tmp", MSG)
    time.sleep(0.8)


def wait_for(cond, sec=4.0):
    end = time.time() + sec
    while time.time() < end:
        if cond():
            return True
        time.sleep(0.2)
    return cond()


def agent_lines(name):
    try:
        return [json.loads(l) for l in open(_z.agent_log(name)) if l.strip()]
    except OSError:
        return []


section("подготовка")
mk = lambda n: (api("POST", "/admin/computers", {"name": n}, "boss")[1] or {}).get("computer_id", "")
P1, P2 = mk("z20a-1"), mk("z20a-2")
cat = [api("POST", "/admin/catalog", {"id": a, "name": a.replace("_", " "), "category": "game"}, "boss")[0] for a in APPS]
_z._pcs.clear()  # кэш ПК стенда — заново, уже с z20a-*
check("ПК z20a-1, z20a-2 и записи каталога z20a_cs2, z20a_gta5, z20a_new заведены; таблица pc_app_status есть (миграция 096)",
      bool(P1) and bool(P2) and all(s in (200, 201) for s in cat) and has_table, J([P1, P2, cat, has_table]))

section("снимок и замена")
send({"type": "apps_status", "payload": {"apps": {
    "z20a_cs2": {"available": True, "kind": "protocol"},
    "z20a_gta5": {"available": False, "reason": "path_unreachable", "checked_at": "2026-11-09T10:00:00Z"},
    "z20a_new": {"available": True, "reason": "checking"},
    "z20a_ghost": {"available": False, "reason": "target_missing"}},
    "running": {"app_id": "z20a_cs2", "pid": 1234}}})
ag1 = agent_start("z20a-1", "z20a-1", send=MSG)
want1 = ["z20a_cs2 true ", "z20a_gta5 false path_unreachable", "z20a_new true checking"]
ok1 = wait_for(lambda: state(P1) == want1)
t1 = at(P1)
check("снимок принят: cs2 — доступно, gta5 — нет (path_unreachable), new — checking; z20a_ghost не из каталога — пропущен",
      ok1, J(state(P1)))
time.sleep(1.1)
send({"type": "apps_status", "payload": {"apps": {
    "z20a_cs2": {"available": False, "reason": "target_missing"},
    "z20a_gta5": {"available": False, "reason": "path_unreachable"}}}})
want2 = ["z20a_cs2 false target_missing", "z20a_gta5 false path_unreachable"]
ok2 = wait_for(lambda: state(P1) == want2)
t2 = at(P1)
check("новый снимок заменяет прежний: cs2 — target_missing, new пропал из снимка — строки нет, время снимка новое",
      ok2 and bool(t1) and bool(t2) and t2 > t1, J([state(P1), t1, t2]))

section("битый снимок, незнакомый тип, пустой снимок")
log0 = os.path.getsize(SRV_LOG) if SRV_LOG and os.path.exists(SRV_LOG) else 0
send({"type": "apps_status", "payload": {"apps": {"z20a_cs2": {"reason": "checking"}}}})
time.sleep(0.6)
s_bad = state(P1)
if SRV_LOG and os.path.exists(SRV_LOG):
    tail = open(SRV_LOG, "rb").read()[log0:].decode("utf-8", "replace")
    check("снимок без available отклонён целиком — строки как были; в журнале сервера — «apps_status … отклонён»",
          s_bad == want2 and f"apps_status от computer={P1} отклонён" in tail, J([s_bad, tail[-300:]]))
else:
    check("снимок без available отклонён целиком — строки как были", s_bad == want2, J(s_bad))
    print("skip проверки лога сервера — задай E2E_SRV_LOG=путь к логу сервера")
n_state0 = sum(1 for l in agent_lines(ag1) if (l.get("msg") or {}).get("type") == "state")
send({"type": "z20_unknown", "payload": {"x": 1}})
got_state = wait_for(lambda: sum(1 for l in agent_lines(ag1) if (l.get("msg") or {}).get("type") == "state") > n_state0, 12)
online = api("GET", "/admin/computers", who="admin1")[1].get("computers") or []
pc1 = next((p for p in online if p.get("id") == P1), {})
check("незнакомый тип — без последствий: сокет жив, на следующий тик пришёл state, строки как были",
      got_state and state(P1) == want2, J([got_state, n_state0, state(P1), {k: pc1.get(k) for k in ("online", "agent_online")}]))
send({"type": "apps_status", "payload": {"apps": {}}})
check("пустой снимок (на ПК нет приложений каталога) — строк нет", wait_for(lambda: state(P1) == []), J(state(P1)))

section("агент без нового типа и удаление ПК")
ag2 = agent_start("z20a-2", "z20a-2")
got2 = wait_for(lambda: any((l.get("msg") or {}).get("type") == "state" for l in agent_lines(ag2)), 12)
check("агент без apps_status — как раньше: подключён, state на тик, строк у ПК нет",
      got2 and state(P2) == [], J([got2, state(P2), agent_lines(ag2)[-2:]]))
send({"type": "apps_status", "payload": {"apps": {"z20a_cs2": {"available": True}}}})
wait_for(lambda: state(P1) == ["z20a_cs2 true "])
agent_stop(ag1)
d = api("DELETE", f"/admin/computers/{P1}", None, "boss")[0]
gone = sql(f"SELECT count(*) FROM pc_app_status WHERE computer_id = '{P1}'") if has_table else "?"
check("ПК удалён — его строки ушли вместе с ним", d == 200 and gone == "0", J([d, gone]))
done()
