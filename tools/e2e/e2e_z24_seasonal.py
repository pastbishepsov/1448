#!/usr/bin/env python3
"""З24-и5 «Сезонные пакеты — сервер» (№55): у пакета — окно продажи (даты), «не больше N на гостя», «только новым» (первое
появление не раньше seg_new_days суток назад, З5-и1), дни и часы списания. Пакет «не сегодня» (как «будни до 17:00» в
субботу) и «не сейчас» (окно через два часа) не тратятся: сессия идёт за счёт пакета «всегда», без него — кошельком,
а их минуты целы; прогноз их не считает. Вне окна продажи — 409 «продаётся с …» / «продажа закончилась …», сверх лимита —
409 «не больше N на гостя» (отменённая выдача не в счёт; сам гость в приложении — тоже 409), «только новым» старому — 409.
Каталог гостя — только то, что этот гость может купить сейчас. Окно списания выдачи — снимок: правка каталога его не
меняет. Проверки полей, журнал.
Данные: временная зона «z24s Сезон» (10 zł/ч) и ПК z24s-1 без агента, пакеты «z24s …»; гости z24sp (сессии; кошелёк — до
30 zł картой), z24sn («новый»: регистрация — сейчас, сессий нет) и z24so («старый»: регистрация — 60 дней назад, сессий
нет) — заводятся один раз, регистрацию проверка ставит сама. Продажи — картой и подарком, отмены — без возврата. В конце
выдачи, расход, сессии на z24s-1, пакеты, ПК и зона удаляются SQL-ом."""
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

from _z import *

GP, GN, GO = "z24sp", "z24sn", "z24so"
for g in (GP, GN, GO):
    if not uid(g):
        s, d = api("POST", "/auth/register", {"nickname": g, "password": GUEST_PASS})
        if s not in (200, 201):
            raise SystemExit(f"регистрация {g}: {s} {J(d)}")
U = {g: uid(g) for g in (GP, GN, GO)}
ids = ",".join(f"'{u}'" for u in U.values())


def wipe():
    end_all(GP)
    sql(f"DELETE FROM pack_spends WHERE user_package_id IN (SELECT id FROM user_packages WHERE user_id IN ({ids}))")
    sql(f"DELETE FROM user_packages WHERE user_id IN ({ids})")
    sql("DELETE FROM time_packages WHERE name LIKE 'z24s %'")
    sql("DELETE FROM sessions WHERE computer_id IN (SELECT id FROM computers WHERE name LIKE 'z24s-%')")
    sql("DELETE FROM computers WHERE name LIKE 'z24s-%'")
    sql("DELETE FROM zones WHERE name LIKE 'z24s %'")


wipe()
at_end(wipe)
sql(f"UPDATE users SET registered_at = now() WHERE id = '{U[GN]}'")
sql(f"UPDATE users SET registered_at = now() - interval '60 days' WHERE id = '{U[GO]}'")
ensure_wallet(GP, 30)
ensure_wallet(GN, 10)
seen = lambda: {x.get("id") for x in audit("boss", limit=80)}
A0 = seen()
W = ZoneInfo("Europe/Warsaw")
now = datetime.now(W)
today_bit = 1 << now.weekday()  # пн = бит 0
day = lambda d: (now + timedelta(days=d)).strftime("%Y-%m-%d")
dru = lambda d: (now + timedelta(days=d)).strftime("%d.%m.%Y")
nm = int(sql("SELECT (extract(hour FROM now() AT TIME ZONE 'Europe/Warsaw') * 60 + extract(minute FROM now() AT TIME ZONE 'Europe/Warsaw'))::int"))
hm = lambda m: f"{m // 60 % 24:02d}:{m % 60:02d}"
cs, ce = (nm + 120) % 1440, (nm + 180) % 1440 or 1440

