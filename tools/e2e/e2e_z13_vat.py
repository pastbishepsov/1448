#!/usr/bin/env python3
"""З13-и1 «Ставки VAT: схема и снимки» (№72 «а»; решения основателя 04.10 — бухгалтерия под любого бухгалтера: вопросы
бухгалтеру — настройки владельца; только Польша; VAT у пополнения — настройкой, по умолчанию «при трате»).
Ставка VAT — у зоны, пакета и позиции (23, 8, 5, 0, zw; по умолчанию 23): ручки принимают и отдают `vat_pct`, ставка
не из списка — 400 bad_vat, менять — только владельцу; ставка всей категории позиций — одной кнопкой
(PUT /admin/vat/goods). Снимок ставки — в каждой операции на её момент: продажа у стойки, заказ кухни от стойки и его
расчёт, пополнение (ставка пополнения из настроек), выданный пакет, сессия (зона ПК); смена ставки потом снимок не
трогает; ПК без зоны — 23 % по умолчанию. Настройки «Бухгалтерии»: плательщик VAT, VAT у пополнения
(по умолчанию — при трате), ставка пополнения — только из списка ставок.
Данные: гость Guest43 — кошелёк докладывается до 30 zł; пополнение 5 zł картой, пакет картой, продажи и расчёт кухни
картой — всё отменяется в уборке («не возвращали»); две сессии за PC-08 завершаются сразу (на вторую ПК на минуту
остаётся без зоны — SQL-ом, зона возвращается); позиции «z13v Товар» и
«z13v Кухня» (категория «z13v») заводятся каждый прогон и в конце удаляются `drop_good`; ставки зоны PC-08 и самого
дешёвого пакета меняются по ходу и возвращаются; настройки возвращаются."""
from _z import *

CAT, MARK = "z13v", "z13v"
G43 = uid("Guest43")
PC = "PC-08"
ZONE = sql(f"SELECT zone_id FROM computers WHERE name = '{PC}'")
ZVAT0 = int(sql(f"SELECT vat_pct FROM zones WHERE id = '{ZONE}'") or 23)
PACK = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
PVAT0 = int(sql(f"SELECT vat_pct FROM time_packages WHERE id = '{PACK}'") or 23)
made = {"sales": [], "packs": [], "deps": []}


def gone():
    for sid in rows(f"""SELECT id FROM sales s WHERE name LIKE '{MARK} %' AND voided_at IS NULL
                         AND NOT EXISTS (SELECT 1 FROM kitchen_orders o WHERE o.sale_id = s.id)"""):  # оплаченный заказ кухни — пусть стоит
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for pid in made["packs"]:
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for did in made["deps"]:
        api("POST", f"/admin/deposits/{did}/void", {"refund_method": "none"}, "boss")
    end_all("Guest43")
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{MARK} %'"):
        drop_good(gid)
    sql(f"UPDATE computers SET zone_id = '{ZONE}' WHERE name = '{PC}'")  # упавший прогон — ПК снова в своей зоне
    sql(f"UPDATE zones SET vat_pct = {ZVAT0} WHERE id = '{ZONE}'")
    sql(f"UPDATE time_packages SET vat_pct = {PVAT0} WHERE id = '{PACK}'")


gone()
at_end(gone)
snap = lambda table, id_: sql(f"SELECT COALESCE(vat_pct::text, 'NULL') FROM {table} WHERE id = '{id_}'")

section("ставки: список, позиции, категория, зона, пакет, права")
s, r = api("GET", "/admin/vat/rates", None, "boss")
sa = api("GET", "/admin/vat/rates", None, "admin1")[0]
check("ставки на выбор — 23 %, 8 %, 5 %, 0 %, zw (−1), по умолчанию 23; админу список не нужен — 403",
      s == 200 and [x.get("vat_pct") for x in r.get("rates") or []] == [23, 8, 5, 0, -1]
      and [x.get("label") for x in r.get("rates") or []] == ["23 %", "8 %", "5 %", "0 %", "zw"] and r.get("default") == 23 and sa == 403, J([r, sa]))
