#!/usr/bin/env python3
"""З0-и3 «Вход персонала без глухого замка» (№28): после 10 неверных паролей персонал не запирается на
15 минут, а пробует не чаще раза в 30 с — верный пароль проходит после паузы; серия неудач — одна строка
login_failed (ник, IP, сколько) в журнале владельца, админу не видна; гость — прежний замок, его снимает
сброс пароля у стойки. Временный сотрудник z0adm, гость Guest32; идёт ~70 с (две паузы по 30 с).
Идемпотентен: z0adm уволен, у Guest32 снова пароль Guest12345."""
import time
from _z import *

WRONG = "Wrong-pass-0"
def paced():
    """Верный пароль: (статус, ответ); на 429 login_paced — сколько ждать, по ответу сервера."""
    s, d = login("z0adm")
    return s, d, d.get("retry_after", 0)
fails = lambda who: [r for r in audit(who, kind="login_failed", limit=200) if r.get("nickname") == "z0adm"]

section("персонал: 10 неверных — темп вместо замка")
zid = hire_temp("z0adm")
if (s := login("z0adm"))[0] == 429:  # хвост упавшего прогона: дождаться паузы и сбросить счётчик входом
    time.sleep(s[1].get("retry_after", 30) + 0.5); login("z0adm")
rows0 = len(fails("boss"))
codes = [login("z0adm", WRONG)[0] for _ in range(10)]
check("10 неверных паролей → 10 × 401", codes == [401] * 10, codes)
s, d, wait = paced()
check("верный пароль сразу → 429 login_paced «через N с», N ≤ 30", s == 429 and d.get("code") == "login_paced" and 0 < wait <= 30, f"{s} {d.get('code')} {d.get('message')}")
check("неверный сразу — тоже 429: пароль в паузе не проверяется", login("z0adm", WRONG)[0] == 429)
time.sleep(wait + 0.5)
check("после паузы неверный проверяется → 401", login("z0adm", WRONG)[0] == 401)
s, d, wait = paced()
check("… и следующая попытка снова через паузу → 429", s == 429 and 0 < wait <= 30, f"{s} {wait}")
time.sleep(wait + 0.5)
s, d = login("z0adm")
check("после паузы верный пароль → 200, персонал входит", s == 200 and d["user"]["role"] == "admin", f"{s} {d.get('code')}")
check("успешный вход обнуляет серию: неверный → 401, верный следом → 200", login("z0adm", WRONG)[0] == 401 and login("z0adm")[0] == 200)

section("журнал: одна строка на серию, только владельцу")
rows = fails("boss")
new = rows[:len(rows) - rows0]
check("у владельца одна новая строка login_failed про z0adm", len(new) == 1, J(new))
check("… в ней сколько неудач и IP, автора нет (кто подбирал — неизвестно)", new and "10 неверных" in new[0].get("text", "")
      and "IP" in new[0].get("text", "") and "admin" not in new[0], new and new[0].get("text"))
check("админу строка login_failed не видна", fails("admin1") == [], J(fails("admin1")))
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
own = int(sql(f"SELECT count(*) FROM admin_actions WHERE admin_id='{zid}' AND action <> 'login_failed' "
              f"AND created_at >= {at_club(club_day(), f'{rh:02d}:00')}"))
acts = {r["nickname"]: r.get("actions_today") for r in api("GET", "/admin/staff", None, "boss")[1]["staff"]}
check("«Персонал» → «Действий сегодня» у z0adm — без строки login_failed", acts.get("z0adm") == own, f"{acts.get('z0adm')} · свои {own}")

section("гость: прежний замок")
g32 = uid("Guest32")
def unlock_guest():
    """Сброс пароля у стойки снимает замок (К0-и5); пароль гостя — обратно Guest12345."""
    api("POST", f"/admin/users/{g32}/password/reset", {}, "admin1")
    sql(f"UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE nickname='Guest31'), "
        f"must_change_password=false WHERE id='{g32}'")
at_end(unlock_guest)
codes = [login("Guest32", WRONG)[0] for _ in range(10)]
s, d = login("Guest32")
check("гость: 10 неверных → верный даёт 429 login_locked (минуты)", codes == [401] * 10 and s == 429 and d.get("code") == "login_locked", f"{s} {d.get('code')}")
unlock_guest()
check("сброс пароля у стойки снимает замок: Guest32 входит", login("Guest32")[0] == 200)
done()
