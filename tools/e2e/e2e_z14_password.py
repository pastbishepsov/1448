#!/usr/bin/env python3
"""З14-и3 «Пароли персонала» (№23; З-Р17). Забыл пароль админ — раньше только SQL или «уволить → сбросить как гостю →
назначить снова»: сброс у стойки персоналу закрыт (cannot_touch_staff). Теперь сбрасывает владелец — и только он:
POST /admin/staff/:id/password/reset даёт временный пароль один раз, все входы сотрудника закрываются сразу (у персонала
adminMiddleware сверяет момент выдачи access с отсечкой — не ждём, пока он истечёт), пауза после неудачных входов
снимается. Вошёл временным — ручки админки отвечают must_change_password, пока он не задаст свой (POST /me/password);
смена своего пароля закрывает входы на других устройствах. Владельцу в журнал — «пароль сброшен» и «пароль сменён»,
самого пароля там нет; админ этого не видит. Наём ставит ту же обязательную смену — это юнит (newStaffUser): аккаунты
проверок живут между прогонами, свежего найма у них нет. Временный сотрудник z14p уходит проверкой; денег не трогает."""
import re

from _z import *

P = "z14p"
hire_temp(P)
NEWP, NEWP2 = "Z14p-own-1", "Z14p-own-2"
at_end(lambda: sql(f"UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE nickname='admin1'), "
                   f"must_change_password=false WHERE nickname='{P}'"))  # пароль — как у временных сотрудников
row = lambda: sql(f"SELECT must_change_password, tokens_valid_from IS NOT NULL FROM users WHERE nickname='{P}'")
reset = lambda who="boss", nick=P: api("POST", f"/admin/staff/{uid(nick)}/password/reset", {}, who)
work_me = lambda token: api("GET", "/admin/work/me", None, token=token)

def fresh_audit(seen, who="boss"):
    """Строки журнала про z14p, появившиеся после seen: у каждого вида — последняя."""
    out = {}
    for x in audit(who, limit=40):
        if x.get("nickname") == P and x.get("id") not in seen:
            out.setdefault(x.get("kind"), x)
    return out

section("сбросить пароль сотруднику может только владелец")
pair = tokens(P, fresh=True)  # вход в админку: живые access и refresh «до сброса»
check("данные: z14p входит в админку, ручки отвечают", work_me(pair["access"])[0] == 200)
s, d = reset("admin1")
check("админу — 403 owner_only", s == 403 and d.get("code") == "owner_only", f"{s} {J(d)}")
s, d = api("POST", f"/admin/users/{uid(P)}/password/reset", {}, "admin1")
check("сброс гостя у стойки по-прежнему не трогает персонал — 403 cannot_touch_staff", s == 403 and d.get("code") == "cannot_touch_staff", f"{s} {J(d)}")
s1, d1 = reset("boss", "boss")
s2, d2 = reset("boss", "Guest45")
s3, d3 = api("POST", "/admin/staff/00000000-0000-0000-0000-000000000000/password/reset", {}, "boss")
check("владельцу себе — 409 self («Сменить пароль»), гостю — 409 not_staff, нет такого — 404",
      s1 == 409 and d1.get("code") == "self" and "Сменить пароль" in d1.get("message", "") and s2 == 409 and d2.get("code") == "not_staff"
      and s3 == 404, f"{s1} {J(d1)} · {s2} {J(d2)} · {s3}")
seen = {x.get("id") for x in audit(limit=60)}
s, d = reset()
temp = d.get("temp_password", "")
check("владелец — 200: временный пароль xxxx-xxxx-xxxx без похожих символов, подсказка «при входе админка попросит задать свой»",
      s == 200 and re.fullmatch(r"[abcdefghjkmnpqrtuvwxyz2346789]{4}(-[abcdefghjkmnpqrtuvwxyz2346789]{4}){2}", temp) is not None
      and "попросит задать свой пароль" in d.get("hint", ""), f"{s} {J(d)}")
check("в базе: обязательная смена и отсечка токенов", row() == "t|t", row())
au = fresh_audit(seen)
pr = au.get("staff_password_reset") or {}
check("журнал владельца: «z14p · выдан временный пароль, все входы закрыты» от boss — самого пароля в журнале нет",
      pr.get("text") == f"{P} · выдан временный пароль, все входы закрыты" and pr.get("admin") == "boss" and temp not in J(au, 5000), J(pr))
check("админу эта строка не видна", "staff_password_reset" not in fresh_audit(set(), "admin1"))

