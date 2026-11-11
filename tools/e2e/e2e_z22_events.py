#!/usr/bin/env python3
"""З22-и2 «Событийные алерты» (№59, З-Р28; решения основателя 10.11) — сразу по событию, раз на событие:
«🚪 Смена z22g «z22g Тест» ДД.ММ с 10:00 закрылась сама: уход не отмечен, касса не сдана …» — владельцу, когда табель
сам закрывает забытую смену админа при его следующем «Я пришёл»; «⛔ z22x: попытка отменить чужую операцию — …» и
«… операцию не из своей смены — …» — владельцу, когда сервер не дал админу отменить продажу или пополнение (403), раз
на операцию и админа; «📦 Заканчивается «z22l Тест»: на складе 5, минимум — 5» — всем, когда продажа опустила остаток
до порога «заканчивается» или ниже, раз на такое движение («Закончилось …» — на нуле). Выключатели — «Экономика» →
«Алерты». Фикстуры — временные админы z22g и z22x (в конце увольняются), гость z22q (заводится один раз), шаблон смены
«z22g Тест» и позиции «z22l Тест», «z22v Тест»: продажи и пополнение проверки в конце отменяет владелец; позиции,
шаблон, записи табеля z22g и z22x, отменённые пополнения z22q, журнал алертов о них и их сообщения в канале удаляются в
начале и в конце; настройки — как были."""
import time

from _z import *

FORGOT, VOIDER, GUEST, TPL, GL, GV = "z22g", "z22x", "z22q", "z22g Тест", "z22l Тест", "z22v Тест"
MSG = (f"text LIKE '🚪 Смена {FORGOT} %' OR text LIKE '⛔ {VOIDER}:%' OR text LIKE '📦 %«{GL}»%' "
       f"OR text LIKE '📦 %«{GV}»%'")


def wipe():
    for nick in (FORGOT, VOIDER):
        u = uid(nick)
        if u:
            sql(f"DELETE FROM alerts_sent WHERE kind = 'shift_forgot' AND key IN "
                f"(SELECT 'w/' || id FROM work_entries WHERE user_id = '{u}')")
            sql(f"DELETE FROM alerts_sent WHERE kind = 'void_denied' AND key LIKE '%/{u}'")
            sql(f"DELETE FROM work_entries WHERE user_id = '{u}'")
    for name in (GL, GV):
        for gid in rows(f"SELECT id FROM goods WHERE name = '{name}'"):
            sql(f"DELETE FROM alerts_sent WHERE kind = 'low_stock' AND key IN "
                f"(SELECT 'm/' || id FROM stock_moves WHERE good_id = '{gid}')")
            drop_good(gid)
    for sid in rows(f"SELECT id FROM shifts WHERE name = '{TPL}'"):
        sql(f"UPDATE work_entries SET shift_id = NULL WHERE shift_id = '{sid}'")
        sql(f"DELETE FROM shifts WHERE id = '{sid}'")
    if uid(GUEST):
        sql(f"DELETE FROM deposits WHERE user_id = '{uid(GUEST)}' AND voided_at IS NOT NULL")
    sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND ({MSG})")


wipe()
at_end(wipe)  # в конце — после отмены продаж и пополнения владельцем (уборка идёт в обратном порядке)
sales, deps = [], []
at_end(lambda: [api("POST", f"/admin/sales/{s}/void", {}, "boss") for s in sales if s])
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in deps if d])
if not uid(GUEST):
    s, d = api("POST", "/auth/register", {"nickname": GUEST, "password": GUEST_PASS})
    if s not in (200, 201):
        raise SystemExit(f"регистрация {GUEST}: {s} {J(d)}")
hire_temp(FORGOT)
hire_temp(VOIDER)
set_settings(alerts_on=1, alert_forgot_on=1, alert_void_denied_on=1, alert_low_stock_on=1)
msgs = lambda like: rows(f"SELECT visible_to || '|' || text FROM staff_messages WHERE kind = 'system' AND text LIKE '{like}' "
                         f"ORDER BY created_at")
dm = lambda day: day[8:10] + "." + day[5:7]  # 'ГГГГ-ММ-ДД' → 'ДД.ММ'

