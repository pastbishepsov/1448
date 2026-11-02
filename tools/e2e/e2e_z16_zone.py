#!/usr/bin/env python3
"""З16-и2 «Биллинг и прогноз по снимку» (№38 «б»). Пакеты, зонные минуты, прогноз и выдача минут «сюда» брали зону
машины, за которой гость сидит сейчас: ПК перевели в VIP под гостем с пакетом Standard — пакет переставал тратиться,
платил кошелёк, а прогноз обещал меньше. Теперь зона сессии — её снимок (З16-и1): PC-08 переводят в VIP, пока за ним
Guest46 с пакетом и минутами Standard, — минуты и пакет тратятся дальше, прогноз в зале, в карточке и у гостя — по ним,
минуты за монеты без зоны идут в Standard. Пересадка за VIP-03 с прежним тарифом — тоже по пакету Standard; с тарифом
VIP — сыгранное до неё пакетом Standard, после — кошельком (пакета VIP у гостя нет). Guest46: кошелёк докладывается до
30 zł; пакет Standard — подарком от boss, минуты за монеты — 5 в Standard и 1 без зоны; в конце пакет и минуты, что
остались, отменяются; PC-08 — снова Standard с прежним MAC (SQL-ом, и при падении тоже)."""
import time

from _z import *

G = "Guest46"
g = uid(G)
STD, VIP = sql("SELECT id FROM zones WHERE name = 'Standard'"), sql("SELECT id FROM zones WHERE name = 'VIP'")
PACK = sql(f"SELECT id FROM time_packages WHERE active AND zone_id = '{STD}' ORDER BY minutes, name LIMIT 1")
PNAME, PMIN = sql(f"SELECT name FROM time_packages WHERE id = '{PACK}'"), int(sql(f"SELECT minutes FROM time_packages WHERE id = '{PACK}'") or 0)
PC8 = pcs()["PC-08"]["id"]
MAC0 = sql(f"SELECT COALESCE(mac, '') FROM computers WHERE id = '{PC8}'")
mac_sql = f"'{MAC0}'" if MAC0 else "NULL"
at_end(lambda: sql(f"UPDATE computers SET zone_id = '{STD}', zone = 'Standard', mac = {mac_sql} WHERE id = '{PC8}'"))


def tidy():
    """Пакеты и минуты за монеты Guest46, что ещё не потрачены (эти и хвосты упавшего прогона), — отменить."""
    for p in rows(f"SELECT id FROM user_packages WHERE user_id = '{g}' AND voided_at IS NULL AND minutes_left > 0"):
        api("POST", f"/admin/user-packages/{p}/void", {"refund_method": "none"}, "boss")
    for r in rows(f"SELECT r.id FROM coin_redemptions r WHERE r.user_id = '{g}' AND r.voided_at IS NULL AND EXISTS "
                  f"(SELECT 1 FROM minute_grants mg WHERE mg.ref_id = r.id AND mg.minutes_left > 0)"):
        api("POST", f"/admin/coin-redemptions/{r}/void", {}, "boss")


end_all(G)
tidy()
at_end(tidy)
at_end(lambda: end_all(G))
ensure_wallet(G, 30)
cost = lambda rate_pln, m: (round(rate_pln * 100) * m + 59) // 60  # costForMinutes: вверх до гроша
mv = lambda sid, pc, **kw: api("POST", f"/admin/sessions/{sid}/move", {"computer_id": pcs()[pc]["id"], **kw}, "admin1")
pc_zone = lambda: sql(f"SELECT COALESCE(zone, '') FROM computers WHERE id = '{PC8}'")
back = lambda sid, m: sql(f"UPDATE sessions SET started_at = now() - interval '{m} minutes 10 seconds', "
                          f"ready_at = now() - interval '{m} minutes 10 seconds' WHERE id = '{sid}'")


