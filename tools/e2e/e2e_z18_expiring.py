#!/usr/bin/env python3
"""З18-и4 «Настройки без интерфейса» (№39), API: «Сгорают скоро» — GET /admin/packages/expiring и «Предупредить сейчас»
— POST /admin/packages/warn (Е2-и5). Ревью 23.09: ручки есть, а админка их не зовёт; в списке нет ника гостя — кому
сейчас улетит предупреждение, не понять. Теперь у строки — ник, в ответе — сколько минут сгорает (minutes) и скольким
пакетам уведомление ещё не ушло (to_warn — столько и предупредит «Предупредить сейчас»). Пакеты Guest48 и Guest49 —
SQL-ом, подарком, задним числом (2020-01-01: в выручку и подарки дня не попадают); в начале и в конце удаляются вместе
со своими уведомлениями; pack_warn_days на время — 3, 15 и 0, потом как был."""
from _z import *

G1, G2 = "Guest48", "Guest49"
U1, U2 = uid(G1), uid(G2)
CLUB, ZONE = sql("SELECT club_id || '|' || id FROM zones WHERE name = 'Standard'").split("|")


def tidy():
    sql(f"DELETE FROM notifications WHERE user_id IN ('{U1}', '{U2}') AND type = 'package_expiring'")
    sql("DELETE FROM user_packages WHERE name LIKE 'z18e %'")


tidy()
at_end(tidy)
set_settings(pack_warn_days=3)


def mk(u, name, left, exp, warned=False, voided=False):
    return sql(f"""INSERT INTO user_packages (club_id, user_id, zone_id, name, zone_name, minutes_total, minutes_left,
        price_pln, method, comp_reason, expires_at, warned_at, voided_at, created_at)
        VALUES ('{CLUB}', '{u}', '{ZONE}', '{name}', 'Standard', 600, {left}, 45, 'comp', 'z18 проверка',
        now() + interval '{exp}', {'now()' if warned else 'NULL'}, {'now()' if voided else 'NULL'}, '2020-01-01')
        RETURNING id""")


mk(U1, "z18e 3 часа", 120, "2 days 3 hours")
mk(U1, "z18e Ночь", 300, "20 hours", warned=True)
mk(U2, "z18e Вечер", 45, "1 day 2 hours")
mk(U2, "z18e Месяц", 600, "10 days")
mk(U2, "z18e Отменён", 60, "1 day", voided=True)
mk(U1, "z18e Пустой", 0, "1 day")
mk(U1, "z18e Сгорел", 90, "-1 hour")
ours = lambda d: [x for x in d.get("packages", []) if str(x.get("name", "")).startswith("z18e ")]
expiring = lambda: api("GET", "/admin/packages/expiring", None, "boss")[1]
notes = lambda: rows(f"SELECT u.nickname || '|' || (n.payload->>'name') || '|' || (n.payload->>'minutes') || '|' || "
                     f"(n.payload->>'days_left') FROM notifications n JOIN users u ON u.id = n.user_id "
                     f"WHERE n.user_id IN ('{U1}', '{U2}') AND n.type = 'package_expiring' ORDER BY 1")
audits = lambda: rows("SELECT details FROM admin_actions WHERE action = 'package_warn' ORDER BY created_at DESC LIMIT 1")
acount = lambda: int(sql("SELECT count(*) FROM admin_actions WHERE action = 'package_warn'"))

section("«Сгорают скоро»: кто и сколько")
d = expiring()
o = ours(d)
check("три пакета, которым до срока 3 дня или меньше, по сроку: Ночь, Вечер, 3 часа; месячный, отменённый, пустой и "
      "сгоревший — нет", [x["name"] for x in o] == ["z18e Ночь", "z18e Вечер", "z18e 3 часа"] and d.get("warn_days") == 3,
      J([(x.get("name"), x.get("days_left")) for x in o]))
check("у строки — ник гостя и ушло ли уведомление: Guest48 «Ночь» — уже да, остальным — ещё нет",
      [(x.get("nickname"), x.get("warned")) for x in o] == [(G1, True), (G2, False), (G1, False)],
      J([(x.get("nickname"), x.get("warned")) for x in o]))
pk = d.get("packages", [])
check("итоги: minutes — сумма остатков (наших 465), to_warn — скольким ещё не ушло (наших 2)",
      d.get("minutes") == sum(x["minutes_left"] for x in pk) and d.get("to_warn") == sum(1 for x in pk if not x["warned"])
      and sum(x["minutes_left"] for x in o) == 465 and sum(1 for x in o if not x["warned"]) == 2,
      f"minutes {d.get('minutes')} to_warn {d.get('to_warn')} count {d.get('count')}")

section("«Предупредить сейчас»")
s, w = api("POST", "/admin/packages/warn", None, "boss")
d2 = expiring()
check("POST /admin/packages/warn — предупреждено столько, сколько было to_warn; Guest48 — «3 часа», 120 мин, через 3 дня; "
      "Guest49 — «Вечер», 45 мин, через 2 дня; «Ночь» второй раз не шлём",
      s == 200 and w.get("warned") == d.get("to_warn") and notes() == [f"{G1}|z18e 3 часа|120|3", f"{G2}|z18e Вечер|45|2"],
      f"{s} {J(w)} · {notes()}")
check("после — у всех «отправлено», to_warn 0; в журнале — «предупреждено о сгорании: N»",
      all(x.get("warned") for x in ours(d2)) and d2.get("to_warn") == 0
      and audits() == [f"предупреждено о сгорании: {w.get('warned')}"], f"{d2.get('to_warn')} · {audits()}")
n0, a0 = len(notes()), acount()
s, w2 = api("POST", "/admin/packages/warn", None, "boss")
check("ещё раз — 0: уведомление по пакету одно, новых строк в журнале и уведомлений нет",
      s == 200 and w2.get("warned") == 0 and len(notes()) == n0 and acount() == a0, f"{s} {J(w2)} · журнал +{acount() - a0}")

section("окно предупреждения — из настройки")
set_settings(pack_warn_days=15)
d3 = expiring()
check("pack_warn_days 15 — в списке и месячный (10 дней), ему ещё не ушло: to_warn 1",
      [x["name"] for x in ours(d3)] == ["z18e Ночь", "z18e Вечер", "z18e 3 часа", "z18e Месяц"] and d3.get("to_warn") == 1,
      J([x.get("name") for x in ours(d3)]) + f" · {d3.get('to_warn')}")
set_settings(pack_warn_days=0)
d4 = expiring()
s, w4 = api("POST", "/admin/packages/warn", None, "boss")
check("pack_warn_days 0 — предупреждения выключены: warn_days 0, пусто, итоги нулевые; «Предупредить сейчас» — 0",
      d4.get("warn_days") == 0 and d4.get("count") == 0 and d4.get("minutes") == 0 and d4.get("to_warn") == 0
      and s == 200 and w4.get("warned") == 0, f"{J(d4)} · {J(w4)}")
s1, _ = api("GET", "/admin/packages/expiring", None, "admin1")
s2, _ = api("POST", "/admin/packages/warn", None, "admin1")
check("админу — 403 на обе ручки (только владелец)", (s1, s2) == (403, 403), f"{s1} {s2}")
done()
