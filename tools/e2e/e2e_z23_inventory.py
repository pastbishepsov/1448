#!/usr/bin/env python3
"""З23-и3 «Инвентаризация — сервер и акт» (№54 «в»): POST /admin/goods/inventory — акт пересчёта (персонал): по строке —
позиция, «в системе», когда начали считать, и «насчитали». Акт целиком или никак: остаток с начала пересчёта изменился —
409 stock_moved со списком, ничего не записано. Разница — корректировка с номером акта (stock_moves.inventory_id):
недостача — со старых партий первыми или, у владельца, из выбранной партии; излишек — партия «нашлось» без цены; сошлось —
строка без движения. Номер — следующий; два акта разом идут по одному. GET /admin/goods/inventories и /:id — акты и акт
(себестоимость недостачи по партиям — только владельцу); /:id/export — файл акта движком выгрузки (владелец, XLSX и
архив CSV, PL и RU). «Потери» в «Деньгах» — losses.inventories: акты периода, недостача штуками, по цене продажи и по
себестоимости, излишек.
Данные: позиции «z23i …» (категория z23i) заводятся каждый прогон, остатки — корректировками «нашлось», цены партиям — PUT
/admin/lots/:id/price (без поставок и расходов); в конце позиции удаляются drop_good вместе с движениями и партиями, акты
проверки (заметка «z23i …») — SQL-ом. «Потери» считаются приростом за прогон."""
import io, threading, urllib.error, urllib.request, xml.etree.ElementTree as ET, zipfile

from _z import *

PRE, CAT = "z23i ", "z23i"
NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
RID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"


def wipe():
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{PRE}%'"):
        drop_good(gid)
    sql("DELETE FROM inventories WHERE note LIKE 'z23i%' AND NOT EXISTS (SELECT 1 FROM stock_moves m WHERE m.inventory_id = inventories.id)")


wipe()
at_end(wipe)
mk = lambda name, price: (api("POST", "/admin/goods", {"name": PRE + name, "category": CAT, "price_pln": price}, "boss")[1].get("good") or {}).get("id")
cola, chips, water, empty = mk("Кола", 5), mk("Чипсы", 8), mk("Вода", 3), mk("Пусто", 4)
stock = lambda gid: int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'") or -1)
left = lambda lid: int(sql(f"SELECT qty_left FROM stock_lots WHERE id = '{lid}'") or -1) if lid else -1
lots = lambda gid: rows(f"SELECT id FROM stock_lots WHERE good_id = '{gid}' ORDER BY received_at, part, id")
acts_n = lambda: int(sql("SELECT count(*) FROM inventories") or 0)
inv = lambda body, who: api("POST", "/admin/goods/inventory", body, who)
L = lambda gid, exp, cnt, **kw: {"good_id": gid, "expected": exp, "counted": cnt, **kw}


def money_now():
    d = api("GET", "/admin/reports/money?preset=today", None, "boss")[1].get("losses") or {}
    return d, d.get("inventories") or {}


section("подготовка: остатки и партии")
for gid, n in ((cola, 6), (cola, 6), (chips, 4), (water, 5)):
    api("POST", f"/admin/goods/{gid}/stock", {"delta": n, "reason": "adjust", "note": "z23i: нашлось для проверки"}, "boss")
l1, l2 = (lots(cola) + ["", ""])[:2]
p1 = api("PUT", f"/admin/lots/{l1}/price", {"price_pln": 2}, "boss")[0]
p2 = api("PUT", f"/admin/lots/{l2}/price", {"price_pln": 2.5}, "boss")[0]
check("позиции z23i: Кола 12 (партии 6 по 2,00 и 6 по 2,50), Чипсы 4, Вода 5, Пусто 0",
      (stock(cola), stock(chips), stock(water), stock(empty), p1, p2) == (12, 4, 5, 0, 200, 200),
      J([stock(cola), stock(chips), stock(water), stock(empty), p1, p2]))
LOSS0, INV0 = money_now()
no0 = int(sql("SELECT COALESCE(MAX(no), 0) FROM inventories") or 0)
seen = lambda who: {x.get("id") for x in audit(who, limit=80)}  # журнал — только новые строки: на HEAD старые не засчитать
A0 = seen("admin1")