zid = (api("POST", "/admin/zones", {"name": "z24s Сезон", "rate_pln": 10}, "boss")[1].get("zone") or {}).get("id")
pc = api("POST", "/admin/computers", {"name": "z24s-1", "zone_id": zid}, "boss")[1].get("computer_id")


def mkpack(name, **kw):
    s, d = api("POST", "/admin/packages", {"name": "z24s " + name, "zone_id": zid, "minutes": 60, "price_pln": 5, **kw}, "boss")
    return s, d.get("package") or d


P = {}
for name, kw in (("Не сегодня", {"spend_days_mask": 127 & ~today_bit}), ("Не сейчас", {"spend_start_min": cs, "spend_end_min": ce}),
                 ("Всегда", {}), ("Завтра", {"sell_from": day(1)}), ("Вчера", {"sell_to": day(-1)}), ("Один", {"per_guest": 1}),
                 ("Новым", {"new_only": True})):
    s, d = mkpack(name, **kw)
    P[name] = d
check("зона «z24s Сезон», ПК z24s-1 и семь пакетов «z24s …» — 201", bool(zid) and bool(pc) and all(p.get("id") for p in P.values()),
      J({k: v.get("id") or v for k, v in P.items()}, 600))
check(f"каталог: «Не сейчас» — «тратится каждый день {hm(cs)}–{hm(ce)}», «Завтра» — «продаётся с {dru(1)}» и не в продаже, "
      "«Один» — «1 на гостя», «Новым» — «только новым», «Всегда» — без условий",
      P["Не сейчас"].get("season_text") == f"тратится каждый день {hm(cs)}–{hm(ce) if ce != 1440 else '24:00'}"
      and P["Завтра"].get("season_text") == f"продаётся с {dru(1)}" and P["Завтра"].get("on_sale") is False
      and P["Один"].get("season_text") == "1 на гостя" and P["Новым"].get("season_text") == "только новым"
      and P["Всегда"].get("season_text") == "" and P["Всегда"].get("on_sale") is True and P["Не сегодня"].get("spend_days_mask") == 127 & ~today_bit,
      J({k: (v.get("season_text"), v.get("on_sale")) for k, v in P.items()}, 600))

section("окно списания: «не сегодня» и «не сейчас» не тратятся")
issue = lambda g, p, **kw: api("POST", f"/admin/users/{U[g]}/packages", {"package_id": P[p]["id"], **kw}, "boss")
gift = lambda g, p: issue(g, p, method="comp", comp_reason="z24s: проверка сезонных пакетов")
ups = {p: (gift(GP, p)[1].get("package") or {}) for p in ("Не сегодня", "Не сейчас", "Всегда")}
mine = {x.get("name"): x for x in api("GET", f"/admin/users/{U[GP]}/packages", None, "admin1")[1].get("packages") or []}
check("подарком z24sp — три пакета; у «Не сегодня» и «Не сейчас» — когда тратится и «не сейчас», у «Всегда» — без окна",
      all(u.get("id") for u in ups.values()) and mine.get("z24s Не сегодня", {}).get("spend_now") is False
      and mine.get("z24s Не сейчас", {}).get("spend_text") == f"каждый день {hm(cs)}–{hm(ce) if ce != 1440 else '24:00'}"
      and mine.get("z24s Не сейчас", {}).get("spend_now") is False and "spend_text" not in mine.get("z24s Всегда", {}),
      J({k: (v.get("spend_text"), v.get("spend_now")) for k, v in mine.items()}, 500))
left = lambda p: int(sql(f"SELECT minutes_left FROM user_packages WHERE id = '{ups[p]['id']}'") or -1)
spent = lambda sid: sql(f"SELECT string_agg(up.name || ':' || ps.minutes, ',' ORDER BY up.name) FROM pack_spends ps "
                        f"JOIN user_packages up ON up.id = ps.user_package_id WHERE ps.session_id = '{sid}'")