def money(sid):
    """«учтено|минуты пакета|минуты деньгами|гроши|зона снимка» сессии."""
    return sql(f"SELECT billed_minutes || '|' || pack_minutes_used || '|' || money_minutes || '|' || charged_grosz || '|' || "
               f"zone_name FROM sessions WHERE id = '{sid}'")


def hall(pc):
    """Сессия ПК в зале (GET /admin/computers): прогноз и пометка тарифа."""
    pcs_ = api("GET", "/admin/computers", None, "admin1")[1].get("computers") or []
    return next((p.get("session") or {} for p in pcs_ if p["name"] == pc), {})


section("ПК перевели в VIP под гостем")
CLEAN = sql(f"SELECT u.coin_minutes = 0 AND NOT EXISTS (SELECT 1 FROM user_packages p WHERE p.user_id = u.id AND "
            f"p.voided_at IS NULL AND p.minutes_left > 0) AND NOT EXISTS (SELECT 1 FROM minute_grants mg WHERE "
            f"mg.user_id = u.id AND mg.minutes_left > 0) FROM users u WHERE u.id = '{g}'") == "t"
s1, d1 = api("POST", f"/admin/users/{g}/packages", {"package_id": PACK, "method": "comp", "comp_reason": "z16 зона сессии"}, "boss")
up = (d1.get("package") or {}).get("id")
s2, d2 = api("POST", f"/admin/users/{g}/redeem", {"zone_id": STD, "minutes": 5}, "boss")
check(f"у Guest46 запаса нет; пакет «{PNAME}» ({PMIN} мин) подарком — 201, 5 минут за монеты в Standard — 200",
      CLEAN and PMIN > 30 and s1 == 201 and bool(up) and s2 == 200 and d2.get("zone") == "Standard",
      f"{CLEAN} {PMIN} {s1} {J(d1, 120)} {s2} {J(d2, 120)}")
sid = seat(G, "PC-08")
snap = sql(f"SELECT zone_name FROM sessions WHERE id = '{sid}'")
s3, _ = api("PATCH", f"/admin/computers/{PC8}", {"zone_id": VIP}, "boss")
check("посадка за PC-08 — снимок зоны Standard; PC-08 переведён в VIP под гостем — 200", snap == "Standard" and s3 == 200
      and pc_zone() == "VIP", f"{snap} {s3} {pc_zone()}")
h = hall("PC-08")
card = api("GET", f"/admin/users/{g}", None, "admin1")[1].get("active_session") or {}
gt = login(G)[1].get("access_token")
me = api("GET", "/me/sessions", token=gt)[1]
seen = lambda x: (x.get("pack_minutes"), x.get("zone_minutes"))
check(f"прогноз — по зоне сессии: в зале, в карточке и у гостя пакет {PMIN} мин и 5 минут Standard; зона в карточке и у "
      f"гостя — Standard (туда — выдача минут), в зале — «тариф Standard»",
      seen(h) == seen(card) == seen(me) == (PMIN, 5) and h.get("tariff_zone") == "Standard"
      and card.get("zone") == "Standard" and me.get("zone") == "Standard",
      f"зал {seen(h)} {h.get('tariff_zone')} · карточка {seen(card)} {card.get('zone')} · гость {seen(me)} {me.get('zone')}")
s4, d4 = api("POST", f"/admin/users/{g}/redeem", {"zone_id": "", "minutes": 1}, "boss")
check("минута за монеты без зоны, пока гость за PC-08 (уже VIP), — в зону его сессии: Standard", s4 == 200
      and d4.get("zone") == "Standard", f"{s4} {J(d4, 160)}")
w0 = wallet(G)
back(sid, 12)
deadline = time.time() + 45
while time.time() < deadline and int(sql(f"SELECT billed_minutes FROM sessions WHERE id = '{sid}'")) < 12:
    time.sleep(2)
b, cm, pm, mm, ch = (int(x) for x in sql(f"SELECT billed_minutes || '|' || coin_minutes_used || '|' || pack_minutes_used "
                                          f"|| '|' || money_minutes || '|' || charged_grosz FROM sessions WHERE id = '{sid}'").split("|"))
