#!/usr/bin/env python3
"""З16-и1 «Снимок зоны и ставок в сессии» (№37, №38 «б», З-Р25). XP и монеты считались при завершении по ставкам этой
минуты: сессия 10 минут, за минуту до конца coins_per_min 2 → 100 — 1000 монет (сценарий M ревью); «акция на вечер»
раздавала монеты всем, кто сидел с обеда, а снижение резало обещанное калькулятором. Теперь у сессии — снимок на начало:
зона ПК (id и имя) и ставки XP и монет за минуту; по нему начисляет finishSession, а /me/economy гостю за ПК отдаёт
ставки его сессии (калькулятор киоска и «≈XP» в PWA). Смена зоны ПК под гостем и пересадка снимок не меняют. Миграция
091 даёт старым сессиям нынешнюю зону их ПК и нынешние ставки, повторный прогон трогает только сессии без снимка.
Гости Guest47 и Guest46: кошелёк докладывается до 30 zł, сессии списывают свои минуты как обычно; ставки — через
set_settings (возвращаются в конце), зона PC-08 возвращается в Standard."""
import math
import os
import subprocess
import urllib.request

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "migrations",
                   "091_session_snapshot.sql")
HINT = "Ставки за минуту игры действуют на новые сессии: уже начатые досчитываются по своим — они записаны в сессии."
G, H = "Guest47", "Guest46"
STD, VIP = sql("SELECT id FROM zones WHERE name = 'Standard'"), sql("SELECT id FROM zones WHERE name = 'VIP'")
PC8 = pcs()["PC-08"]["id"]
HAS = len(rows("SELECT 1 FROM information_schema.columns WHERE table_name = 'sessions' AND column_name IN "
               "('zone_id', 'zone_name', 'xp_per_min', 'coins_per_min')")) == 4
end_all(G, H)
at_end(lambda: end_all(G, H))
for n in (G, H):
    ensure_wallet(n, 30)


def snap(sid):
    """Снимок сессии «зона|id зоны|XP|монеты» за минуту; без колонок 091 — «нет снимка»."""
    if not HAS:
        return "нет снимка"
    return sql(f"SELECT zone_name || '|' || COALESCE(zone_id::text, '') || '|' || COALESCE(xp_per_min::text, '') || '|' "
               f"|| COALESCE(coins_per_min::text, '') FROM sessions WHERE id = '{sid}'")


def economy(nick):
    """/me/economy гостя: ((XP, монеты) за минуту, прибавка таланта к XP)."""
    d = api("GET", "/me/economy", None, nick)[1]
    r = d.get("rates") or {}
    return (r.get("xp_per_min"), r.get("coins_per_min")), (d.get("talents") or {}).get("xp_boost") or 0


rnd = lambda x: int(math.floor(x + 0.5))  # math.Round сервера (всё ≥ 0)
r5 = lambda v: (v + 2) // 5 * 5           # models.RoundToStep(v, 5)


def earned(d, xp_rate, coin_rate, boost):
    """Что начислит finishSession за минуты ответа по ставкам: (XP без бонуса дня, монеты) — формула sessionRewards."""
    m, rk = d.get("minutes") or 0, d.get("rank") or {}
    base = m * xp_rate
    boosted = rnd(base * (1 + boost)) if boost > 0 else base
    return r5(rnd(boosted * rk.get("xp_mult", 1))), r5(rnd(m * coin_rate * rk.get("coin_mult", 1)))


got = lambda d: ((d.get("xp_earned") or 0) - (d.get("daily_bonus_xp") or 0), d.get("coins_earned"))

section("сценарий M: ставки подняли посреди сессии")
set_settings(xp_per_min=10, coins_per_min=2)  # исходные ставки — явно, на любом стенде
sid = seat(G, "PC-08", minutes_ago=10)
s0 = snap(sid)
check("посадка за PC-08 записала снимок: зона Standard, 10 XP и 2 монеты за минуту", s0 == f"Standard|{STD}|10|2", s0)
s1, _ = set_settings(xp_per_min=100, coins_per_min=100)
gr, boost = economy(G)
check("ставки подняли до 100 и 100 посреди сессии (200) — /me/economy гостю за ПК отдаёт ставки его сессии, 10 и 2: "
      "калькулятор киоска и «≈XP» в PWA сходятся с начислением", s1 == 200 and gr == (10, 2), f"{s1} {gr}")
sz, _ = api("PATCH", f"/admin/computers/{PC8}", {"zone_id": VIP}, "boss")
at_end(lambda: api("PATCH", f"/admin/computers/{PC8}", {"zone_id": STD}, "boss"))
s2 = snap(sid)
sb, _ = api("PATCH", f"/admin/computers/{PC8}", {"zone_id": STD}, "boss")
check("PC-08 перевели в VIP под гостем и вернули (200, 200) — снимок сессии прежний: Standard, 10 и 2",
      (sz, sb) == (200, 200) and s2 == f"Standard|{STD}|10|2", f"{sz} {sb} {s2}")