section("акт админа: недостача — со старых партий первыми, излишек — «нашлось», сошлось — без движения")
s, d = inv({"note": "z23i: пересчёт бара", "lines": [L(cola, 12, 9), L(chips, 4, 6), L(water, 5, 5), L(empty, 0, 0)]}, "admin1")
a1 = d.get("inventory") or {}
t1 = a1.get("totals") or {}
check("201, номер — следующий, итоги: 4 позиции, сошлось 2; недостача — 1 позиция, 3 шт на 15 zł; излишек — 1, 2 шт на 16 zł",
      s == 201 and a1.get("no") == no0 + 1 and (t1.get("positions"), t1.get("matched"), t1.get("short_positions"), t1.get("short_units"),
      t1.get("short_pln"), t1.get("surplus_positions"), t1.get("surplus_units"), t1.get("surplus_pln"), t1.get("net_pln")) == (4, 2, 1, 3, 15, 1, 2, 16, 1),
      J([s, a1.get("no"), no0, t1], 500))
check("админу — без себестоимости: ни в итогах, ни в строках", s == 201 and len(a1.get("lines") or []) == 4 and "cost_pln" not in t1
      and all("cost_pln" not in x for x in a1.get("lines") or []), J(a1.get("lines"), 400))
ln = {x.get("name"): x for x in a1.get("lines") or []}
check("строки в порядке пересчёта: Кола −3 (−15 zł), Чипсы +2, Вода и Пусто — 0",
      [x.get("name") for x in a1.get("lines") or []] == [PRE + n for n in ("Кола", "Чипсы", "Вода", "Пусто")]
      and (ln.get(PRE + "Кола") or {}).get("delta") == -3 and (ln.get(PRE + "Кола") or {}).get("delta_pln") == -15
      and (ln.get(PRE + "Чипсы") or {}).get("delta") == 2 and (ln.get(PRE + "Вода") or {}).get("delta") == 0, J(a1.get("lines"), 500))
check("склад: Кола 9 — ушло из первой партии (осталось 3 и 6), Чипсы 6, Вода 5, Пусто 0",
      (stock(cola), stock(chips), stock(water), stock(empty), left(l1), left(l2)) == (9, 6, 5, 0, 3, 6),
      J([stock(cola), stock(chips), stock(water), stock(empty), left(l1), left(l2)]))
aid1 = a1.get("id") or "00000000-0000-0000-0000-000000000000"
mv = rows(f"SELECT g.name || ':' || m.delta || ':' || m.reason || ':' || m.note FROM stock_moves m JOIN goods g ON g.id = m.good_id "
          f"WHERE m.inventory_id = '{aid1}' ORDER BY g.name")
note1 = f"инвентаризация №{no0 + 1} · z23i: пересчёт бара"
check("движения акта — две корректировки с номером акта; у сошедшихся движений нет",
      mv == [f"{PRE}Кола:-3:adjust:{note1}", f"{PRE}Чипсы:2:adjust:{note1}"], J(mv, 400))
fl = rows(f"SELECT kind || ':' || qty_in || ':' || COALESCE(price_grosz::text, 'нет') FROM stock_lots WHERE good_id = '{chips}' ORDER BY received_at")
check("излишек Чипсов — новая партия «нашлось» +2 без цены; сверка «партии = остаток» зелёная",
      fl == ["found:4:нет", "found:2:нет"] and api("GET", "/admin/goods/lots/check", None, "boss")[1].get("ok") is True, J(fl))
aud = [x.get("text", "") for x in audit("admin1", limit=30) if x.get("id") not in A0 and x.get("kind") == "inventory"]
check("журнал: «Инвентаризация №N · 4 позиции: сошлось 2, недостача — 1 позиция (3 шт), излишек — 1 позиция (2 шт) · заметка» — без денег",
      any(t.endswith("4 позиции: сошлось 2, недостача — 1 позиция (3 шт), излишек — 1 позиция (2 шт) · z23i: пересчёт бара") and "zł" not in t for t in aud),
      J(aud, 300))

section("владелец выбирает партию недостачи; админу — нельзя")
s2, d2 = inv({"note": "z23i: из второй партии", "lines": [L(cola, 9, 7, lot_id=l2)]}, "boss")
a2 = d2.get("inventory") or {}
c2 = ((a2.get("lines") or [{}])[0])
check("владелец: −2 из второй партии (2,50) — себестоимость 5 zł, партия в строке; номер следующий",
      s2 == 201 and a2.get("no") == no0 + 2 and (left(l1), left(l2), stock(cola)) == (3, 4, 7) and c2.get("cost_pln") == 5
      and [(x.get("kind"), x.get("qty"), x.get("price_pln")) for x in c2.get("lots") or []] == [("found", 2, 2.5)]
      and (a2.get("totals") or {}).get("cost_pln") == 5, J([s2, a2.get("no"), left(l1), left(l2), c2], 500))