new_good = lambda name, **kw: api("POST", "/admin/goods", {"name": name, "category": CAT, "price_pln": 6, "stock": 20, **kw}, "boss")
s1, a = new_good(f"{MARK} Товар", vat_pct=8)
s2, b = new_good(f"{MARK} Кухня")
s3, bad = new_good(f"{MARK} Плохая", vat_pct=7)
A, B = (a.get("good") or {}).get("id"), (b.get("good") or {}).get("id")
check("позиция со ставкой 8 % — 8, без ставки — 23 по умолчанию; ставка 7 — 400 bad_vat «Ставка VAT — одна из: …»",
      s1 == 201 and s2 == 201 and (a.get("good") or {}).get("vat_pct") == 8 and (b.get("good") or {}).get("vat_pct") == 23
      and s3 == 400 and bad.get("code") == "bad_vat" and "23 %, 8 %, 5 %, 0 %, zw" in (bad.get("message") or ""), J([a, b, bad]))
p1 = api("PATCH", f"/admin/goods/{B}", {"vat_pct": 5}, "boss")
p2 = api("PATCH", f"/admin/goods/{B}", {"vat_pct": 22}, "boss")
p3 = api("PATCH", f"/admin/goods/{B}", {"vat_pct": 0}, "admin1")[0]
p4 = api("PATCH", f"/admin/goods/{B}", {"price_pln": 7}, "boss")
check("правка ставки позиции: 5 % — сохранилась; 22 — 400; правка цены ставку не трогает; админ ставку не меняет — 403",
      p1[0] == 200 and (p1[1].get("good") or {}).get("vat_pct") == 5 and p2[0] == 400 and p2[1].get("code") == "bad_vat"
      and p3 == 403 and (p4[1].get("good") or {}).get("vat_pct") == 5 and snap("goods", B) == "5", J([p1, p2, p3, p4]))
k1 = api("PUT", "/admin/vat/goods", {"category": CAT, "vat_pct": 8}, "boss")
k2 = api("PUT", "/admin/vat/goods", {"category": CAT, "vat_pct": 9}, "boss")
k3 = api("PUT", "/admin/vat/goods", {"category": CAT, "vat_pct": 23}, "admin1")[0]
check(f"ставка всей категории «{CAT}» одной кнопкой: 8 % — поменялась одна позиция из двух (у второй уже 8); 9 — 400; админу — 403",
      k1[0] == 200 and k1[1].get("changed") == 1 and k1[1].get("total") == 2 and snap("goods", A) == "8" and snap("goods", B) == "8"
      and k2[0] == 400 and k3 == 403, J([k1, k2, k3]))
z1 = api("PATCH", f"/admin/zones/{ZONE}", {"vat_pct": 8}, "boss")
zl = next((z for z in api("GET", "/admin/zones", None, "admin1")[1].get("zones") or [] if z.get("id") == ZONE), {})
z2 = api("PATCH", f"/admin/zones/{ZONE}", {"vat_pct": 50}, "boss")[0]
check(f"зона ПК {PC}: ставка 8 % — в ответе и в списке зон; 50 — 400", z1[0] == 200 and (z1[1].get("zone") or {}).get("vat_pct") == 8
      and zl.get("vat_pct") == 8 and z2 == 400, J([z1, zl, z2]))
k = api("PATCH", f"/admin/packages/{PACK}", {"vat_pct": 5}, "boss")
pl = next((x for x in api("GET", "/admin/packages", None, "admin1")[1].get("packages") or [] if x.get("id") == PACK), {})
check("самый дешёвый пакет: ставка 5 % — в ответе и в каталоге", k[0] == 200 and (k[1].get("package") or {}).get("vat_pct") == 5
      and pl.get("vat_pct") == 5, J([k, pl]))

section("снимок ставки в операции")
sell = lambda gid: api("POST", "/admin/sales", {"good_id": gid, "qty": 1, "method": "card"}, "boss")[0]
q1 = sell(A)
api("PATCH", f"/admin/goods/{A}", {"vat_pct": 23}, "boss")
q2 = sell(A)
first = rows(f"SELECT COALESCE(vat_pct::text, 'NULL') FROM sales WHERE good_id = '{A}' ORDER BY created_at")
check("продажа у стойки — снимок ставки позиции (8 %); позицию перевели на 23 % — прошлая продажа осталась 8 %, новая — 23 %",
      q1 == 201 and q2 == 201 and first == ["8", "23"], J([q1, q2, first]))
s, d = api("POST", "/admin/kitchen/orders", {"guest_id": G43, "good_id": B, "qty": 1, "method": "card"}, "admin1")
oid, ksale = (d.get("order") or {}).get("id"), d.get("sale_id")
if oid:
    api("POST", f"/admin/kitchen/orders/{oid}/status", {"status": "done"}, "admin1")