se, de = end(sid)
want, high = earned(de, 10, 2, boost), earned(de, 100, 100, boost)
check(f"завершение (200): XP и монеты за {de.get('minutes')} мин — по снимку 10 и 2: +{want[0]} XP, +{want[1]} монет "
      f"(по нынешним 100 и 100 было бы +{high[0]} и +{high[1]})", se == 200 and got(de) == want and want != high,
      f"{se} {got(de)} · {J(de.get('rank'))}")
row = sql(f"SELECT coins_earned FROM sessions WHERE id = '{sid}'")
gr2, _ = economy(G)
check("в истории сессии те же монеты; после неё /me/economy — нынешние 100 и 100: по ним пойдёт следующая сессия",
      row == str(want[1]) and gr2 == (100, 100), f"{row} {gr2}")

section("ставки снизили посреди сессии, гостя пересадили на VIP")
sid2 = seat(H, "PC-09", minutes_ago=5)
s3 = snap(sid2)
check("новая посадка за PC-09 — по нынешним ставкам: снимок Standard, 100 и 100", s3 == f"Standard|{STD}|100|100", s3)
set_settings(xp_per_min=10, coins_per_min=2)
sm, _ = api("POST", f"/admin/sessions/{sid2}/move", {"computer_id": pcs()["VIP-02"]["id"]}, "admin1")
pc_now = sql(f"SELECT c.name FROM sessions s JOIN computers c ON c.id = s.computer_id WHERE s.id = '{sid2}'")
s4 = snap(sid2)
hr, hboost = economy(H)
check("ставки снизили до 10 и 2, гостя пересадили на VIP-02 (200) — снимок прежний: Standard, 100 и 100; "
      "/me/economy ему — 100 и 100", sm == 200 and pc_now == "VIP-02" and s4 == f"Standard|{STD}|100|100"
      and hr == (100, 100), f"{sm} {pc_now} {s4} {hr}")
se2, de2 = end(sid2)
want2, low2 = earned(de2, 100, 100, hboost), earned(de2, 10, 2, hboost)
check(f"завершение (200): за {de2.get('minutes')} мин — по снимку 100 и 100: +{want2[0]} XP, +{want2[1]} монет — "
      f"обещанное не урезано (по нынешним 10 и 2 было бы +{low2[0]} и +{low2[1]})",
      se2 == 200 and got(de2) == want2 and want2 != low2, f"{se2} {got(de2)} · {J(de2.get('rank'))}")

section("миграция 091: старым — нынешняя зона ПК и нынешние ставки; повторный прогон — только без снимка")


def migrate():
    if not os.path.exists(MIG):
        return -1, "нет файла"
    r = subprocess.run(["psql", "-v", "ON_ERROR_STOP=1", DB, "-q", "-f", MIG], capture_output=True, text=True,
                       env={**os.environ, "PGOPTIONS": "-c client_min_messages=warning"})  # без NOTICE «уже есть»
    return r.returncode, r.stderr.strip()[-200:]


OTHERS = (f"SELECT md5(string_agg(id || '|' || zone_name || '|' || COALESCE(zone_id::text, '') || '|' || "
          f"COALESCE(xp_per_min::text, '') || '|' || COALESCE(coins_per_min::text, ''), ',' ORDER BY id)) FROM sessions "
          f"WHERE id <> '{sid}'")
set_settings(xp_per_min=13, coins_per_min=7)
if HAS:  # сессия «как у прежней версии сервера» — без снимка
    sql(f"UPDATE sessions SET zone_id = NULL, zone_name = '', xp_per_min = NULL, coins_per_min = NULL WHERE id = '{sid}'")
o0 = sql(OTHERS) if HAS else ""
c1, e1 = migrate()
s5, o1 = snap(sid), (sql(OTHERS) if HAS else "")
check("сессии без снимка 091 дала нынешнюю зону её ПК и нынешние ставки: Standard, 13 и 7; снимки остальных не тронуты",
      c1 == 0 and s5 == f"Standard|{STD}|13|7" and o1 == o0, f"{c1} {e1} · {s5} · {o0 == o1}")
c2, e2 = migrate()
s6, o2 = snap(sid), (sql(OTHERS) if HAS else "")
check("повторный прогон 091 — без ошибок и ничего не меняет", c2 == 0 and s6 == s5 and o2 == o1, f"{c2} {e2} · {s6}")

section("«Экономика» говорит, с какого момента действуют ставки")
with urllib.request.urlopen(BASE + "/admin") as r:
    page = r.read().decode()
check(f"в карточке «Монеты и XP»: «{HINT}»", HINT in page)
done()
