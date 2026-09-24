#!/usr/bin/env python3
"""З1-и2 «Служебные сообщения и „только владельцу“» (№59, Д4): деньги сдачи кассы — должно, пересчитано,
расхождение — уходят в канал персонала служебным сообщением, которое видит только владелец. Админ не видит
его ни в ленте канала, ни в счётчике непрочитанного, ни в live-канале, а строку журнала shift_close — ни в
журнале, ни в ленте зала. Заметка «передал смену» — обычное сообщение сдавшего, её видят все.
Временный сотрудник z1ch, гость Guest32. Идемпотентен: пополнения отменяет, своё тестовое сообщение
удаляет, z1ch уволен."""
from _z import *

zid, a2id = hire_temp("z1ch"), uid("admin2")
deps, posted = [], []
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in reversed(deps)])
at_end(lambda: [sql(f"DELETE FROM staff_messages WHERE id='{m}'") for m in posted])
chat = lambda who: api("GET", "/admin/staff-chat", None, who)[1]
read = lambda who: api("POST", "/admin/staff-chat/read", {}, who)
fresh = lambda who, seen: [m for m in chat(who)["messages"] if m["id"] not in seen]
ids = lambda who: {m["id"] for m in chat(who)["messages"]}
money = lambda m: any(w in m.get("text", "") for w in ("zł", "должно", "расхожд", "пересч", "сошлась", "в ящике"))
ticket = lambda who: api("POST", "/admin/ws-ticket", {}, who)[1]["ticket"]
live = lambda s: [e.get("data", {}) for e in ws_read(s, 2)[0] if e.get("type") == "staff_chat"]

section("сдача с недостачей: деньги — владельцу, заметка — всем")
tok("z1ch")  # вход открывает смену (Е3-и1)
deps.append(topup("Guest32", 100, method="cash", who="z1ch"))
for who in ("boss", "admin2"):
    read(who)
seen = {who: ids(who) for who in ("boss", "admin2")}
wsb, wsa = ws_admin(ticket("boss"))[0], ws_admin(ticket("admin2"))[0]
s, d = api("POST", "/admin/work/stop", {"counted_pln": 80, "note": "ключи от шкафа на стойке"}, "z1ch")
check("z1ch сдал 80 из 100 → 200, расхождение −20", s == 200 and near(d.get("closeout", {}).get("diff_pln", 0), -20), f"{s} {J(d, 200)}")
eb, ea = live(wsb), live(wsa)
wsb.close(); wsa.close()
nb, na = fresh("boss", seen["boss"]), fresh("admin2", seen["admin2"])
sysm = [m for m in nb if m.get("kind") == "system"]
check("владельцу — служебное «только владельцу» без автора: кто сдал, 80 из 100, расхождение −20",
      len(sysm) == 1 and sysm[0].get("visible_to") == "owner" and not sysm[0].get("author_id")
      and all(w in sysm[0]["text"] for w in ("z1ch", "80.00", "100.00", "−20.00")), J(nb, 400))
note = [m for m in nb if m.get("author_id") == zid]
check("заметка — отдельным сообщением от z1ch, без денег, для всех", len(note) == 1 and "ключи от шкафа на стойке" in note[0]["text"]
      and not money(note[0]) and note[0].get("kind") == "text" and note[0].get("visible_to") == "all", J(note, 300))
check("admin2 видит заметку z1ch", any(m.get("author_id") == zid and "ключи от шкафа" in m["text"] for m in na), J(na, 300))
check("admin2 не видит денег сдачи в канале", not any(money(m) for m in na), J(na, 300))
check("счётчик владельца: 2 новых — деньги и заметка", chat("boss")["unread"] == 2, chat("boss")["unread"])
check("счётчик admin2: 1 новое — только заметка", chat("admin2")["unread"] == 1, chat("admin2")["unread"])
check("live-канал владельца: служебное и заметка", len(eb) == 2 and any(e.get("kind") == "system" for e in eb), J(eb))
check("live-канал admin2: только заметка — о служебном он не узнаёт", len(ea) == 1 and ea[0].get("kind") != "system", J(ea))

section("журнал: строка сдачи с суммами — только владельцу")
check("журнал владельца: shift_close z1ch с «расхождение -20.00»",
      any(i.get("admin") == "z1ch" and "расхождение -20.00" in i["text"] for i in audit("boss", kind="shift_close", limit=5)))
check("журнал admin2 по виду shift_close — пусто", not audit("admin2", kind="shift_close", limit=5))
rows_a2 = audit("admin2", limit=100)
check("журнал admin2 без фильтра (им же живёт лента зала): ни строки сдачи, ни её сумм",
      not any(i.get("kind") == "shift_close" or any(w in i.get("text", "") for w in ("должно быть", "пересчитано", "расхождение"))
              for i in rows_a2), J([i for i in rows_a2 if i.get("admin") == "z1ch"], 300))

section("«прочитано» — у каждого своё")
read("admin2")
check("admin2 прочитал: у него 0, у владельца по-прежнему 2", chat("admin2")["unread"] == 0 and chat("boss")["unread"] == 2,
      f"{chat('admin2')['unread']} / {chat('boss')['unread']}")

section("уход без пересчёта и без заметки: в канале — только строка владельцу")
for who in ("boss", "admin2"):
    read(who)
seen = {who: ids(who) for who in ("boss", "admin2")}
api("POST", "/admin/work/start", {}, "z1ch")
s, d = api("POST", "/admin/work/stop", {}, "z1ch")
nb, na = fresh("boss", seen["boss"]), fresh("admin2", seen["admin2"])
check("владельцу: одно служебное «сдал смену без пересчёта кассы»", s == 200 and len(nb) == 1 and nb[0].get("kind") == "system"
      and "без пересчёта" in nb[0]["text"] and chat("boss")["unread"] == 1, J(nb, 300))
check("admin2: ничего нового, счётчик 0", not na and chat("admin2")["unread"] == 0, J(na, 300))

section("служебное нельзя подделать с клиента")
s, d = api("POST", "/admin/staff-chat", {"text": "z1: обычное сообщение, не служебное", "kind": "system", "visible_to": "owner"}, "admin2")
m = d.get("message", {})
if m.get("id"):
    posted.append(m["id"])
check("kind/visible_to из тела игнорируются: обычное сообщение admin2 для всех", s == 201 and m.get("kind") == "text"
      and m.get("visible_to") == "all" and m.get("author_id") == a2id, f"{s} {J(d, 300)}")
has = sql("SELECT count(*) FROM information_schema.columns WHERE table_name='staff_messages' AND column_name IN ('kind','visible_to')") == "2"
check("в базе у сообщений есть kind и visible_to", has)
if has:
    check("старые строки сдачи (до З1-и2 — деньги и заметка от имени админа) после миграции — только владельцу",
          sql("SELECT count(*) FROM staff_messages WHERE text LIKE '🧮 Сдал смену%' AND visible_to <> 'owner'") == "0")
    check("остальные сообщения с автором — обычные и для всех",
          sql("SELECT count(*) FROM staff_messages WHERE author_id IS NOT NULL AND text NOT LIKE '🧮 Сдал смену%' "
              "AND (kind <> 'text' OR visible_to <> 'all')") == "0")
done()
