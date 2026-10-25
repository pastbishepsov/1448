#!/usr/bin/env python3
"""З15-и5 «Карточка сотрудника: периоды, бан, телефон» (№30, З-Р20). Четыре дыры карточки. Повторный наём («Назначить»
бывшего сотрудника) стирал увольнение: прежний период работы и его причина оставались только в журнале. PUT карточки
принимал dismissed_at и отправлял работающего админа в «Уволенные», не снимая роли. Забаненного гостя можно было
назначить админом. Телефон в карточке был свободным текстом. Теперь: периоды работы — своей таблицей (миграция 089):
наём и назначение открывают период, увольнение (сразу или отложенное) закрывает его датой и причиной, правка «Работает с»
двигает начало последнего, «Стереть личные данные» стирает и причины; карточка и «Уволенные» отдают все периоды. Дату
увольнения PUT не принимает (400 dismissed_readonly), забаненного назначить нельзя (409 banned), телефон — как у анкеты
гостя (+ и 6–15 цифр, без пробелов), прежний нетронутый — как есть. Миграция 089 заполняет периоды из карточек, причину
— из строки журнала (staff_dismiss_reason); проверка прогоняет её ещё раз (идемпотентна). Временный z15k — сотрудник:
его периоды удаляются в начале, бан снимается в начале и в конце; наём с плохим телефоном пробуется на нике гостя
Guest46 — телефон проверяется раньше ника, аккаунт не заводится; денег не трогает."""
import os
import subprocess

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "migrations", "089_staff_periods.sql")
Z, X = "z15k", "Guest46"
R1, R2 = "z15 переезд", "z15 учёба, вернётся"
zid = hire_temp(Z)
D = {n: club_day(n) for n in (-20, -10, -6, -5, -3, -1, 0, 2)}
unban = lambda: sql(f"UPDATE users SET status = 'active' WHERE id = '{zid}'")
unban()  # хвост упавшего прогона
at_end(unban)
sql(f"DELETE FROM staff_periods WHERE user_id = '{zid}'")
sql(f"UPDATE staff_profiles SET phone = '', position = '', leave_on = NULL, leave_by = NULL, leave_reason = '' WHERE user_id = '{zid}'")
put = lambda body: api("PUT", f"/admin/staff/{zid}/profile", body, "boss")
prof = lambda: api("GET", f"/admin/staff/{zid}/profile", None, "boss")[1]
per = lambda: [(x.get("hired_at"), x.get("dismissed_at"), x.get("reason")) for x in (prof().get("periods") or [])]
card = lambda: sql(f"SELECT COALESCE(hired_at::text, '') || '|' || COALESCE(dismissed_at::text, '') || '|' || phone || '|' || position "
                   f"FROM staff_profiles WHERE user_id = '{zid}'")
role = lambda: sql(f"SELECT role FROM users WHERE id = '{zid}'")
listed = lambda: any(x.get("nickname") == Z for x in api("GET", "/admin/staff", None, "boss")[1].get("staff", []))


def migrate():
    """Прогнать 089 ещё раз: (код, последняя строка вывода — итог заполнения)."""
    r = subprocess.run(["psql", "-v", "ON_ERROR_STOP=1", DB, "-f", MIG], capture_output=True, text=True)
    out = r.stdout.strip().splitlines()
    return r.returncode, (out[-1] if out else r.stderr.strip()[-200:])


section("дату увольнения карточка не правит")
s, d = put({"hired_at": D[-20], "dismissed_at": D[0]})
check("PUT с dismissed_at у работающего — 400 dismissed_readonly; не уволен, роль admin, в списке персонала",
      s == 400 and d.get("code") == "dismissed_readonly" and card().split("|")[1] == "" and role() == "admin" and listed(),
      f"{s} {J(d)} {card()}")
s, d = put({"hired_at": D[-20]})
check("PUT без даты увольнения — 200; период работы открыт с «Работает с» карточки",
      s == 200 and per() == [(D[-20], "", "")], f"{s} {J(per())}")

section("телефон — как у анкеты гостя")
s, d = put({"hired_at": D[-20], "phone": "+48 500 000 009"})
check("«+48 500 000 009» — сохраняется без пробелов: +48500000009", s == 200 and (d.get("profile") or {}).get("phone") == "+48500000009"
      and card().split("|")[2] == "+48500000009", f"{s} {J(d, 200)}")
