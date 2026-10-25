#!/usr/bin/env python3
"""З15-и5а «Ранние периоды работы — из журнала» (№30, З-Р20). 089 завела каждой карточке один период из её дат, а тем,
кого до неё уже возвращали («Назначить» бывшего сотрудника — «ник → admin (вернулся)»), прежний код стирал увольнение
из карточки: их период склеил всё подряд — с первого найма по сегодня, без прежних увольнений и причин. Миграция 090
делит такой период по журналу: увольнение закрывает период датой и причиной, назначение после увольнения открывает
новый — с клубных суток строки назначения; начало периода, где «Работает с» ставили последним, — из карточки, где
ставили раньше — что тогда поставили, где не ставили — день возвращения; два увольнения подряд — между ними период с
неизвестным началом; начало позже конца — начало = конец; последний период — тот, что завела 089, и «Работает с»
карточки — его начало, если он у человека последний. Проверка готовит три истории и прогоняет 090 ещё раз (она
идемпотентна): z15u — настоящими вызовами (правка «Работает с», увольнение с причиной, «Назначить», снова увольнение,
«Назначить»), после которых карточка и период сведены SQL-ом к виду «как 089 у прежнего кода»; z15v и z15w —
подставными строками журнала (SQL): у z15v — наём, отложенное увольнение, правки «Работает с», ночное возвращение,
снятие с прав и назначение без увольнения, стёртая причина, увольнение задним числом, два увольнения подряд; у z15w —
заведение карточки без даты, увольнение раньше дня возвращения и следующий период, заведённый уже новым кодом. Временные
z15u, z15v, z15w — сотрудники: их периоды и строки журнала о них удаляются в начале; денег не трогает."""
import datetime
import os
import re
import subprocess

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "migrations",
                   "090_staff_periods_history.sql")
U, V, W = "z15u", "z15v", "z15w"
R1, R2, R3 = "z15 переезд", "z15 учёба", "z15 снова"
BASE_DAY = datetime.date.fromisoformat(club_day(0))
D = lambda n: (BASE_DAY + datetime.timedelta(days=n)).isoformat()
RH = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
boss = uid("boss")
ids = {n: hire_temp(n) for n in (U, V, W)}
zu, zv, zw = ids[U], ids[V], ids[W]
for z in ids.values():  # чистый лист: ни периодов, ни строк журнала о нём, карточка без личного и без даты ухода
    sql(f"DELETE FROM staff_periods WHERE user_id = '{z}'")
    sql(f"DELETE FROM admin_actions WHERE target_user_id = '{z}'")
    sql(f"UPDATE staff_profiles SET full_name = '', phone = '', position = '', note = '', rate_type = 'none', rate_amount = 0, "
        f"leave_on = NULL, leave_by = NULL, leave_reason = '' WHERE user_id = '{z}'")
prof = lambda z: api("GET", f"/admin/staff/{z}/profile", None, "boss")[1]
per = lambda z: [(x.get("hired_at"), x.get("dismissed_at"), x.get("reason")) for x in (prof(z).get("periods") or [])]
card = lambda z: sql(f"SELECT COALESCE(hired_at::text, '') || '|' || COALESCE(dismissed_at::text, '') FROM staff_profiles "
                     f"WHERE user_id = '{z}'")
journal = lambda z: rows(f"SELECT action || ' ' || details FROM admin_actions WHERE target_user_id = '{z}' ORDER BY created_at")


def row(z, action, details, at):
    """Подставная строка журнала о z от boss; at — SQL-выражение времени."""
    sql(f"INSERT INTO admin_actions (admin_id, action, target_user_id, details, created_at) "
        f"VALUES ('{boss}', '{action}', '{z}', '{details}', {at})")


def period(z, hired, dismissed=None, reason="", at="now()"):
    """Период, как его завела бы 089 (или следующий — новым кодом), с created_at = at."""
    q = lambda v: f"'{v}'" if v else "NULL"
    sql(f"INSERT INTO staff_periods (user_id, hired_on, dismissed_on, reason, created_at) "
        f"VALUES ('{z}', {q(hired)}, {q(dismissed)}, '{reason}', {at})")


def migrate():
    """Прогнать 090: (код, сколько человек, сколько периодов добавлено — из её NOTICE)."""
    if not os.path.exists(MIG):
        return -1, None, None
    r = subprocess.run(["psql", "-v", "ON_ERROR_STOP=1", DB, "-f", MIG], capture_output=True, text=True)
    m = re.search(r"восстановлены у (\d+) чел\., добавлено периодов: (\d+)", r.stderr)
    return r.returncode, (int(m.group(1)) if m else None), (int(m.group(2)) if m else None)


