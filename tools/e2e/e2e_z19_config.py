#!/usr/bin/env python3
"""З19-и3 «Табло, удаление ПК и записей каталога» (№40, решение основателя 23.09). Табло: GET /board/state — токен
заголовком X-Board-Token (200), без токена и с чужим — 401, токен в адресе — только для старых вкладок (200); опрос
табло в журнал запросов сервера не попадает — ни путь, ни токен. ПК: удалить, пока киоск на связи, — 409 pc_online;
пока по нему открыта проблема — 409 has_incidents; на связи нет и проблема закрыта — 200. Правка ПК: MAC и клетка схемы
— в строке журнала «было → стало» («MAC — → AA:…», «клетка — → 3,8», «клетка 3,8 → —»). Каталог: платформу с игрой
не удалить — 409 platform_has_games с числом игр; удалённая игра уходит из избранного гостя (unfavorited); пустую
платформу — можно. Временные ПК z19c-*, записи каталога z19c_plat и z19c_game, токен табло, избранное Guest48 — в
начале и в конце как были."""
import json
import os
import time
import urllib.error
import urllib.request

from _z import *

G = "Guest48"
tok0 = rows("SELECT value FROM settings WHERE key = 'board_token'")
fav0 = sql(f"SELECT favorite_games::text FROM users WHERE nickname = '{G}'")


def wipe():
    sql("DELETE FROM incidents WHERE note LIKE 'z19c %'")
    sql("DELETE FROM computers WHERE name LIKE 'z19c-%'")
    sql("DELETE FROM catalog_apps WHERE id IN ('z19c_plat', 'z19c_game')")


def restore():
    sql("DELETE FROM settings WHERE key = 'board_token'")
    if tok0:
        sql(f"INSERT INTO settings (key, value) VALUES ('board_token', '{tok0[0]}')")
    sql(f"UPDATE users SET favorite_games = '{fav0 or '[]'}'::jsonb WHERE nickname = '{G}'")


wipe()
at_end(wipe)
at_end(restore)

section("табло")
s, d = api("POST", "/admin/board/token", {}, "boss")
T = d.get("token", "")
log0 = os.path.getsize(SRV_LOG) if SRV_LOG and os.path.exists(SRV_LOG) else 0
st = lambda hdr=None, q="": _call_board(q, hdr)


def _call_board(q, hdr):
    req = urllib.request.Request(API + "/board/state" + q, headers=hdr or {})
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code


r = [st({"X-Board-Token": T}), st(), st({"X-Board-Token": "z19c-wrong-token"}), st(None, "?token=" + T)]
check("GET /board/state: токен заголовком — 200, без токена и с чужим — 401, в адресе (старые вкладки) — 200", r == [200, 401, 401, 200], J(r))
api("GET", "/clubs")  # обычный запрос — в журнале есть
time.sleep(0.5)
if SRV_LOG and os.path.exists(SRV_LOG):
    tail = open(SRV_LOG, "rb").read()[log0:].decode("utf-8", "replace")  # смещение — в байтах
    check("журнал запросов сервера: опроса табло нет — ни пути, ни токена; обычный запрос (/clubs) — есть",
          bool(tail) and "/board/state" not in tail and T not in tail and "/api/v1/clubs" in tail, J([len(tail), "/board/state" in tail, T in tail]))
else:
    print("skip проверки лога сервера — задай E2E_SRV_LOG=путь к логу сервера")

section("удаление ПК")
mk = lambda n: (api("POST", "/admin/computers", {"name": n}, "boss")[1] or {}).get("computer_id", "")
P1, P2, P3 = mk("z19c-1"), mk("z19c-2"), mk("z19c-3")
ag = agent_start("z19c-1", "z19c-1")
s1, d1 = api("DELETE", f"/admin/computers/{P1}", None, "boss")
agent_stop(ag)
time.sleep(1.5)
s1b = api("DELETE", f"/admin/computers/{P1}", None, "boss")[0]
check("киоск z19c-1 на связи — 409 pc_online («выключи ПК»); связь пропала — 200", s1 == 409 and d1.get("code") == "pc_online" and s1b == 200,
      J([s1, d1, s1b]))
club = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")
inc = sql(f"INSERT INTO incidents (club_id, computer_id, kind, started_at, note) VALUES ('{club}', '{P2}', 'pc_hang', now(), 'z19c проверка') RETURNING id")
s2, d2 = api("DELETE", f"/admin/computers/{P2}", None, "boss")
sql(f"UPDATE incidents SET resolved_at = now() WHERE id = '{inc}'")
s2b = api("DELETE", f"/admin/computers/{P2}", None, "boss")[0]
check("по z19c-2 открыта проблема — 409 has_incidents («сначала закрой»); закрыта — 200, проблема осталась без ПК",
      s2 == 409 and d2.get("code") == "has_incidents" and s2b == 200 and sql(f"SELECT computer_id IS NULL FROM incidents WHERE id = '{inc}'") == "t",
      J([s2, d2, s2b]))

section("журнал правки ПК: MAC и клетка")
W_, Hh = (int(x) for x in sql(f"SELECT layout_w || ' ' || layout_h FROM clubs WHERE id = '{club}'").split())
busy = {tuple(map(int, r.split())) for r in rows("SELECT pos_x || ' ' || pos_y FROM computers WHERE pos_x IS NOT NULL")}
cell = next(((x, y) for y in range(Hh - 1, -1, -1) for x in range(W_) if (x, y) not in busy), None)
last = lambda: sql("SELECT details FROM admin_actions WHERE action = 'pc_update' ORDER BY created_at DESC LIMIT 1")
api("PATCH", f"/admin/computers/{P3}", {"mac": "aa:bb:cc:dd:ee:19"}, "boss"); l1 = last()
api("PATCH", f"/admin/computers/{P3}", {"pos_x": cell[0], "pos_y": cell[1]}, "boss"); l2 = last()
api("PATCH", f"/admin/computers/{P3}", {"clear_pos": True, "mac": ""}, "boss"); l3 = last()
want = f"{cell[0] + 1},{cell[1] + 1}"
check(f"«z19c-3: MAC — → AA:BB:CC:DD:EE:19», «z19c-3: клетка — → {want}», «z19c-3: MAC AA:BB:CC:DD:EE:19 → — · клетка {want} → —»",
      l1 == "z19c-3: MAC — → AA:BB:CC:DD:EE:19" and l2 == f"z19c-3: клетка — → {want}" and l3 == f"z19c-3: MAC AA:BB:CC:DD:EE:19 → — · клетка {want} → —",
      J([l1, l2, l3]))

section("каталог: платформа с играми, избранное")
up = lambda body: api("POST", "/admin/catalog", body, "boss")
a = [up({"id": "z19c_plat", "name": "z19c Платформа", "category": "platform"})[0],
     up({"id": "z19c_game", "name": "z19c Игра", "category": "game", "subtitle": "z19c Платформа"})[0]]
sql(f"UPDATE users SET favorite_games = '[\"z19c_game\"]'::jsonb WHERE nickname = '{G}'")
s3, d3 = api("DELETE", "/admin/catalog/z19c_plat", None, "boss")
s4, d4 = api("DELETE", "/admin/catalog/z19c_game", None, "boss")
fav = json.loads(sql(f"SELECT favorite_games::text FROM users WHERE nickname = '{G}'") or "[]")
s5 = api("DELETE", "/admin/catalog/z19c_plat", None, "boss")[0]
check("платформа с игрой — 409 platform_has_games (игр: 1); игру удалили — 200, из избранного Guest48 ушла (unfavorited 1); "
      "пустую платформу — 200", a == [s for s in a if s in (200, 201)] and s3 == 409 and d3.get("code") == "platform_has_games"
      and d3.get("games") == 1 and s4 == 200 and d4.get("unfavorited") == 1 and "z19c_game" not in fav and s5 == 200, J([a, [s3, d3], [s4, d4], fav, s5]))
done()
