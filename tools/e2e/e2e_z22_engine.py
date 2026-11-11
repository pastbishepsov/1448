#!/usr/bin/env python3
"""З22-и1 «Движок алертов» (№59, З-Р28). Миграция 097 — журнал alerts_sent (вид, ключ, когда; первичный ключ — вид и
ключ); POST /admin/alerts/test (владелец; админу — 403) — проверочный алерт в канал персонала: служебное сообщение без
автора, видит только владелец, раз за клубные сутки (ключ «ГГГГ-ММ-ДД/test»); пять одновременных запросов — одно
сообщение; «Экономика» → «Алерты»: alerts_on 0 — молчать (sent: false, reason off), 1 — снова присылать. Уборка:
проверочные алерты (строки alerts_sent и сообщения канала) — в начале и в конце; alerts_on — как был."""
from concurrent.futures import ThreadPoolExecutor

from _z import *

TEXT = "Проверка алертов: так алерт выглядит в канале персонала. Этот видит только владелец."


def wipe():
    if sql("SELECT to_regclass('alerts_sent') IS NOT NULL") == "t":
        sql("DELETE FROM alerts_sent WHERE kind = 'test'")
    sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND text = '{TEXT}'")


wipe()
at_end(wipe)
test = lambda who="boss": api("POST", "/admin/alerts/test", {}, who)
msgs = lambda: int(sql(f"SELECT COUNT(*) FROM staff_messages WHERE kind = 'system' AND text = '{TEXT}'"))
in_chat = lambda who: any(m.get("text") == TEXT for m in (api("GET", "/admin/staff-chat", None, who)[1] or {}).get("messages") or [])

section("журнал отправленных")
cols = rows("SELECT column_name || ':' || data_type FROM information_schema.columns WHERE table_name = 'alerts_sent' "
            "ORDER BY ordinal_position")
pk = rows("SELECT a.attname FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey) "
          "WHERE i.indrelid = to_regclass('alerts_sent') AND i.indisprimary ORDER BY a.attname")
check("миграция 097: alerts_sent — вид, ключ, когда ушёл; первичный ключ — вид и ключ",
      cols == ["kind:character varying", "key:character varying", "sent_at:timestamp with time zone"] and pk == ["key", "kind"],
      J([cols, pk]))

section("проверочный алерт")
s_adm = test("admin1")[0]
s1, r1 = test()
if s1 == 404 or sql("SELECT to_regclass('alerts_sent') IS NOT NULL") != "t":  # движка нет (до З22-и1) — FAIL, а не обрыв
    for name in ("проверочный: 403 админу, раз за сутки", "одно служебное сообщение", "видит только владелец", "ключ — сутки и предмет",
                 "гонка", "выключатель 0", "выключатель 1", "вне 0…1 — 400"):
        check(f"{name}: движка нет", False, J([s_adm, s1, r1]))
    done()
s2, r2 = test()
check("admin1 — 403; владелец — ушёл (sent: true), второй раз за те же сутки — нет (already)",
      s_adm == 403 and s1 == 200 and r1 == {"sent": True} and r2 == {"sent": False, "reason": "already"}, J([s_adm, s1, r1, s2, r2]))
row = rows(f"SELECT COALESCE(author_id::text, '') || '|' || role || '|' || kind || '|' || visible_to FROM staff_messages "
           f"WHERE kind = 'system' AND text = '{TEXT}'")
check("в канале — одно служебное сообщение: без автора, роль и вид system, видит только владелец",
      row == ["|system|system|owner"], J(row))
check("владелец видит его в канале, админ — нет", in_chat("boss") and not in_chat("admin1"), "")
check(f"ключ — клубные сутки и предмет: «{club_day(0)}/test»",
      rows("SELECT key FROM alerts_sent WHERE kind = 'test'") == [f"{club_day(0)}/test"], J(rows("SELECT key FROM alerts_sent")))

section("гонка: пять запросов разом")
wipe()
with ThreadPoolExecutor(5) as ex:
    got = list(ex.map(lambda _: test()[1], range(5)))
check("ушёл ровно один, сообщение одно — остальные упёрлись в журнал",
      sum(1 for r in got if r.get("sent")) == 1 and msgs() == 1, J(got))

section("выключатель «Алерты в канал»")
wipe()
set_settings(alerts_on=0)
s3, r3 = test()
check("alerts_on 0 — молчит: sent: false, reason off; ни сообщения, ни строки журнала",
      r3 == {"sent": False, "reason": "off"} and msgs() == 0 and sql("SELECT COUNT(*) FROM alerts_sent WHERE kind = 'test'") == "0",
      J([s3, r3]))
set_settings(alerts_on=1)
s4, r4 = test()
check("alerts_on 1 — снова присылает", r4 == {"sent": True} and msgs() == 1, J([s4, r4]))
s5, d5 = api("PUT", "/admin/settings", {"alerts_on": 2}, "boss")
check("alerts_on вне 0…1 — 400", s5 == 400, J([s5, d5]))
done()
