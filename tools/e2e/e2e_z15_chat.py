#!/usr/bin/env python3
"""З15-и2 «Канал: «ещё» и честное «прочитано»» (№27, З-Р20). Лента канала персонала отдавала последние 100 сообщений, а
открытие шторки ставило «прочитано» на «сейчас»: 130 непрочитанных — 30 старших не достать, счётчик сброшен, пришедшее,
пока лента открывалась, — тоже «прочитано». Теперь открытие отдаёт все непрочитанные (до 500), но не меньше 100; «Ещё» —
?before= (как у журнала) с has_more и next_before; «прочитано» — до показанного (until — id сообщения), назад не ходит;
строк «только владельцу» админ не видит ни в страницах, ни в отметке; без until — как раньше, «до сейчас». В «Персонале» —
расхождения касс за 7 клубных суток из самих сдач: недостача z15d видна, объяснённая расходом «из кассы» — с пометкой и
не в «не объяснено». Данные: 130 сообщений z15c и 5 служебных «только владельцу» — SQL-ом подряд, по миллисекунде, в
уборке удаляются вместе со всем «z15 канал …»; читатели — временный z15r и boss (его отметка «прочитано» возвращается);
z15d сдаёт кассу с недостачей 1 zł (сдача остаётся в книге), расход «из кассы» 1 zł в конце сторнируется."""
import urllib.parse
from datetime import datetime

from _z import *

C, R, DD = "z15c", "z15r", "z15d"
for nick in (C, R, DD):
    hire_temp(nick)
MARK = "z15 канал"
drop = lambda: sql(f"DELETE FROM staff_messages WHERE text LIKE '{MARK} %'")
drop()  # хвосты упавшего прогона
at_end(drop)
B0 = sql("SELECT COALESCE(staff_chat_read_at::text, '') FROM users WHERE nickname = 'boss'")
at_end(lambda: sql("UPDATE users SET staff_chat_read_at = " + (f"'{B0}'::timestamptz" if B0 else "NULL") + " WHERE nickname = 'boss'"))
EXP = "z15 канал: из кассы"


def exp_cleanup():
    for i in rows(f"SELECT id FROM expenses WHERE note = '{EXP}' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")


exp_cleanup()
at_end(exp_cleanup)
ts = lambda v: datetime.fromisoformat(v) if v else None
page = lambda who, q="": api("GET", "/admin/staff-chat" + q, None, who)
read = lambda who, body: api("POST", "/admin/staff-chat/read", body, who)
ours = lambda msgs: [m for m in msgs if (m.get("text") or "").startswith(MARK + " #")]
texts = lambda msgs: [m.get("text") for m in msgs]
num = lambda i: f"{MARK} #{i:03d}"
mid = lambda i: sql(f"SELECT id FROM staff_messages WHERE text = '{num(i)}'")

section("данные: 130 сообщений z15c и 5 строк «только владельцу» — подряд, по миллисекунде")
club = sql("SELECT id FROM clubs ORDER BY created_at LIMIT 1")
sql(f"INSERT INTO staff_messages (club_id, author_id, role, kind, visible_to, text, created_at) SELECT '{club}', '{uid(C)}', 'admin', "
    f"'text', 'all', '{MARK} #' || lpad(i::text, 3, '0'), now() - ((140 - i) || ' milliseconds')::interval FROM generate_series(1, 130) i")
sql(f"INSERT INTO staff_messages (club_id, author_id, role, kind, visible_to, text, created_at) SELECT '{club}', NULL, 'system', "
    f"'system', 'owner', '{MARK} служебное #' || k, m.created_at + interval '500 microseconds' FROM generate_series(1, 5) k "
    f"JOIN staff_messages m ON m.text = '{MARK} #' || lpad((100 + 5 * k)::text, 3, '0')")  # через полмиллисекунды после #105, #110, … #125
first_at = sql(f"SELECT created_at FROM staff_messages WHERE text = '{num(1)}'")
sql(f"UPDATE users SET staff_chat_read_at = timestamptz '{first_at}' - interval '1 millisecond' WHERE nickname = '{R}'")
n = sql(f"SELECT count(*) FROM staff_messages WHERE text LIKE '{MARK} %'")
check("в канале 135 строк проверки, z15r дочитал до них", n == "135", n)