n0 = acts_n()
s3, d3 = inv({"note": "z23i: админ с партией", "lines": [L(cola, 7, 6, lot_id=l1)]}, "admin1")
check("админ с партией — 403 lot_owner_only, ничего не записано", s3 == 403 and d3.get("code") == "lot_owner_only" and d3.get("line") == 0
      and acts_n() == n0 and stock(cola) == 7, J([s3, d3]))

section("пока считали, остаток изменился — акт не проводится")
s4, d4 = inv({"note": "z23i: устаревший", "lines": [L(water, 5, 4), L(cola, 8, 6)]}, "boss")
check("409 stock_moved: в списке Кола — ждали 8, в системе 7; Вода не тронута, акта нет",
      s4 == 409 and d4.get("code") == "stock_moved" and [(x.get("name"), x.get("expected"), x.get("stock")) for x in d4.get("lines") or []] == [(PRE + "Кола", 8, 7)]
      and "было 8, стало 7" in (d4.get("message") or "") and stock(water) == 5 and acts_n() == n0, J([s4, d4], 400))

section("строки с ошибками")
bad = [
    ("пустой акт — 400 bad_lines", {"lines": []}, 400, "bad_lines", None),
    ("позиция дважды — 400 dup_line, строка 1", {"lines": [L(water, 5, 5), L(water, 5, 4)]}, 400, "dup_line", 1),
    ("насчитали минус — 400 bad_line", {"lines": [L(water, 5, -1)]}, 400, "bad_line", 0),
    ("без «в системе» — 400 bad_line", {"lines": [{"good_id": water, "counted": 3}]}, 400, "bad_line", 0),
    ("чужой id — 404 good_not_found", {"lines": [L(water, 5, 5), L("8f0e6c43-1111-4c1e-9e55-0a0b0c0d0e0f", 1, 1)]}, 404, "good_not_found", 1),
    ("партия у излишка — 400 bad_inv_lot", {"lines": [L(cola, 7, 8, lot_id=l1)]}, 400, "bad_inv_lot", 0),
    ("партия другой позиции — 400 bad_inv_lot", {"lines": [L(water, 5, 4, lot_id=l1)]}, 400, "bad_inv_lot", 0),
    ("в партии меньше недостачи — 409 lot_short (осталось 3)", {"lines": [L(cola, 7, 2, lot_id=l1)]}, 409, "lot_short", 0),
    ("заметка длиннее 200 — 400 bad_note", {"note": "z23i " + "я" * 200, "lines": [L(water, 5, 5)]}, 400, "bad_note", None),
]
for name, body, st, code, line in bad:
    s_, d_ = inv(body, "boss")
    check(name, s_ == st and d_.get("code") == code and d_.get("line") == line and (code != "lot_short" or d_.get("left") == 3), J([s_, d_], 300))
check("после ошибок — ни акта, ни движения: актов как было, Кола 7, Вода 5", (acts_n(), stock(cola), stock(water)) == (n0, 7, 5),
      J([acts_n(), n0, stock(cola), stock(water)]))

section("два пересчёта разом — по одному")
res = {}


def race(who, cnt):
    res[who] = inv({"note": f"z23i: разом {who}", "lines": [L(water, 5, cnt)]}, who)


th = [threading.Thread(target=race, args=(w, c)) for w, c in (("boss", 4), ("admin1", 3))]
[t.start() for t in th]
[t.join() for t in th]
sts = sorted(v[0] for v in res.values())
win = [w for w, v in res.items() if v[0] == 201]
lose = [v[1] for v in res.values() if v[0] == 409]
want = {"boss": 4, "admin1": 3}.get(win[0] if win else "", -1)
check("один акт проведён, второй — 409 stock_moved: «в системе» уже другое; Вода — по проведённому",
      sts == [201, 409] and stock(water) == want and lose and (lose[0].get("lines") or [{}])[0].get("stock") == want, J([res, stock(water)], 500))
race_short = 5 - want if want > 0 else 0

section("акты и акт")
sl, dl = api("GET", "/admin/goods/inventories", None, "admin1")
mine = [x for x in dl.get("inventories") or [] if (x.get("note") or "").startswith("z23i")]
check("список (админ): акты проверки — новые первыми, итоги без себестоимости",
      sl == 200 and [x.get("no") for x in mine][-2:] == [no0 + 2, no0 + 1] and len(mine) == 3 and all("cost_pln" not in (x.get("totals") or {}) for x in mine)
      and (mine[-1].get("totals") or {}).get("short_units") == 3 and mine[-1].get("by") == "admin1", J([sl, mine], 500))
