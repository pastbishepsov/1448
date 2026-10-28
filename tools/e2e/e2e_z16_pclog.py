#!/usr/bin/env python3
"""З16-и1б «Журнал смены зоны ПК» (из «Найдено по дороге» З16-и1, вопрос 2, решение основателя 28.10). Перевод ПК в
другую зону писал в журнал « · зона VIP» — без имени ПК и прежней зоны (имя попадало, только если его меняли вместе с
зоной), а сохранение с той же зоной и тем же MAC — ещё одну строку «ПК изменён». Теперь — «PC-08: зона Standard → VIP»,
с переименованием — «PC-08z → PC-08 · зона VIP → Standard»; без зоны — словами; та же зона и тот же MAC — ни правки, ни
строки. PC-08 на время проверки — в VIP, без зоны и под именем PC-08z; в конце — PC-08 в Standard с прежним MAC, как
был (SQL-ом, и при падении тоже); денег не трогает."""
from _z import *

PC8 = pcs()["PC-08"]["id"]
STD, VIP = sql("SELECT id FROM zones WHERE name = 'Standard'"), sql("SELECT id FROM zones WHERE name = 'VIP'")
MAC0 = sql(f"SELECT COALESCE(mac, '') FROM computers WHERE id = '{PC8}'")
mac_sql = f"'{MAC0}'" if MAC0 else "NULL"
at_end(lambda: sql(f"UPDATE computers SET name = 'PC-08', zone_id = '{STD}', zone = 'Standard', mac = {mac_sql} "
                   f"WHERE id = '{PC8}'"))
t0 = sql("SELECT now()::text")
log = lambda: rows(f"SELECT details FROM admin_actions WHERE action = 'pc_update' AND created_at >= '{t0}' "
                   f"ORDER BY created_at")
pc = lambda: sql(f"SELECT name || '|' || COALESCE(zone, '') || '|' || COALESCE(zone_id::text, '') FROM computers "
                 f"WHERE id = '{PC8}'")
patch = lambda **body: api("PATCH", f"/admin/computers/{PC8}", body, "boss")

section("перевод в другую зону")
s1, _ = patch(zone_id=VIP)
check("PC-08 в VIP — 200, в журнале «PC-08: зона Standard → VIP»", s1 == 200 and pc() == f"PC-08|VIP|{VIP}"
      and log() == ["PC-08: зона Standard → VIP"], f"{s1} {pc()} · {J(log())}")
s2, d2 = patch(name="PC-08", zone_id=VIP, mac=MAC0)  # как «Сохранить» в редакторе зала без правок
s3, d3 = patch(zone_id=VIP)
check("сохранение без правок (то же имя, та же зона, тот же MAC) и та же зона ещё раз — 200 «без изменений», новых "
      "строк нет", (s2, s3) == (200, 200) and d2.get("unchanged") is True and d3.get("unchanged") is True
      and len(log()) == 1, f"{s2} {J(d2)} {s3} {J(d3)} · {J(log())}")

section("переименование и без зоны")
s4, _ = patch(name="PC-08z", zone_id=VIP, mac=MAC0)
s5, _ = patch(name="PC-08", zone_id=STD, mac=MAC0)
check("переименование с той же зоной — «PC-08 → PC-08z», обратно со сменой зоны — «PC-08z → PC-08 · зона VIP → "
      "Standard»", (s4, s5) == (200, 200) and log()[1:] == ["PC-08 → PC-08z", "PC-08z → PC-08 · зона VIP → Standard"]
      and pc() == f"PC-08|Standard|{STD}", f"{s4} {s5} {J(log()[1:])} · {pc()}")
s6, _ = patch(zone_id="")
s7, _ = patch(zone_id=STD)
check("без зоны и обратно — «PC-08: зона Standard → без зоны», «PC-08: без зоны → зона Standard»; ПК снова в Standard",
      (s6, s7) == (200, 200) and log()[3:] == ["PC-08: зона Standard → без зоны", "PC-08: без зоны → зона Standard"]
      and pc() == f"PC-08|Standard|{STD}", f"{s6} {s7} {J(log()[3:])} · {pc()}")
done()