section("открытие: все непрочитанные, «Ещё» — страница старше")
s, p = page(R)
mine = ours(p.get("messages", []))
check("130 непрочитанных — все в первой странице, хоть их больше 100: с #001 по #130, старые сверху; счётчик 130; строк «только "
      "владельцу» у админа нет", s == 200 and texts(mine) == [num(i) for i in range(1, 131)] and len(p.get("messages", [])) == 130
      and p.get("unread") == 130 and not any(m.get("visible_to") == "owner" for m in p.get("messages", [])),
      f"{s} n={len(p.get('messages', []))} unread={p.get('unread')} {texts(mine)[:2]}…{texts(mine)[-1:]}")
check("старше — есть: has_more и next_before — время #001", p.get("has_more") is True and ts(p.get("next_before")) == ts(mine[0].get("created_at")) if mine else False,
      f"{p.get('has_more')} {p.get('next_before')}")
s, p2 = page(R, "?before=" + urllib.parse.quote(p.get("next_before") or ""))
older = p2.get("messages", [])
t_first = ts(mine[0]["created_at"]) if mine else None
left = int(sql(f"SELECT count(*) FROM staff_messages WHERE visible_to = 'all' AND created_at < "
               f"timestamptz '{older[0]['created_at']}'")) if older else -1
check("«Ещё» (?before=next_before): до 100 строк раньше #001, ни одной из 130, старые сверху; has_more — есть ли ещё старше",
      s == 200 and 0 < len(older) <= 100 and not ours(older) and all(ts(m["created_at"]) < t_first for m in older)
      and [ts(m["created_at"]) for m in older] == sorted(ts(m["created_at"]) for m in older) and p2.get("has_more") is (left > 0)
      and (left == 0 or ts(p2.get("next_before")) == ts(older[0]["created_at"])), f"{s} n={len(older)} has_more={p2.get('has_more')} старше={left}")
s, bad = page(R, "?before=" + urllib.parse.quote("вчера"))
check("кривой before — 400 bad_before", s == 400 and bad.get("code") == "bad_before", f"{s} {J(bad)}")

section("«прочитано» — до показанного, назад не ходит")
s, r1 = read(R, {"until": mine[99]["id"]})
check("«прочитано» до #100 — непрочитанных 30, отметка — время #100", s == 200 and r1.get("unread") == 30
      and ts(r1.get("read_at")) == ts(mine[99]["created_at"]), f"{s} {J(r1)}")
s, r2 = read(R, {"until": mine[49]["id"]})
check("потом до #50 — отметка назад не ходит: непрочитанных по-прежнему 30", s == 200 and r2.get("unread") == 30
      and ts(r2.get("read_at")) == ts(mine[99]["created_at"]), f"{s} {J(r2)}")
sys1 = sql(f"SELECT id FROM staff_messages WHERE text = '{MARK} служебное #1'")
s, e1 = read(R, {"until": sys1})
s2, e2 = read(R, {"until": "не-id"})
s3, e3 = read(R, {"until": "00000000-0000-0000-0000-000000000000"})
check("админ: до строки «только владельцу» — 404 message_not_found (её для него нет), до несуществующей — тоже, кривой until — "
      "400 bad_until; отметка на месте", s == 404 and e1.get("code") == "message_not_found" and s3 == 404 and s2 == 400
      and e2.get("code") == "bad_until" and page(R)[1].get("unread") == 30, f"{s} {J(e1)} · {s2} {J(e2)} · {s3}")
s, p3 = page(R)
shown = p3.get("messages", [])
check("непрочитанных 30 — открытие отдаёт обычные 100 последних: с #031 по #130", s == 200 and len(shown) == 100
      and texts(ours(shown)) == [num(i) for i in range(31, 131)], f"{len(shown)} {texts(ours(shown))[:1]}")
s, n131 = api("POST", "/admin/staff-chat", {"text": num(131)}, C)  # пришло, пока лента открывалась
s, r3 = read(R, {"until": shown[-1]["id"] if shown else ""})
check("лента показала до #130, пока открывалась — пришло #131: «прочитано» до показанного — непрочитанным осталось одно",
      shown and shown[-1].get("text") == num(130) and r3.get("unread") == 1, f"{J(r3)}")
s, r4 = read(R, {"until": (n131.get("message") or {}).get("id")})
check("дочитал до #131 — непрочитанных нет", s == 200 and r4.get("unread") == 0, f"{s} {J(r4)}")
s, p4 = page(R)
check("всё прочитано — снова 100 последних: с #032 по #131, старше — «Ещё»", len(p4.get("messages", [])) == 100
      and texts(ours(p4.get("messages", []))) == [num(i) for i in range(32, 132)] and p4.get("has_more") is True,
      f"{len(p4.get('messages', []))} {texts(ours(p4.get('messages', [])))[:1]}")