s, d = put({"hired_at": D[-20], "phone": "600 000 009"})
check("без «+» — 400 bad_phone «Телефон — в международном формате: + и 6–15 цифр», телефон прежний",
      s == 400 and d.get("code") == "bad_phone" and d.get("message") == "Телефон — в международном формате: + и 6–15 цифр"
      and card().split("|")[2] == "+48500000009", f"{s} {J(d)}")
sql(f"UPDATE staff_profiles SET phone = '600 000 000' WHERE user_id = '{zid}'")  # записан до проверки свободным текстом
s, d = put({"hired_at": D[-20], "phone": "600 000 000", "position": "кассир"})
check("прежний телефон свободным текстом, не тронутый правкой, — как есть; правка должности сохраняется",
      s == 200 and card().split("|")[2:] == ["600 000 000", "кассир"], f"{s} {J(d, 200)}")
s, d = api("POST", "/admin/staff/hire", {"nickname": X, "password": STAFF_PASS, "phone": "12345"}, "boss")
check("наём с телефоном «12345» — 400 bad_phone раньше проверки ника (на нике гостя Guest46 — не already_exists), гость как был",
      s == 400 and d.get("code") == "bad_phone" and sql(f"SELECT role FROM users WHERE nickname = '{X}'") == "player", f"{s} {J(d)}")

section("периоды: увольнение, возвращение, отложенное увольнение")
s, d = api("POST", f"/admin/staff/{zid}/dismiss", {"date": D[-10], "reason": R1}, "boss")
check(f"уволен {D[-10]} с причиной — период закрыт: {D[-20]} — {D[-10]} · «{R1}»",
      s == 200 and per() == [(D[-20], D[-10], R1)] and role() == "player", f"{s} {J(per())}")
s, d = api("POST", "/admin/staff", {"nickname": Z}, "boss")
check("«Назначить» бывшего сотрудника — 200 returned; прежний период остался, новый — с сегодня; в карточке «Работает с» "
      "сегодня, не уволен; в «Уволенных» его нет",
      s == 200 and d.get("returned") is True and per() == [(D[-20], D[-10], R1), (D[0], "", "")]
      and card().split("|")[:2] == [D[0], ""] and not any(x.get("nickname") == Z for x in
                                                         api("GET", "/admin/staff/archive", None, "boss")[1].get("staff", [])),
      f"{s} {J(d)} {J(per())} {card()}")
s, _ = put({"hired_at": D[-5], "phone": "600 000 000", "position": "кассир"})
check(f"«Работает с» {D[-5]} — сдвинулось начало нового периода, прежний не тронут",
      s == 200 and per() == [(D[-20], D[-10], R1), (D[-5], "", "")], J(per()))
s, d = api("POST", f"/admin/staff/{zid}/dismiss", {"date": D[2], "reason": R2}, "boss")
p_mid = per()
sql(f"UPDATE staff_profiles SET leave_on = '{D[-1]}' WHERE user_id = '{zid}'")  # последний день прошёл
s2, d2 = api("POST", "/admin/staff/leave/run", {}, "boss")
check(f"отложенное увольнение: пока не прошло — период открыт; прошло — закрыт последним днём {D[-1]} и его причиной",
      s == 200 and d.get("deferred") is True and p_mid == [(D[-20], D[-10], R1), (D[-5], "", "")] and s2 == 200
      and per() == [(D[-20], D[-10], R1), (D[-5], D[-1], R2)] and role() == "player", f"{s} {J(p_mid)} {s2} {J(per())}")
arch = next((x for x in api("GET", "/admin/staff/archive", None, "boss")[1].get("staff", []) if x.get("nickname") == Z), {})
check("«Уволенные»: z15k — оба периода с причинами, «работал» по последнему",
      [(x.get("hired_at"), x.get("dismissed_at"), x.get("reason")) for x in arch.get("periods", [])] == [(D[-20], D[-10], R1), (D[-5], D[-1], R2)]
      and arch.get("hired_at") == D[-5] and arch.get("dismissed_at") == D[-1], J(arch, 400))
