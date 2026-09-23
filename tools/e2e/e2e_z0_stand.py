#!/usr/bin/env python3
"""З0-и0 «Стенд трека З»: стенд готов к проверкам трека — гости Guest31…Guest50 из сида и помощники
_z.py (вход с кэшем, клубные сутки, настройки с возвратом, деньги, посадка, временный сотрудник).
Гость Guest31, временный сотрудник z0std. Идемпотентен: сессию закрывает, настройку возвращает,
сотрудника увольняет."""
from _z import *

section("сид: гости трека З")
z = rows("SELECT nickname || ' ' || role || ' ' || phone FROM users WHERE nickname ~ '^Guest(3[1-9]|4[0-9]|50)$' ORDER BY nickname")
check("Guest31…Guest50 заведены гостями, телефоны +48500120031…050", len(z) == 20 and all(" player " in r for r in z)
      and z[0].endswith("+48500120031") and z[-1].endswith("+48500120050"), f"{len(z)} {z[:1]} {z[-1:]}")
bad = rows("SELECT u.nickname FROM users u WHERE u.nickname ~ '^Guest(3[1-9]|4[0-9]|50)$' AND u.wallet_grosz <> "
           "COALESCE((SELECT sum(amount_grosz) FROM wallet_transactions w WHERE w.user_id = u.id), 0)")
check("кошельки гостей трека З сходятся со своим журналом", not bad, J(bad))

section("вход с кэшем и клубные сутки")
t = tok("admin1")
check("второй tok() — тот же токен, без нового входа", tok("admin1") == t)
s, d = api("GET", "/admin/audit?limit=1", who="admin1")
check("club_day() = клубные сутки сервера (журнал админа)", s == 200 and d.get("day") == club_day(), f"{d.get('day')} · {club_day()}")

section("настройки с возвратом")
before = rows("SELECT value FROM settings WHERE key='admin_day_deposit_cap_pln'")
s, _ = set_settings(admin_day_deposit_cap_pln=321)
check("set_settings → 200, значение видно в /admin/settings", s == 200 and settings().get("admin_day_deposit_cap_pln") == 321, s)
restore_settings()
check("restore_settings → строка настройки как до проверки", rows("SELECT value FROM settings WHERE key='admin_day_deposit_cap_pln'") == before, J(before))

section("деньги и посадка")
at_end(lambda: end_all("Guest31"))
ensure_wallet("Guest31", 20)
check("ensure_wallet: на кошельке Guest31 не меньше 20 zł", wallet("Guest31") >= 2000, wallet("Guest31"))
sid = seat("Guest31", "PC-01", minutes_ago=30)
s, d = end(sid)
row = sql(f"SELECT status || ' ' || round(extract(epoch FROM ended_at - started_at) / 60) FROM sessions WHERE id='{sid}'")
check("seat(minutes_ago=30) → end: сессия закрыта, длилась 30 мин", s == 200 and row == "completed 30", f"{s} {row}")

section("временный сотрудник")
hire_temp("z0std")
s, d = login("z0std")
check("hire_temp: z0std входит с ролью admin", s == 200 and d["user"]["role"] == "admin", f"{s} {d.get('user', {}).get('role') or d.get('code')}")
check("… и видит админку", api("GET", "/admin/work/me", who="z0std")[0] == 200)
s, d = dismiss("z0std")
check("dismiss → 200, в базе роль player", s == 200 and sql("SELECT role FROM users WHERE nickname='z0std'") == "player", f"{s} {J(d)}")
s, d = login("z0std")
check("новый вход уволенного — уже гость", s == 200 and d["user"]["role"] == "player", f"{s} {d.get('user', {}).get('role')}")
done()