so, do = api("GET", f"/admin/goods/inventories/{aid1}", None, "boss")
oa = do.get("inventory") or {}
ol = {x.get("name"): x for x in oa.get("lines") or []}
oc, ch = ol.get(PRE + "Кола") or {}, ol.get(PRE + "Чипсы") or {}
check("акт владельцу: Кола — себестоимость 6 zł (3 из первой партии по 2,00), Чипсы — излишек в «нашлось» без цены, себестоимости нет",
      so == 200 and oc.get("cost_pln") == 6 and [(x.get("qty"), x.get("price_pln")) for x in oc.get("lots") or []] == [(3, 2)]
      and ch.get("cost_pln") is None and [(x.get("kind"), x.get("qty"), x.get("price_pln")) for x in ch.get("lots") or []] == [("found", 2, None)]
      and (oa.get("totals") or {}).get("cost_pln") == 6 and oa.get("by") == "admin1", J(oa, 600))
sa, da = api("GET", f"/admin/goods/inventories/{aid1}", None, "admin1")
aa = da.get("inventory") or {}
check("чужой акт — 404 inventory_not_found; админ видит акт — строки есть, себестоимости нет",
      api("GET", "/admin/goods/inventories/8f0e6c43-1111-4c1e-9e55-0a0b0c0d0e0f", None, "boss")[1].get("code") == "inventory_not_found"
      and sa == 200 and len(aa.get("lines") or []) == 4 and "cost_pln" not in (aa.get("totals") or {})
      and all("lots" not in x and "cost_pln" not in x for x in aa.get("lines") or []), J([sa, aa.get("totals")]))

section("«Потери → по инвентаризациям»")
LOSS1, INV1 = money_now()
dv = lambda k: (INV1.get(k) or 0) - (INV0.get(k) or 0)
check("акты за сегодня: +3; недостача +%d шт (3 + 2 + %d разом), по цене продажи +%s zł, по себестоимости +11 zł (6 + 5), без цены +%d шт; излишек +2 шт на 16 zł"
      % (5 + race_short, race_short, 25 + 3 * race_short, race_short),
      (dv("acts"), dv("short_units"), round(dv("short_pln"), 2), round(dv("cost_pln"), 2), dv("no_cost_units"), dv("surplus_units"), round(dv("surplus_pln"), 2))
      == (3, 5 + race_short, 25 + 3 * race_short, 11, race_short, 2, 16), J([INV0, {k: v for k, v in INV1.items() if k != "items"}], 500))
its = [x for x in INV1.get("items") or [] if x.get("no") in (no0 + 1, no0 + 2)]
check("по актам: номер, кто, недостача и себестоимость каждого",
      sorted((x.get("no"), x.get("by"), x.get("short_units"), x.get("cost_pln")) for x in its) == [(no0 + 1, "admin1", 3, 6), (no0 + 2, "boss", 2, 5)], J(its, 400))
check("недостачи актов уже в «Потерях» склада: штук +%d, по себестоимости +11 zł" % (5 + race_short),
      (LOSS1.get("units") or 0) - (LOSS0.get("units") or 0) == 5 + race_short
      and near((LOSS1.get("cost_pln") or 0) - (LOSS0.get("cost_pln") or 0), 11), J([LOSS0.get("units"), LOSS1.get("units"), LOSS0.get("cost_pln"), LOSS1.get("cost_pln")]))

section("файл акта — движком выгрузки")


def raw(aid, q, who="boss"):
    req = urllib.request.Request(f"{API}/admin/goods/inventories/{aid}/export?{q}")
    req.add_header("Authorization", "Bearer " + tok(who))
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.headers.get("Content-Type") or "", r.headers.get("Content-Disposition") or "", r.read()
    except urllib.error.HTTPError as e:
        return e.code, "", "", b""