section("владелец: строки «только владельцу» — на своих местах")
sql(f"UPDATE users SET staff_chat_read_at = timestamptz '{first_at}' - interval '1 millisecond' WHERE nickname = 'boss'")
s, pb = page("boss")
mb = [m for m in pb.get("messages", []) if (m.get("text") or "").startswith(MARK)]
want = []
for i in range(1, 132):
    want.append(num(i))
    if i in (105, 110, 115, 120, 125):
        want.append(f"{MARK} служебное #{(i - 100) // 5}")
check("boss: 136 непрочитанных — все в первой странице, служебные «только владельцу» — между #105, #110, … как были написаны",
      s == 200 and texts(mb) == want and pb.get("unread") == 136, f"{s} unread={pb.get('unread')} n={len(mb)}")
s, rb = read("boss", {"until": sys1})
check("boss: «прочитано» до служебной строки после #105 — можно, непрочитанных 30 (#106…#131 и четыре служебные)",
      s == 200 and rb.get("unread") == 30, f"{s} {J(rb)}")
s, rb2 = read("boss", {})
check("без until (страница, открытая до обновления) — как раньше, «до сейчас»: непрочитанных нет", s == 200 and rb2.get("unread") == 0,
      f"{s} {J(rb2)}")

section("«Персонал»: расхождения касс за 7 дней")
tok(DD)  # вход в админку открывает смену
pre = drawer_ready(DD)
s, stp = api("POST", "/admin/work/stop", {"counted_pln": round(pre.get("expected_pln", 0) - 1, 2)}, DD)
co = ((stp or {}).get("closeout") or {}).get("id")
check("z15d сдал кассу на 1,00 меньше, чем должно быть", s == 200 and co and near(stp["closeout"].get("diff_pln", 0), -1), f"{s} {J(stp, 200)}")
s, st = api("GET", "/admin/staff", None, "boss")
cd = st.get("cash_diffs") or {}
it = next((x for x in cd.get("items", []) if x.get("id") == co), {})
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
n7 = int(sql(f"SELECT count(*) FROM shift_closeouts WHERE created_at >= {at_club(club_day(-6), f'{rh:02d}:00')} AND diff_grosz IS NOT NULL "
             f"AND diff_grosz <> 0 AND NOT baseline"))
check("недостача z15d — в «расхождениях за 7 дней»: кто, должно, пересчитано, −1,00, не объяснена", s == 200 and it.get("nickname") == DD
      and near(it.get("diff_pln", 0), -1) and it.get("open") is True and near(it.get("expected_pln", 0) - it.get("counted_pln", 0), 1), J(it, 300))
check(f"счётчик — все пересчёты не в ноль с {club_day(-6)} {rh:02d}:00 (как «7 дней» в отчётах), свежие сверху; "
      "«не объяснено» — сумма недостач и излишков без объяснённого", cd.get("count") == n7 == len(cd.get("items", [])) and cd.get("from") == club_day(-6)
      and [ts(x["created_at"]) for x in cd.get("items", [])] == sorted((ts(x["created_at"]) for x in cd.get("items", [])), reverse=True)
      and cd.get("open", 0) >= 1 and cd.get("short_pln", 0) >= 1,
      f"count={cd.get('count')} sql={n7} from={cd.get('from')} open={cd.get('open')} short={cd.get('short_pln')} over={cd.get('over_pln')}")
cats = {c.get("code"): c.get("id") for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", [])}
s, ex = api("POST", "/admin/expenses", {"op_date": club_day(0), "category_id": cats.get("other"), "amount_pln": 1, "method": "till",
                                        "note": EXP, "before_closeout": co}, "boss")
s2, st2 = api("GET", "/admin/staff", None, "boss")
cd2 = st2.get("cash_diffs") or {}
it2 = next((x for x in cd2.get("items", []) if x.get("id") == co), {})
check("расход «из кассы» 1,00 до этой сдачи её объяснил: в списке осталась — «объяснено 1,00», в «не объяснено» её больше нет",
      s == 201 and it2.get("explained_pln") == 1 and it2.get("open") is False and cd2.get("count") == cd.get("count")
      and cd2.get("open") == cd.get("open") - 1 and near(cd2.get("short_pln", 0), cd.get("short_pln", 0) - 1),
      f"{s} {J(it2, 200)} open {cd.get('open')}→{cd2.get('open')} short {cd.get('short_pln')}→{cd2.get('short_pln')}")
s, sa = api("GET", "/admin/staff", None, "admin1")
check("админу «Персонал» владельца закрыт, расхождений касс он не видит — 403", s == 403 and "cash_diffs" not in sa, f"{s} {J(sa)}")
done()