check(f"тик биллинга за PC-08 в VIP: {b} мин — 6 минут Standard за монеты и {pm} пакета Standard, кошелёк не тронут",
      b >= 12 and cm == 6 and pm == b - 6 and mm == 0 and ch == 0 and wallet(G) == w0, f"{b}|{cm}|{pm}|{mm}|{ch} · {w0} → {wallet(G)}")
s5, _ = end(sid)
b1, p1, m1, c1, z1 = money(sid).split("|")
spent = int(sql(f"SELECT COALESCE(SUM(minutes), 0) FROM pack_spends WHERE session_id = '{sid}'"))
left1 = int(sql(f"SELECT minutes_left FROM user_packages WHERE id = '{up}'") or -1)
check("завершение — тот же расчёт: деньгами 0; минуты пакета — в его расходе по сессии, у пакета осталось столько же меньше",
      s5 == 200 and (m1, c1, z1) == ("0", "0", "Standard") and int(p1) == spent and left1 == PMIN - spent and wallet(G) == w0,
      f"{s5} {b1}|{p1}|{m1}|{c1}|{z1} · расход {spent} · осталось {left1} · кошелёк {w0} → {wallet(G)}")
s6, _ = api("PATCH", f"/admin/computers/{PC8}", {"zone_id": STD}, "boss")
check("PC-08 снова в Standard — 200", s6 == 200 and pc_zone() == "Standard", f"{s6} {pc_zone()}")

section("пересадка за VIP-03")
sid2 = seat(G, "PC-07")
s7, d7 = mv(sid2, "VIP-03", tariff="keep", reason="z16: пакет Standard на VIP")
h2 = hall("VIP-03")
back(sid2, 7)
s8, _ = end(sid2)  # завершение досчитывает и начатую минуту
b2, p2, m2, c2, z2 = money(sid2).split("|")
left2 = int(sql(f"SELECT minutes_left FROM user_packages WHERE id = '{up}'") or -1)
check(f"с прежним тарифом: в зале пакет Standard {left1} мин; все {b2} мин сессии — пакетом Standard, деньгами 0",
      s7 == 200 and d7.get("tariff") == "keep" and h2.get("pack_minutes") == left1 and s8 == 200 and int(b2) >= 7
      and p2 == b2 and (m2, c2, z2) == ("0", "0", "Standard") and left2 == left1 - int(b2),
      f"{s7} {J(d7, 120)} · зал {seen(h2)} · {s8} {money(sid2)} · пакет {left1} → {left2}")
sid3 = seat(G, "PC-07", minutes_ago=5)
s9, d9 = mv(sid3, "VIP-03", tariff="new")  # сыгранные 5 минут досчитаны до пересадки
h3 = hall("VIP-03")
rate = float(sql(f"SELECT effective_rate_pln FROM sessions WHERE id = '{sid3}'"))
back(sid3, 9)
s10, _ = end(sid3)
b3, p3, m3, c3, z3 = money(sid3).split("|")
left3 = int(sql(f"SELECT minutes_left FROM user_packages WHERE id = '{up}'") or -1)
check(f"с тарифом VIP: 5 сыгранных до пересадки минут — пакетом Standard; после неё пакет Standard не тратится — в зале "
      f"его нет, остальные {m3} мин — кошельком по {rate:.2f} zł/ч".replace(".", ","),
      s9 == 200 and d9.get("tariff") == "new" and seen(h3) == (0, 0) and s10 == 200 and p3 == "5" and int(b3) >= 9
      and int(m3) == int(b3) - 5 and int(c3) == cost(rate, int(m3)) and z3 == "VIP" and left3 == left2 - 5,
      f"{s9} {J(d9, 120)} · зал {seen(h3)} · {s10} {money(sid3)} · пакет {left2} → {left3}")
done()