ALL = ("SELECT p.user_id || '|' || COALESCE(p.hired_on::text, '') || '|' || COALESCE(p.dismissed_on::text, '') || '|' || "
       "p.reason || '|' || p.created_at FROM staff_periods p {} ORDER BY 1")
NO_RETURNS = ("WHERE NOT EXISTS (SELECT 1 FROM admin_actions a WHERE a.target_user_id = p.user_id "
              "AND a.action = 'staff_promote' AND a.details LIKE '% → admin (вернулся)')")
CARDS = "SELECT user_id || '|' || COALESCE(hired_at::text, '') || '|' || COALESCE(dismissed_at::text, '') FROM staff_profiles ORDER BY 1"

section("z15u: настоящая история — и вид, в каком её оставила 089 у прежнего кода")
s1, _ = api("PUT", f"/admin/staff/{zu}/profile", {"hired_at": D(-20)}, "boss")
s2, _ = api("POST", f"/admin/staff/{zu}/dismiss", {"date": D(-10), "reason": R1}, "boss")
s3, d3 = api("POST", "/admin/staff", {"nickname": U}, "boss")
s4, _ = api("POST", f"/admin/staff/{zu}/dismiss", {"reason": R2}, "boss")
s5, d5 = api("POST", "/admin/staff", {"nickname": U}, "boss")
want = [f"staff_card {U}: дата найма: {D(-20)}", f"staff_dismiss {U} · уволен {D(-10)} · {R1}",
        f"staff_promote {U} → admin (вернулся)", f"staff_dismiss {U} · уволен {D(0)} · {R2}",
        f"staff_promote {U} → admin (вернулся)"]
check("правка «Работает с», увольнение с причиной, «Назначить», снова увольнение, «Назначить» — 200; в журнале пять строк, "
      "как их писал и прежний код",
      (s1, s2, s3, s4, s5) == (200,) * 5 and d3.get("returned") is True and d5.get("returned") is True and journal(zu) == want,
      f"{(s1, s2, s3, s4, s5)} {J(journal(zu), 600)}")
# прежний код при возвращении только снимал дату увольнения: «Работает с» оставался от первого найма, а 089 завела по
# карточке один период — с него по сегодня
sql(f"DELETE FROM staff_periods WHERE user_id = '{zu}'")
sql(f"UPDATE staff_profiles SET hired_at = '{D(-20)}', dismissed_at = NULL WHERE user_id = '{zu}'")
period(zu, D(-20))
check(f"до 090: у z15u один период {D(-20)} — сейчас, в карточке «Работает с» {D(-20)}",
      per(zu) == [(D(-20), "", "")] and card(zu) == f"{D(-20)}|", f"{J(per(zu))} {card(zu)}")

section("z15v и z15w: подставной журнал")
at = at_club
row(zv, "staff_hire", f"{V} · кассир · с {D(-40)}", at(D(-40), "10:00"))
row(zv, "staff_dismiss", f"{V} · уволен {D(-30)} (последний рабочий день прошёл) · {R1}, снят с 2 будущих смен",
    at(D(-29), "08:10"))
row(zv, "staff_promote", f"{V} → admin (вернулся)", at(D(-25), "11:00"))
row(zv, "staff_card", f"{V}: телефон, дата найма: {D(-24)}", at(D(-25), "11:05"))
row(zv, "staff_dismiss", f"{V} · уволен {D(-20)} · —, смена с 14:00 закрыта (1 ч 00 м — на проверку), касса не сдана",
    at(D(-20), "15:00"))
row(zv, "staff_promote", f"{V} → admin (вернулся)", at(D(-14), "02:30"))  # ночь: клубные сутки — предыдущие
row(zv, "staff_demote", f"{V} → player", at(D(-12), "09:00"))
row(zv, "staff_promote", f"{V} → admin", at(D(-11), "09:00"))  # сняли с прав и назначили снова — тот же период
row(zv, "staff_dismiss", f"{V} · уволен {D(-9)} · {R2}", at(D(-8), "18:00"))  # задним числом
row(zv, "staff_dismiss", f"{V} · уволен {D(-7)} · {R3}", at(D(-7), "12:00"))  # снова, без назначения между
row(zv, "staff_promote", f"{V} → admin (вернулся)", at(D(-5), "10:00"))
row(zv, "staff_card", f"{V}: дата найма: {D(-6)}, заметка", at(D(-4), "10:00"))
sql(f"UPDATE staff_profiles SET hired_at = '{D(-6)}', dismissed_at = NULL WHERE user_id = '{zv}'")
period(zv, D(-6))
row(zw, "staff_card", f"{W}: карточка заведена", at(D(-30), "10:00"))
row(zw, "staff_dismiss", f"{W} · уволен {D(-20)} · z15 причина 1", at(D(-20), "10:00"))
row(zw, "staff_promote", f"{W} → admin (вернулся)", at(D(-10), "10:00"))
row(zw, "staff_dismiss", f"{W} · уволен {D(-12)} · z15 причина 2", at(D(-9), "10:00"))  # раньше дня возвращения
period(zw, D(-30), D(-12), "z15 причина 2", "now() - interval '2 minutes'")
row(zw, "staff_promote", f"{W} → admin (вернулся)", "now() - interval '90 seconds'")  # уже новым кодом — свой период
period(zw, D(0), at="now() - interval '1 minute'")
row(zw, "staff_card", f"{W}: дата найма: {D(0)}", "now() - interval '30 seconds'")  # правка следующего периода
sql(f"UPDATE staff_profiles SET hired_at = '{D(0)}', dismissed_at = NULL WHERE user_id = '{zw}'")
check(f"до 090: у z15v один период {D(-6)} — сейчас; у z15w — {D(-30)} — {D(-12)} и {D(0)} — сейчас",
      per(zv) == [(D(-6), "", "")] and per(zw) == [(D(-30), D(-12), "z15 причина 2"), (D(0), "", "")],
      f"{J(per(zv))} {J(per(zw))}")