s, d = put({"hired_at": D[-6], "dismissed_at": D[-3], "phone": "600 000 000", "position": "кассир"})
s2, d2 = put({"hired_at": D[-6], "dismissed_at": D[-1], "phone": "600 000 000", "position": "кассир"})
check("карточка уволенного: другая дата увольнения — 400 dismissed_readonly; та же — 200, «Работает с» двигает начало "
      "последнего периода",
      s == 400 and d.get("code") == "dismissed_readonly" and s2 == 200 and per() == [(D[-20], D[-10], R1), (D[-6], D[-1], R2)],
      f"{s} {s2} {J(per())}")

section("миграция 089: периоды из карточек, причина — из журнала")
have = os.path.exists(MIG)
check("миграция 089 есть", have, MIG)
rc, out = migrate() if have else (-1, "")
fn = lambda s_, day: sql(f"SELECT COALESCE(staff_dismiss_reason('{s_}', '{day}'), '<нет>')") if rc == 0 else "?"
cases = {
    "a · уволен 2026-10-23 · z14 опоздания, грубость, снят с 1 будущих смен": "z14 опоздания, грубость",
    "a · уволен 2026-10-23 (последний рабочий день прошёл) · сам, смена с 22.10 20:00 закрыта (11 ч 00 м — на проверку), "
    "касса не сдана, снят с 3 будущих смен": "сам",
    "a · уволен 2026-10-23 · переезд, смена с 15:20 закрыта (0 ч 00 м — на проверку), касса без пересчёта": "переезд",
    "a · уволен 2026-10-23 · —, снят с 1 будущих смен": "<нет>",
    "a · уволен 2026-10-23, смена с 15:21 закрыта (0 ч 00 м — на проверку), касса не сдана": "<нет>",
    "a · уволен 2026-10-22 · другой день": "<нет>",
}
got = {k: fn(k, "2026-10-23") for k in cases}
check("причина из строки журнала: с хвостами про смену и график, после «последний рабочий день прошёл», с запятыми; стёртая "
      "«—», без причины и другой день — нет", got == cases, J(got, 600))
sql(f"DELETE FROM staff_periods WHERE user_id = '{zid}'")  # как до 089: периодов нет, карточка есть
rc2, out2 = migrate() if have else (-1, "")
check(f"заполнение: z15k без периодов — один период из карточки ({D[-6]} — {D[-1]}), причина «{R2}» — из строки журнала "
      "о завершении отложенного увольнения", rc2 == 0 and out2 == "INSERT 0 1" and per() == [(D[-6], D[-1], R2)],
      f"{rc2} {out2} {J(per())}")
rc3, out3 = migrate() if have else (-1, "")
check("повторный прогон 089 ничего не добавляет — INSERT 0 0", rc3 == 0 and out3 == "INSERT 0 0" and len(per()) == 1,
      f"{rc3} {out3}")

section("забаненного назначить нельзя")
sb, _ = api("POST", f"/admin/users/{zid}/ban", {}, "boss")
s, d = api("POST", "/admin/staff", {"nickname": Z}, "boss")
check("забанен — «Назначить» 409 banned «… забанен — сначала сними бан …»; роль и периоды прежние",
      sb == 200 and s == 409 and d.get("code") == "banned" and "сначала сними бан" in d.get("message", "") and role() == "player"
      and len(per()) == 1, f"{sb} {s} {J(d)}")
su, _ = api("POST", f"/admin/users/{zid}/unban", {}, "boss")
s, d = api("POST", "/admin/staff", {"nickname": Z}, "boss")
check("бан снят — «Назначить» 200, вернулся: новый период с сегодня", su == 200 and s == 200 and d.get("returned") is True
      and per() == [(D[-6], D[-1], R2), (D[0], "", "")], f"{su} {s} {J(per())}")
s, _ = api("POST", f"/admin/staff/{zid}/dismiss", {}, "boss")
check("уволен сегодня без причины — период закрыт сегодня, причины нет", s == 200 and (per() or [None])[-1] == (D[0], D[0], ""),
      J(per()))

section("стереть личные данные — и причины в периодах")
s, d = api("POST", f"/admin/staff/{zid}/erase", {}, "boss")
check("«Стереть личные данные» — причин увольнения в периодах больше нет (period_reasons ≥ 1), даты на месте",
      s == 200 and d.get("period_reasons", 0) >= 1 and all(r == "" for _, _, r in per()) and len(per()) == 2, f"{s} {J(d)} {J(per())}")
done()