section("все входы сотрудника закрыты сразу")
s, d = work_me(pair["access"])
check("access, выданный до сброса, — 401 token_revoked «Пароль менялся — войди заново» (не ждём, пока истечёт)",
      s == 401 and d.get("code") == "token_revoked" and d.get("message") == "Пароль менялся — войди заново", f"{s} {J(d)}")
s, d = api("POST", "/auth/refresh", {"refresh_token": pair["refresh"]})
check("refresh — тоже 401 token_revoked", s == 401 and d.get("code") == "token_revoked", f"{s} {J(d)}")
check("старый пароль больше не подходит — 401", login(P)[0] == 401)

section("вход временным паролем — сначала свой")
s, lg = login(P, temp, client="admin")
tt = lg.get("access_token", "")
check("вход временным — 200, must_change_password в ответе", s == 200 and (lg.get("user") or {}).get("must_change_password") is True, f"{s} {J(lg, 200)}")
s1, d1 = work_me(tt)
s2, d2 = api("GET", "/admin/staff-chat", None, token=tt)
s3, d3 = api("GET", "/me", None, token=tt)
check("ручки админки — 403 must_change_password «Сначала задай свой пароль вместо временного»; /me — 200",
      s1 == 403 and d1.get("code") == "must_change_password" and d1.get("message") == "Сначала задай свой пароль вместо временного"
      and s2 == 403 and d2.get("code") == "must_change_password" and s3 == 200 and d3.get("must_change_password") is True, f"{s1} {J(d1)} · {s2} · {s3}")
chg = lambda cur, new, token: api("POST", "/me/password", {"current_password": cur, "new_password": new}, token=token)
c1, c2, c3 = chg("не тот", NEWP, tt), chg(temp, "12345", tt), chg(temp, temp, tt)
check("смена: чужой текущий — 401 bad_password, короткий новый — 400 password_short, тот же — 400 password_same",
      (c1[0], c1[1].get("code"), c2[0], c2[1].get("code"), c3[0], c3[1].get("code")) == (401, "bad_password", 400, "password_short", 400, "password_same"),
      J([c1, c2, c3], 400))
seen = {x.get("id") for x in audit(limit=60)}
s, ch = chg(temp, NEWP, tt)
nt = ch.get("access_token", "")
check("свой пароль — 200: свежая пара, флага больше нет, ручки админки отвечают", s == 200 and (ch.get("user") or {}).get("must_change_password") is False
      and work_me(nt)[0] == 200 and row().startswith("f|"), f"{s} {J(ch, 200)} · {row()}")
s, d = work_me(tt)
check("токен входа временным — 401 token_revoked (смена своей отсечкой закрыла его)", s == 401 and d.get("code") == "token_revoked", f"{s} {J(d)}")
check("вход своим — 200 без обязательной смены; временным — 401", login(P, NEWP)[0] == 200
      and (login(P, NEWP)[1].get("user") or {}).get("must_change_password") is False and login(P, temp)[0] == 401)
pc = fresh_audit(seen).get("staff_password_change") or {}
check("журнал владельца: «z14p · сменил пароль (временный — на свой)» от самого z14p; админу не видна",
      pc.get("text") == f"{P} · сменил пароль (временный — на свой)" and pc.get("admin") == P
      and "staff_password_change" not in fresh_audit(set(), "admin1"), J(pc))

section("свой пароль — в админке: другие устройства выходят")
s, other = login(P, NEWP, client="admin")  # «второе устройство»
ot = other.get("access_token", "")
s, ch = chg(NEWP, NEWP2, nt)
s2, d2 = work_me(ot)
check("смена с одного устройства — 200 и свежая пара; на другом — 401 token_revoked", s == 200 and work_me(ch.get("access_token", ""))[0] == 200
      and s2 == 401 and d2.get("code") == "token_revoked", f"{s} · {s2} {J(d2)}")
pc = fresh_audit(seen).get("staff_password_change") or {}
check("журнал: «z14p · сменил пароль» — без «временный»", pc.get("text") == f"{P} · сменил пароль", J(pc))

section("сброс снимает паузу после неудачных входов")
codes = [login(P, "неверный-" + str(i))[0] for i in range(10)]
s, d = login(P, NEWP2)
check("10 неверных — верный пароль ждёт паузу: 429 login_paced (персоналу — пауза, не замок, З0-и3)",
      codes == [401] * 10 and s == 429 and d.get("code") == "login_paced", f"{codes[-1]} · {s} {J(d)}")
s, d = reset()
s2, lg = login(P, d.get("temp_password", ""))
check("владелец сбросил — временным входит сразу, без паузы", s == 200 and s2 == 200, f"{s} · {s2} {J(lg, 200)}")
done()