section("миграция 090: периоды — по журналу")
have = os.path.exists(MIG)
check("миграция 090 есть", have, MIG)
quiet = rows(ALL.format(NO_RETURNS))
rc, people, added = migrate()
check("прогон 090 без ошибок: восстановлено не меньше чем у троих, добавлено не меньше семи периодов",
      rc == 0 and (people or 0) >= 3 and (added or 0) >= 7, f"{rc} {people} {added}")
check(f"z15u: {D(-20)} — {D(-10)} · {R1}, {D(0)} — {D(0)} · {R2}, {D(0)} — сейчас",
      per(zu) == [(D(-20), D(-10), R1), (D(0), D(0), R2), (D(0), "", "")], J(per(zu)))
lst = next((x for x in api("GET", "/admin/staff", None, "boss")[1].get("staff", []) if x.get("nickname") == U), {})
check(f"z15u: «Работает с» в карточке и в списке персонала — {D(0)}, день последнего возвращения",
      rc == 0 and card(zu) == f"{D(0)}|" and lst.get("hired_at") == D(0), f"{card(zu)} {J(lst, 200)}")
night = D(-15) if RH > 2 else D(-14)
check(f"z15v: наём «с {D(-40)}» — по отложенное увольнение {D(-30)} · {R1}; правка «Работает с» после возвращения — "
      f"{D(-24)}, стёртая причина — пусто; ночное возвращение — с {night}, снятие с прав и назначение без увольнения "
      f"период не делят, увольнение задним числом — {D(-9)} · {R2}; второе увольнение подряд — период без начала по "
      f"{D(-7)}; последний — с {D(-6)}, как в карточке",
      per(zv) == [(D(-40), D(-30), R1), (D(-24), D(-20), ""), (night, D(-9), R2), ("", D(-7), R3), (D(-6), "", "")],
      J(per(zv), 600))
check(f"z15v: карточка — как была ({D(-6)}): «Работает с» последний раз ставили в последнем периоде",
      rc == 0 and card(zv) == f"{D(-6)}|", card(zv))
check(f"z15w: заведение карточки — {D(-30)} — {D(-20)} · z15 причина 1; уволен задним числом раньше дня возвращения — "
      f"{D(-12)} — {D(-12)}; следующий период, его возвращение и правка не тронуты, карточка — {D(0)}",
      per(zw) == [(D(-30), D(-20), "z15 причина 1"), (D(-12), D(-12), "z15 причина 2"), (D(0), "", "")]
      and card(zw) == f"{D(0)}|", f"{J(per(zw), 400)} {card(zw)}")
check("у кого возвращений в журнале нет — периоды как были", rc == 0 and rows(ALL.format(NO_RETURNS)) == quiet,
      f"{len(quiet)} строк")
snap = (rows(ALL.format("")), rows(CARDS))
rc2, people2, added2 = migrate()
check("повторный прогон 090 ничего не меняет: 0 чел., периоды и карточки те же",
      rc == 0 and rc2 == 0 and (people2, added2) == (0, 0) and (rows(ALL.format("")), rows(CARDS)) == snap,
      f"{rc2} {people2} {added2}")

section("карточка после 090")
h = (prof(zu).get("profile") or {}).get("hired_at")
s, _ = api("PUT", f"/admin/staff/{zu}/profile", {"hired_at": h, "phone": "+48500000015"}, "boss")
check("правка карточки z15u (телефон, «Работает с» — как в форме) не двигает начало последнего периода",
      s == 200 and per(zu) == [(D(-20), D(-10), R1), (D(0), D(0), R2), (D(0), "", "")], f"{s} {h} {J(per(zu))}")
done()