section("смена закрылась сама, касса не сдана")
s, tpl = api("POST", "/admin/shifts", {"name": TPL, "start_min": 600, "end_min": 720, "force": True}, "boss")
tok(FORGOT)  # вход открывает смену; ниже она «забыта» позавчера с 10:00 по шаблону «z22g Тест» (10:00–12:00)
zg = uid(FORGOT)
# забытые смены — в днях −2, −3, −4: ночью смена, открытая входом, датирована вчерашним днём (график), и «забытая» во
# вчерашнем совпала бы с ней датой — «Я пришёл» ответил бы 409, а не закрыл её
d2, d3, d4 = club_day(-2), club_day(-3), club_day(-4)


def forget(day, shift_id=None):
    """Открытая смена z22g — «забыта»: в клубный день day с 10:00, с шаблоном shift_id или без, ухода нет."""
    sid, name = (f"'{shift_id}'", TPL) if shift_id else ("NULL", "")
    sql(f"UPDATE work_entries SET date = '{day}', started_at = {at_club(day, '10:00')}, shift_id = {sid}, "
        f"shift_name = '{name}' WHERE user_id = '{zg}' AND ended_at IS NULL")


forget(d2, tpl.get("id"))
s1, r1 = api("POST", "/admin/work/start", {}, FORGOT)
m = msgs(f"🚪 Смена {FORGOT} %")
check(f"«Я пришёл» после забытой смены — табель закрыл её сам, владельцу: «🚪 Смена z22g «{TPL}» {dm(d2)} с 10:00 закрылась "
      "сама: уход не отмечен, касса не сдана … по графику — до 12:00»",
      s == 201 and s1 == 201 and r1.get("auto_closed_previous") is True and len(m) == 1
      and m[0].startswith(f"owner|🚪 Смена {FORGOT} «{TPL}» {dm(d2)} с 10:00 закрылась сама: уход не отмечен, касса не сдана.")
      and "по графику — до 12:00. Проверь табель и ящик" in m[0], J([s, s1, r1, m]))
s2, _ = api("POST", "/admin/work/start", {}, FORGOT)
check("второй «Я пришёл» — 409, алерта не прибавилось", s2 == 409 and len(msgs(f"🚪 Смена {FORGOT} %")) == 1, f"{s2}")
forget(d3)
api("POST", "/admin/work/start", {}, FORGOT)
m = msgs(f"🚪 Смена {FORGOT} %")
check(f"без шаблона смены — «… {dm(d3)} с 10:00 закрылась сама … смены по графику не было — часов ноль»", len(m) == 2
      and m[1].startswith(f"owner|🚪 Смена {FORGOT} {dm(d3)} с 10:00 закрылась сама") and "смены по графику не было — часов ноль"
      in m[1], J(m))
set_settings(alert_forgot_on=0)
forget(d4)
s3, r3 = api("POST", "/admin/work/start", {}, FORGOT)
check("выключатель «Молчать» — смена закрылась сама, а алерта нет", s3 == 201 and r3.get("auto_closed_previous") is True
      and len(msgs(f"🚪 Смена {FORGOT} %")) == 2, J([s3, r3]))

section("попытка отменить чужое или не из своей смены")
gv = (api("POST", "/admin/goods", {"name": GV, "category": "z22", "price_pln": 4, "stock": 10}, "boss")[1].get("good") or {}).get("id")
sell = lambda gid, q=1, who="boss": (api("POST", "/admin/sales", {"good_id": gid, "method": "card", "qty": q}, who)[1].get("sale")
                                     or {}).get("id")
sale = sell(gv)
sales.append(sale)
tok(VOIDER)  # на смене
s1, r1 = api("POST", f"/admin/sales/{sale}/void", {}, VOIDER)
m = msgs(f"⛔ {VOIDER}:%")
check(f"z22x отменяет продажу владельца — 403 not_yours, владельцу: «⛔ z22x: попытка отменить чужую операцию — продажу «{GV}» "
      "4,00 zł (от boss, …); сервер не дал — такое отменяет владелец»",
      s1 == 403 and r1.get("code") == "not_yours" and len(m) == 1
      and m[0].startswith(f"owner|⛔ {VOIDER}: попытка отменить чужую операцию — продажу «{GV}» 4,00 zł (от boss, ")
      and m[0].endswith("); сервер не дал — такое отменяет владелец"), J([s1, r1, m]))
