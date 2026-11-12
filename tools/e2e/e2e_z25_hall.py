#!/usr/bin/env python3
"""З25-и1 «Экспорт и импорт зала — сервер» (№56): GET /admin/hall/export (владелец) — зал файлом: схема, зоны с ценой,
VAT, порядком и расписанием цен, ПК с клеткой и MAC; POST /admin/hall/import — просмотр (что заведём, что поправим, что
с историей не тронем, чего в файле нет) и запись одной транзакцией и строкой журнала. Экспорт → «чистый клуб» (свои
ПК и зону убрали) → импорт → та же схема (повторный экспорт совпадает). ПК с историей из файла не трогаются; клетка
занята машиной, которую импорт не двигает, — ПК вне схемы; схему не сжимаем, если за границами останутся ПК; кривой
файл — 400 со словами; админу — 403.
Данные: временная зона «z25 Ложа» (15 zł/ч, VAT 8 %, порядок 50, правило «Сб, Вс 12:00–18:00 — 12 zł/ч») и ПК z25-1,
z25-2 (на свободных клетках, с MAC) и z25-3 (вне схемы); в конце ПК «z25-…» и зоны «z25 …» удаляются SQL-ом."""
import copy

from _z import *


def wipe():
    sql("DELETE FROM computers WHERE name LIKE 'z25-%'")
    sql("DELETE FROM zones WHERE name LIKE 'z25 %'")


wipe()
at_end(wipe)
seen = lambda: {x.get("id") for x in audit("boss", limit=60)}
A0 = seen()
W, H = (int(x) for x in sql("SELECT layout_w || ' ' || layout_h FROM clubs WHERE is_active ORDER BY name LIMIT 1").split())
busy = set(rows("SELECT pos_x || ':' || pos_y FROM computers WHERE pos_x IS NOT NULL"))
free = [(x, y) for y in range(H - 1, -1, -1) for x in range(W) if f"{x}:{y}" not in busy][:2]
zid = (api("POST", "/admin/zones", {"name": "z25 Ложа", "rate_pln": 15, "vat_pct": 8, "sort": 50}, "boss")[1].get("zone") or {}).get("id")
sr, _ = api("POST", f"/admin/zones/{zid}/rules", {"days_mask": 96, "start_min": 720, "end_min": 1080, "rate_pln": 12}, "boss")
pcs_ = {}
for n, cell, mac in (("z25-1", free[0], "AA:BB:CC:00:25:01"), ("z25-2", free[1], "AA:BB:CC:00:25:02"), ("z25-3", None, "")):
    body = {"name": n, "zone_id": zid}
    if cell:
        body.update(pos_x=cell[0], pos_y=cell[1])
    pcs_[n] = api("POST", "/admin/computers", body, "boss")[1].get("computer_id")
    if mac:
        api("PATCH", f"/admin/computers/{pcs_[n]}", {"mac": mac}, "boss")
check(f"зона «z25 Ложа» с правилом, ПК z25-1 и z25-2 на свободных клетках {free}, z25-3 вне схемы", bool(zid) and sr == 201
      and all(pcs_.values()), J([zid, sr, pcs_]))

section("экспорт")
se, f0 = api("GET", "/admin/hall/export", None, "boss")
zf = next((z for z in f0.get("zones") or [] if z.get("name") == "z25 Ложа"), {})
pf = {p.get("name"): p for p in f0.get("computers") or [] if p.get("name", "").startswith("z25-")}
check("экспорт: «1448-hall», версия 1, схема клуба, «z25 Ложа» — 15 zł/ч, VAT 8, порядок 50 и правило; z25-1, z25-2 — клетка, "
      "зона и MAC; z25-3 — без клетки",
      se == 200 and f0.get("format") == "1448-hall" and f0.get("version") == 1 and f0.get("layout") == {"w": W, "h": H}
      and zf.get("rate_pln") == 15 and zf.get("vat_pct") == 8 and zf.get("sort") == 50
      and zf.get("rules") == [{"days_mask": 96, "start_min": 720, "end_min": 1080, "rate_pln": 12}]
      and pf.get("z25-1", {}).get("pos_x") == free[0][0] and pf.get("z25-1", {}).get("mac") == "AA:BB:CC:00:25:01"
      and pf.get("z25-2", {}).get("zone") == "z25 Ложа" and "pos_x" not in pf.get("z25-3", {}), J([se, zf, pf], 700))