def xlsx(data):
    try:
        z = zipfile.ZipFile(io.BytesIO(data))
    except zipfile.BadZipFile:
        return {}
    wb = ET.fromstring(z.read("xl/workbook.xml"))
    rels = {r.get("Id"): r.get("Target") for r in ET.fromstring(z.read("xl/_rels/workbook.xml.rels"))}
    out = {}
    for sh in wb.find("m:sheets", NS):
        table = []
        for row in ET.fromstring(z.read("xl/" + rels[sh.get(RID)])).find("m:sheetData", NS):
            cells = {}
            for c in row:
                n = 0
                for ch_ in c.get("r"):
                    if ch_.isalpha():
                        n = n * 26 + ord(ch_) - 64
                if c.get("t") == "inlineStr":
                    cells[n - 1] = "".join(t.text or "" for t in c.iter("{%s}t" % NS["m"]))
                else:
                    v = c.find("m:v", NS)
                    cells[n - 1] = float(v.text) if v is not None else None
            table.append([cells.get(i) for i in range(max(cells) + 1 if cells else 0)])
        out[sh.get("name")] = table
    return out


day = sql("SELECT to_char(now() AT TIME ZONE 'Europe/Warsaw', 'YYYY-MM-DD')")
B0 = seen("boss")
st, ct, cd, body = raw(aid1, "format=xlsx&lang=pl")
book = xlsx(body)
check(f"XLSX по-польски: 200, имя «1448-arkusz-spisu-{no0 + 1}-{day}.xlsx», листы «Arkusz spisu» и «Pozycje»",
      st == 200 and "spreadsheetml" in ct and f'filename="1448-arkusz-spisu-{no0 + 1}-{day}.xlsx"' in cd and list(book) == ["Arkusz spisu", "Pozycje"],
      J([st, ct, cd, list(book)]))
about = {r[0]: (r[1] if len(r) > 1 else None) for r in (book.get("Arkusz spisu") or [[]])[1:] if r}
check("лист «Arkusz spisu»: номер, кто провёл, недостача по себестоимости 6,00, излишек 2 шт",
      about.get("Arkusz spisu z natury") == f"nr {no0 + 1}" and about.get("Spis przeprowadził") == "admin1"
      and about.get("Niedobór wg cen zakupu") == 6 and about.get("Nadwyżka — sztuk") == 2 and about.get("Uwagi") == "z23i: пересчёт бара",
      J(about, 600))
pos = book.get("Pozycje") or [[]]
head = pos[0]
prow = {r[1]: dict(zip(head, r)) for r in pos[1:] if len(r) > 1 and r[1]}
kola = prow.get(PRE + "Кола") or {}
check("лист «Pozycje»: Кола — stan ewidencyjny 12, rzeczywisty 9, różnica −3, wartość −15,00, niedobór wg cen zakupu 6,00, partia; «Razem» внизу",
      head[:7] == ["Lp.", "Nazwa towaru", "J.m.", "Kategoria", "Stan ewidencyjny", "Stan rzeczywisty", "Różnica"]
      and (kola.get("Stan ewidencyjny"), kola.get("Stan rzeczywisty"), kola.get("Różnica"), kola.get("Wartość różnicy wg cen sprzedaży"),
           kola.get("Wartość niedoboru wg cen zakupu")) == (12, 9, -3, -15, 6)
      and "nadwyżka z " in (kola.get("Partie") or "") and (pos[-1] or [None])[0] == "Razem" and len(pos) == 6, J([head, kola, pos[-1]], 600))
st2_, ct2, cd2, body2 = raw(aid1, "format=csv&lang=ru")
try:
    zc = zipfile.ZipFile(io.BytesIO(body2))
    names = zc.namelist()
    csv2 = zc.read(names[1]).decode("utf-8-sig") if len(names) > 1 else ""
except zipfile.BadZipFile:
    names, csv2 = [], ""
check("CSV по-русски: архив из двух файлов, «Насчитали» в шапке", st2_ == 200 and "zip" in ct2 and ".zip" in cd2
      and names == ["01_akt.csv", "02_pozicii.csv"] and "Насчитали" in csv2, J([st2_, ct2, cd2, names, csv2[:200]], 400))
check("файл — только владельцу; формат и язык проверяются; чужой акт — 404",
      raw(aid1, "format=xlsx", "admin1")[0] == 403 and raw(aid1, "format=pdf")[0] == 400 and raw(aid1, "lang=de")[0] == 400
      and raw("8f0e6c43-1111-4c1e-9e55-0a0b0c0d0e0f", "format=xlsx")[0] == 404)
ex = [x.get("text", "") for x in audit("boss", limit=20) if x.get("id") not in B0 and x.get("kind") == "inventory_export"]
check("журнал выгрузки — «акт инвентаризации №N · XLSX · PL» и «· CSV · RU»",
      sorted(ex) == [f"акт инвентаризации №{no0 + 1} · CSV · RU", f"акт инвентаризации №{no0 + 1} · XLSX · PL"], J(ex))

done()