s2, _ = api("POST", f"/admin/sales/{sale}/void", {}, VOIDER)
check("вторая попытка той же продажи — снова 403, алерта не прибавилось (раз на операцию и админа)",
      s2 == 403 and len(msgs(f"⛔ {VOIDER}:%")) == 1, f"{s2}")
s3, r3 = api("POST", f"/admin/users/{uid(GUEST)}/deposit", {"amount_pln": 5, "method": "card"}, VOIDER)
dep = r3.get("deposit_id")
deps.append(dep)
sql(f"UPDATE deposits SET created_at = now() - interval '3 days' WHERE id = '{dep}'")
s4, r4 = api("POST", f"/admin/deposits/{dep}/void", {}, VOIDER)
m = msgs(f"⛔ {VOIDER}:%")
check(f"своё пополнение трёхдневной давности — 403 too_old, владельцу: «… попытка отменить операцию не из своей смены — "
      f"пополнение {GUEST} на 5,00 zł (от z22x, …)»",
      s3 in (200, 201) and s4 == 403 and r4.get("code") == "too_old" and len(m) == 2
      and m[1].startswith(f"owner|⛔ {VOIDER}: попытка отменить операцию не из своей смены — пополнение {GUEST} на 5,00 zł "
                          f"(от {VOIDER}, "), J([s3, s4, r4, m]))
set_settings(alert_void_denied_on=0)
sale2 = sell(gv)
sales.append(sale2)
s5, _ = api("POST", f"/admin/sales/{sale2}/void", {}, VOIDER)
check("выключатель «Молчать» — 403, а алерта нет", s5 == 403 and len(msgs(f"⛔ {VOIDER}:%")) == 2, f"{s5}")

section("товар заканчивается")
gl = (api("POST", "/admin/goods", {"name": GL, "category": "z22", "price_pln": 3, "stock": 7, "low_stock": 5}, "boss")[1]
      .get("good") or {}).get("id")
low = lambda: msgs(f"📦 %«{GL}»%")


def wait_low(n):
    for _ in range(25):
        if len(low()) >= n:
            break
        time.sleep(0.3)
    return low()


sales.append(sell(gl))  # 7 → 6
time.sleep(1.5)
check("7 → 6 при минимуме 5 — молчит", gl and low() == [], J(low()))
sales.append(sell(gl))  # 6 → 5
m = wait_low(1)
check(f"6 → 5 — всем: «📦 Заканчивается «{GL}»: на складе 5, минимум — 5»",
      m == [f"all|📦 Заканчивается «{GL}»: на складе 5, минимум — 5"], J(m))
sales.append(sell(gl))  # 5 → 4
time.sleep(1.5)
check("5 → 4 — уже на минимуме, второго нет (раз на движение, которое перешло порог)", len(low()) == 1, J(low()))
stock_to(gl, 8, "z22 проверка: доложили")
sales.append(sell(gl, 4))  # 8 → 4
m = wait_low(2)
check("доложили до 8 и продали 4 разом — снова перешёл: второй алерт «на складе 4»",
      len(m) == 2 and m[1] == f"all|📦 Заканчивается «{GL}»: на складе 4, минимум — 5", J(m))
stock_to(gl, 6, "z22 проверка: доложили")
set_settings(alert_low_stock_on=0)
sales.append(sell(gl, 2))  # 6 → 4
time.sleep(1.5)
check("выключатель «Молчать» — молчит", len(low()) == 2, J(low()))
set_settings(alert_low_stock_on=1)
stock_to(gl, 6, "z22 проверка: доложили")
sales.append(sell(gl, 6))  # 6 → 0
m = wait_low(3)
check(f"продали всё — «📦 Закончилось «{GL}»: на складе 0, минимум — 5»",
      len(m) == 3 and m[2] == f"all|📦 Закончилось «{GL}»: на складе 0, минимум — 5", J(m))
put = lambda kv: [api("PUT", "/admin/settings", {k: v}, "boss")[0] for k, v in kv]
edge = put((("alert_forgot_on", 1), ("alert_void_denied_on", 1), ("alert_low_stock_on", 1)))
bad = put((("alert_forgot_on", 2), ("alert_void_denied_on", -1), ("alert_low_stock_on", 2)))
check("выключатели — 0 и 1 сохраняются, 2 и −1 — 400", edge == [200, 200, 200] and bad == [400, 400, 400], J([edge, bad]))
done()