sa, _ = api("GET", "/admin/hall/export", None, "admin1")
sb, _ = api("POST", "/admin/hall/import", {"hall": f0}, "admin1")
check("админу экспорт и импорт — 403", sa == 403 and sb == 403, [sa, sb])

if se != 200 or not f0.get("computers"):  # экспорта нет (до З25-и1) — FAIL по каждой проверке, а не обрыв
    for n in ("чистый клуб — просмотр", "запись и повторный экспорт", "ПК с историей", "занятая клетка", "схема не сжимается",
              "кривые файлы", "журнал"):
        check(n + ": экспорта зала нет", False)
    done()

section("чистый клуб → импорт → та же схема")
for n in ("z25-1", "z25-2", "z25-3"):
    api("DELETE", f"/admin/computers/{pcs_[n]}", None, "boss")
sd, _ = api("DELETE", f"/admin/zones/{zid}", None, "boss")
gone = sql("SELECT count(*) FROM computers WHERE name LIKE 'z25-%'") == "0" and sql("SELECT count(*) FROM zones WHERE name LIKE 'z25 %'") == "0"
sp, pv = api("POST", "/admin/hall/import", {"hall": f0}, "boss")
acts = {p.get("name"): p.get("action") for p in pv.get("computers") or []}
za = {z.get("name"): (z.get("action"), z.get("rules_add")) for z in pv.get("zones") or []}
others = [a for n, a in acts.items() if not n.startswith("z25-")]
check("свои ПК и зона удалены; просмотр: «z25 Ложа» — новая (правил 1), z25-1…3 — новые, остальные — без изменений; "
      "ничего не записано",
      sd == 200 and gone and sp == 200 and pv.get("applied") is False and za.get("z25 Ложа") == ("create", 1)
      and all(acts.get(n) == "create" for n in ("z25-1", "z25-2", "z25-3")) and others and all(a == "same" for a in others)
      and sql("SELECT count(*) FROM computers WHERE name LIKE 'z25-%'") == "0" and pv.get("layout", {}).get("change") is False,
      J([sd, gone, sp, za, acts, pv.get("summary")], 700))
si, im = api("POST", "/admin/hall/import", {"hall": f0, "apply": True}, "boss")
s2, f1 = api("GET", "/admin/hall/export", None, "boss")
strip = lambda f: {k: v for k, v in f.items() if k != "exported_at"}
check("запись — 200; повторный экспорт — тот же зал, что до удаления (схема, зоны с правилами, ПК с клетками и MAC)",
      si == 200 and im.get("applied") is True and s2 == 200 and strip(f1) == strip(f0), J([si, im.get("summary"), s2], 500))

section("история, занятая клетка, схема, кривые файлы")
pc01 = next((p for p in f0["computers"] if p["name"] == "PC-01"), None)
hist = sql(f"SELECT count(*) > 0 FROM sessions WHERE computer_id = (SELECT id FROM computers WHERE name = 'PC-01')") == "t"
f2 = copy.deepcopy(f0)
p01 = next(p for p in f2["computers"] if p["name"] == "PC-01")
p01["zone"] = "z25 Ложа"
p01["mac"] = "AA:BB:CC:00:25:99"
s3, pv3 = api("POST", "/admin/hall/import", {"hall": f2, "apply": True}, "boss")
r01 = next((p for p in pv3.get("computers") or [] if p.get("name") == "PC-01"), {})
check("PC-01 с историей: в файле — другая зона и MAC — «history» (есть история — не трогаем), после записи PC-01 как был",
      hist and s3 == 200 and r01.get("action") == "history" and any("зона:" in c for c in r01.get("changes") or [])
      and sql("SELECT zone || '|' || COALESCE(mac, '') FROM computers WHERE name = 'PC-01'") == f"{pc01.get('zone')}|{pc01.get('mac', '')}",
      J([hist, s3, r01]))