w0 = wallet(GP)
s_seat, d_seat = api("POST", f"/admin/computers/{pc}/session", {"guest": GP}, "admin1")
sid = d_seat.get("session_id")
fc = (api("GET", f"/admin/users/{U[GP]}", None, "admin1")[1].get("active_session") or {}).get("pack_minutes")
sql(f"UPDATE sessions SET started_at = now() - interval '10 minutes 10 seconds', ready_at = now() - interval '10 minutes 10 seconds' WHERE id = '{sid}'")
s_end, _ = end(sid)
row = sql(f"SELECT pack_minutes_used || '|' || money_minutes FROM sessions WHERE id = '{sid}'")
used = int(row.split("|")[0] or 0)  # завершение досчитывает и начатую минуту: 10 мин 10 с — 11
check("сессия 10 минут — пакетом «Всегда», «Не сегодня» и «Не сейчас» целы (по 60), кошелёк не тронут; прогноз — 60 минут пакета",
      s_seat == 201 and s_end == 200 and used >= 10 and row == f"{used}|0" and spent(sid) == f"z24s Всегда:{used}" and left("Не сегодня") == 60
      and left("Не сейчас") == 60 and wallet(GP) == w0 and fc == 60, J([s_seat, s_end, row, spent(sid), left("Не сегодня"), left("Не сейчас"), fc]))
api("POST", f"/admin/user-packages/{ups['Всегда']['id']}/void", {"refund_method": "none"}, "boss")
s_seat2, d_seat2 = api("POST", f"/admin/computers/{pc}/session", {"guest": GP}, "admin1")
sid2 = d_seat2.get("session_id")
fc2 = (api("GET", f"/admin/users/{U[GP]}", None, "admin1")[1].get("active_session") or {}).get("pack_minutes")
sql(f"UPDATE sessions SET started_at = now() - interval '5 minutes 10 seconds', ready_at = now() - interval '5 minutes 10 seconds' WHERE id = '{sid2}'")
s_end2, _ = end(sid2)
row2 = sql(f"SELECT pack_minutes_used || '|' || money_minutes FROM sessions WHERE id = '{sid2}'")
check("«Всегда» отменён — сессия 5 минут кошельком: пакеты «не сегодня» и «не сейчас» не тронуты, прогноз — 0 минут пакета",
      s_seat2 == 201 and s_end2 == 200 and row2.startswith("0|") and int(row2.split("|")[1]) >= 5 and (spent(sid2) or "") == ""
      and left("Не сегодня") == 60 and left("Не сейчас") == 60 and wallet(GP) < w0 and fc2 == 0, J([row2, spent(sid2), wallet(GP) - w0, fc2]))
sp, _ = api("PATCH", f"/admin/packages/{P['Не сегодня']['id']}", {"spend_days_mask": 127}, "boss")
mine2 = {x.get("name"): x for x in api("GET", f"/admin/users/{U[GP]}/packages", None, "admin1")[1].get("packages") or []}
check("каталог «Не сегодня» — «всегда», а выданный пакет — по снимку: по-прежнему «не сейчас»",
      sp == 200 and mine2.get("z24s Не сегодня", {}).get("spend_now") is False, J([sp, mine2.get("z24s Не сегодня")], 300))

section("окно продажи, лимит, только новым")
s1, d1 = issue(GO, "Завтра", method="card")
s2, d2 = issue(GO, "Вчера", method="card")
check(f"вне окна продажи — 409: «продаётся с {dru(1)}», «закончилась {dru(-1)}»",
      s1 == 409 and d1.get("code") == "not_on_sale" and d1.get("message") == f"Пакет «z24s Завтра» продаётся с {dru(1)}"
      and s2 == 409 and d2.get("message") == f"Продажа пакета «z24s Вчера» закончилась {dru(-1)}", J([s1, d1, s2, d2]))
