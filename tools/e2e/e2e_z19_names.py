#!/usr/bin/env python3
"""З19-и2 «Имена без дублей» (№40, решение основателя 23.09). Миграция 094: имена ПК, зон и товаров — без пробелов по
краям и уникальны без учёта регистра в пределах клуба. При живом PC-01 новый ПК «PC-01 » и «pc-01» — 409 name_taken,
«  z19n-1  » заводится как «z19n-1»; переименовать его в «pc-02» — 409, в «Z19N-1» (тот же ПК, другой регистр) — можно.
Зона «standard» и « Standard » при живой Standard — 409 already_exists; товар «Z19N СОК» и « z19n сок » при «z19n Сок» —
409. Индексы на месте; дубли есть — миграция останавливается с текстом «какие имена» и ничего не меняет (проверка — в
одной транзакции с откатом: индекс снят, дубль заведён, тело миграции — отказ, ROLLBACK). Временные ПК z19n-*, зона
«z19n Зона» и товар «z19n Сок» удаляются в начале и в конце."""
import os
import subprocess

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "../../backend/migrations/094_names_unique_ci.sql")
GOOD, ZONE = "z19n Сок", "z19n Зона"


def wipe():
    # и хвосты прогона на HEAD (до 094 имена с пробелами и в другом регистре заводились): точные имена, не PC-01 и Standard
    sql("DELETE FROM computers WHERE lower(name) LIKE 'z19n-%' OR name IN ('PC-01 ', 'pc-01') OR btrim(name) = ''")
    sql(f"DELETE FROM zones WHERE lower(name) = lower('{ZONE}') OR name IN ('standard', ' Standard ')")
    for gid in rows(f"SELECT id FROM goods WHERE lower(name) = lower('{GOOD}')"):
        drop_good(gid)


wipe()
at_end(wipe)
CLUB = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")

section("миграция 094")
idx = rows("SELECT indexname FROM pg_indexes WHERE indexname IN ('computers_club_name_ci', 'zones_club_name_ci', 'goods_club_name_ci') ORDER BY 1")
check("индексы без учёта регистра у ПК, зон и товаров — на месте", idx == ["computers_club_name_ci", "goods_club_name_ci", "zones_club_name_ci"], J(idx))
body = open(MIG, encoding="utf-8").read() if os.path.exists(MIG) else ""
do = body[body.find("DO $$"):body.find("END $$;") + len("END $$;")] if "DO $$" in body else ""
r = subprocess.run(["psql", DB, "-v", "ON_ERROR_STOP=0", "-q", "-c",
                    "BEGIN; DROP INDEX computers_club_name_ci; "
                    f"INSERT INTO computers (club_id, name, status) VALUES ('{CLUB}', 'pc-01', 'available'); " + do + " ROLLBACK;"],
                   capture_output=True, text=True)
left = sql("SELECT count(*) FROM computers WHERE name = 'pc-01'")
idx2 = sql("SELECT count(*) FROM pg_indexes WHERE indexname = 'computers_club_name_ci'")
check("дубль «PC-01» и «pc-01» — миграция останавливается: «имена совпадают без учёта регистра — ПК «PC-01», «pc-01»», "
      "ничего не меняет (в одной транзакции с откатом)", bool(do) and "Миграция 094 остановлена" in r.stderr and "ПК «PC-01», «pc-01»" in r.stderr
      and left == "0" and idx2 == "1", J([r.stderr[-400:], left, idx2]))

section("ПК")
r = [api("POST", "/admin/computers", {"name": n}, "boss") for n in ("PC-01 ", "pc-01", "  z19n-1  ", "z19n-2", "   ")]
p1 = (r[2][1] or {}).get("computer_id", "")
check("новый ПК «PC-01 » и «pc-01» при живом PC-01 — 409 name_taken; «  z19n-1  » — 201, имя «z19n-1»; «   » — 400",
      [x[0] for x in r] == [409, 409, 201, 201, 400] and r[0][1].get("code") == r[1][1].get("code") == "name_taken"
      and sql(f"SELECT name FROM computers WHERE id = '{p1}'") == "z19n-1", J([[x[0], x[1].get("code")] for x in r]))
u = [api("PATCH", f"/admin/computers/{p1}", {"name": n}, "boss") for n in ("pc-02", " Z19N-2 ", "Z19N-1")]
check("переименовать z19n-1 в «pc-02» и « Z19N-2 » — 409; в «Z19N-1» (тот же ПК, другой регистр) — 200",
      [x[0] for x in u] == [409, 409, 200] and sql(f"SELECT name FROM computers WHERE id = '{p1}'") == "Z19N-1", J([[x[0], x[1].get("code")] for x in u]))

section("зоны и товары")
z = [api("POST", "/admin/zones", {"name": n, "rate_pln": 15}, "boss") for n in ("standard", " Standard ", ZONE, ZONE.upper())]
check("зона «standard» и « Standard » при живой Standard — 409 already_exists; «z19n Зона» — 201, «Z19N ЗОНА» — 409",
      [x[0] for x in z] == [409, 409, 201, 409] and all(x[1].get("code") == "already_exists" for x in (z[0], z[1], z[3])), J([[x[0], x[1].get("code")] for x in z]))
g = [api("POST", "/admin/goods", {"name": n, "category": "z19n", "price_pln": 4}, "boss") for n in (GOOD, "Z19N СОК", " z19n сок ")]
check("товар «z19n Сок» — 201, «Z19N СОК» и « z19n сок » — 409 already_exists",
      [x[0] for x in g] == [201, 409, 409] and all(x[1].get("code") == "already_exists" for x in g[1:]), J([[x[0], x[1].get("code")] for x in g]))
done()