f3 = copy.deepcopy(f0)
pc02 = next(p for p in f3["computers"] if p["name"] == "PC-02")
f3["computers"] = [p for p in f3["computers"] if p["name"] != "PC-02"]  # PC-02 в файле нет — клетку держит он
f3["computers"].append({"name": "z25-4", "zone": "z25 Ложа", "pos_x": pc02["pos_x"], "pos_y": pc02["pos_y"]})
s4, pv4 = api("POST", "/admin/hall/import", {"hall": f3}, "boss")
r4 = next((p for p in pv4.get("computers") or [] if p.get("name") == "z25-4"), {})
check("новый z25-4 в клетку PC-02, которого в файле нет, — «клетка … занята «PC-02» — заводим вне схемы», PC-02 — в «в файле нет»",
      s4 == 200 and r4.get("action") == "create" and "занята «PC-02» — заводим вне схемы" in (r4.get("note") or "") and r4.get("cell") == "вне схемы"
      and "PC-02" in ((pv4.get("missing") or {}).get("computers") or []), J([s4, r4, pv4.get("missing")]))
f5 = copy.deepcopy(f0)
f5["layout"] = {"w": 4, "h": 3}
f5["computers"] = [p for p in f5["computers"] if p.get("pos_x") is None or (p["pos_x"] < 4 and p["pos_y"] < 3)]
s5, pv5 = api("POST", "/admin/hall/import", {"hall": f5}, "boss")
lay = pv5.get("layout") or {}
check("файл со схемой 4×3 — «схему 4×3 не сжимаем: за границами останутся …», ПК, которых нет в файле, — в «в файле нет»",
      s5 == 200 and lay.get("change") is False and (lay.get("note") or "").startswith("схему 4×3 не сжимаем: за границами останутся")
      and len((pv5.get("missing") or {}).get("computers") or []) > 0, J([s5, lay, pv5.get("missing")], 500))
bad = [("не тот формат", lambda f: f.update(format="x"), "Это не файл зала"),
       ("ПК дважды", lambda f: f["computers"].append(dict(f["computers"][0])), "в файле дважды"),
       ("клетка за схемой", lambda f: f["computers"].append({"name": "z25-9", "pos_x": 99, "pos_y": 0}), "за схемой"),
       ("кривой MAC", lambda f: f["computers"].append({"name": "z25-9", "mac": "zz"}), "MAC «zz»")]
res = []
for n, edit, want in bad:
    fb = copy.deepcopy(f0)
    edit(fb)
    s, d = api("POST", "/admin/hall/import", {"hall": fb}, "boss")
    res.append((n, s, d.get("message", ""), want))
check("кривые файлы — 400 со словами: " + ", ".join(n for n, *_ in bad), all(s == 400 and w in m for _, s, m, w in res),
      J([(n, s, m) for n, s, m, _ in res], 700))
aud = [(x.get("kind"), x.get("text", "")) for x in audit("boss", limit=60) if x.get("id") not in A0 and (x.get("kind") or "").startswith("hall_")]
imp = [t for k, t in aud if k == "hall_import"]
check("журнал: экспорты — «hall_export», записи — «hall_import»: «из файла: зоны: новых 1 … заведены: z25-1, z25-2, z25-3», "
      "и вторая — без новых", sum(1 for k, _ in aud if k == "hall_export") >= 2 and len(imp) == 2
      and any(t.startswith("из файла: зоны: новых 1") and t.endswith("заведены: z25-1, z25-2, z25-3") for t in imp), J(aud, 900))

done()