s3, d3 = issue(GN, "Один", method="card")
s4, d4 = issue(GN, "Один", method="card")
check("«Один» (1 на гостя): z24sn — первая 201, вторая — 409 «не больше 1 на гостя, у гостя уже 1»",
      s3 == 201 and s4 == 409 and d4.get("code") == "guest_limit" and d4.get("message") == "Пакет «z24s Один» — не больше 1 на гостя, у гостя уже 1",
      J([s3, s4, d4]))
gt = login(GN, GUEST_PASS)[1].get("access_token")  # ник «z<цифра>…» — не персонал: пароль гостя явно
s5, d5 = api("POST", "/me/packages", {"package_id": P["Один"]["id"]}, token=gt)
api("POST", f"/admin/user-packages/{(d3.get('package') or {}).get('id')}/void", {"refund_method": "none"}, "boss")
s6, d6 = issue(GN, "Один", method="card")
check("сам в приложении — тоже 409 «у тебя уже 1»; отменил выдачу — снова можно (201)",
      s5 == 409 and d5.get("message", "").endswith("у тебя уже 1") and s6 == 201, J([s5, d5, s6]))
s7, d7 = issue(GO, "Новым", method="card")
s8, d8 = issue(GN, "Новым", method="card")
check("«Новым»: старому z24so — 409 «только новым гостям», новому z24sn — 201",
      s7 == 409 and d7.get("code") == "new_only" and d7.get("message", "").startswith("Пакет «z24s Новым» — только новым гостям") and s8 == 201,
      J([s7, d7, s8]))
cat = lambda g: {x.get("name") for x in api("GET", "/me/packages/catalog", token=login(g, GUEST_PASS)[1].get("access_token"))[1].get("packages") or []}
co, cn = cat(GO), cat(GN)
check("каталог гостя: старому — нет «Новым», «Завтра», «Вчера», есть «Один»; новому — нет «Один» (купил), есть «Новым»",
      not {"z24s Новым", "z24s Завтра", "z24s Вчера"} & co and "z24s Один" in co and "z24s Всегда" in co
      and "z24s Один" not in cn and "z24s Новым" in cn, J([sorted(x for x in co if x.startswith("z24s")), sorted(x for x in cn if x.startswith("z24s"))]))

section("проверки полей и журнал")
bad = [("продажа наоборот", {"sell_from": day(5), "sell_to": day(1)}, "bad_sell_dates"),
       ("дата не ГГГГ-ММ-ДД", {"sell_from": "01.12.2026"}, "bad_sell_dates"),
       ("101 на гостя", {"per_guest": 101}, "bad_per_guest"),
       ("без дней списания", {"spend_days_mask": 0}, "bad_spend_days"),
       ("пустое окно списания", {"spend_start_min": 600, "spend_end_min": 600}, "bad_spend_time")]
res = [(n, *mkpack("Кривой", **kw)) for n, kw, _ in bad]
check("кривые поля — 400: " + ", ".join(n for n, _, _ in bad),
      all(s == 400 and d.get("code") == want for (_, s, d), (_, _, want) in zip(res, bad)), J([(n, s, d.get("code")) for n, s, d in res]))
api("PATCH", f"/admin/packages/{P['Всегда']['id']}", {"per_guest": 2, "new_only": True, "sell_to": day(30)}, "boss")
aud = [(x.get("kind"), x.get("text", "")) for x in audit("boss", limit=80) if x.get("id") not in A0 and (x.get("kind") or "").startswith("package_")]
want_upd = f"z24s Всегда: продажа: всегда → по {dru(30)}, на гостя: без ограничения → 2, только новым"
check("журнал: создание — с условиями («z24s Не сейчас · … · тратится …»), правка — «продажа: всегда → по …, на гостя: без ограничения → 2, только новым»",
      any(k == "package_create" and t.startswith("z24s Не сейчас · z24s Сезон · 60 мин · 5.00 zł") and t.endswith(f"тратится каждый день {hm(cs)}–{hm(ce) if ce != 1440 else '24:00'}") for k, t in aud)
      and ("package_update", want_upd) in aud, J(aud, 900))

done()