v = api("POST", f"/admin/sales/{ksale}/void", {"refund_method": "none"}, "boss")[0] if ksale else 0
api("PATCH", f"/admin/goods/{B}", {"vat_pct": 5}, "boss")
pay = api("POST", f"/admin/kitchen/orders/{oid}/pay", {"method": "card"}, "admin1") if oid else (0, {})
psale = (pay[1] or {}).get("sale_id")
check("заказ кухни от стойки картой — снимок ставки позиции (8 %); после отмены оплаты позицию перевели на 5 % — расчёт заказа "
      "у стойки снимает уже 5 %", s == 201 and snap("sales", ksale) == "8" and v == 200 and pay[0] == 200 and snap("sales", psale) == "5",
      J([s, d, v, pay, snap("sales", ksale) if ksale else None]))
set_settings(acc_topup_vat=8)
dep = topup("Guest43", 5)
made["deps"].append(dep)
set_settings(acc_topup_vat=23)
dep2 = topup("Guest43", 5)
made["deps"].append(dep2)
check("пополнение — снимок ставки пополнения из настроек: 8 % при настройке 8, 23 % после возврата на 23 — у первого осталось 8",
      snap("deposits", dep) == "8" and snap("deposits", dep2) == "23", J([snap("deposits", dep), snap("deposits", dep2)]))
up = (api("POST", f"/admin/users/{G43}/packages", {"package_id": PACK, "method": "card"}, "boss")[1].get("package") or {}).get("id")
if up:
    made["packs"].append(up)
api("PATCH", f"/admin/packages/{PACK}", {"vat_pct": PVAT0}, "boss")
check(f"выданный пакет — снимок ставки пакета (5 %); пакету вернули {PVAT0} % — выданный остался с 5 %",
      bool(up) and snap("user_packages", up) == "5" and snap("time_packages", PACK) == str(PVAT0), J([up, snap("user_packages", up) if up else None]))
ensure_wallet("Guest43", 30)
sid = seat("Guest43", PC)
zsnap = snap("sessions", sid)
end(sid)
api("PATCH", f"/admin/zones/{ZONE}", {"vat_pct": ZVAT0}, "boss")
check(f"сессия за {PC} — снимок ставки зоны (8 %); зоне вернули {ZVAT0} % — у сессии осталось 8 %",
      zsnap == "8" and snap("sessions", sid) == "8", J([sid, zsnap]))
sql(f"UPDATE computers SET zone_id = NULL WHERE name = '{PC}'")  # ПК без зоны — на минуту, в конце зона вернётся
try:
    sid2 = seat("Guest43", PC)
    nosnap = snap("sessions", sid2)
    end(sid2)
finally:
    sql(f"UPDATE computers SET zone_id = '{ZONE}' WHERE name = '{PC}'")
check(f"сессия за {PC} без зоны — снимок 23 % (ставка по умолчанию): брать ставку больше неоткуда", nosnap == "23", J([sid2, nosnap]))

section("настройки «Бухгалтерии»")
st = settings()
check("в настройках владельца — плательщик VAT, VAT у пополнения (0 — при трате), ставка пополнения",
      all(k in st for k in ("acc_vat_payer", "acc_vat_moment", "acc_topup_vat")), J({k: st.get(k) for k in ("acc_vat_payer", "acc_vat_moment", "acc_topup_vat")}))
r1 = set_settings(acc_vat_moment=1)
r2 = set_settings(acc_vat_moment=2)
r3 = set_settings(acc_topup_vat=7)
r4 = set_settings(acc_topup_vat=-1, acc_vat_payer=0)
r5 = api("PUT", "/admin/settings", {"acc_vat_moment": 0}, "admin1")[0]
now = settings()
check("«при пополнении» (1) — сохраняется, 2 — 400; ставка пополнения 7 — 400 «Ставка VAT — одна из…», zw (−1) — можно; "
      "«не плательщик VAT» — можно; админу — 403",
      r1[0] == 200 and r2[0] == 400 and r3[0] == 400 and "Ставка VAT" in (r3[1].get("message") or "") and r4[0] == 200
      and r5 == 403 and now.get("acc_vat_moment") == 1 and now.get("acc_topup_vat") == -1 and now.get("acc_vat_payer") == 0,
      J([r1, r2, r3, r4, r5, {k: now.get(k) for k in ("acc_vat_payer", "acc_vat_moment", "acc_topup_vat")}]))
done()
