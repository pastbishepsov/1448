#!/usr/bin/env python3
"""З8-и2 «Инкассация и коррекция» (№64 «б», Д11): POST /admin/cash/events. Инкассация — деньги вынули из ящика
в банк или сейф: её вносят админ (обязательно с заметкой «по чьему указанию») и владелец; «должно быть в
ящике» уменьшается на сумму, владельцу — служебная строка, в журнале строку видят и админы. Коррекция «в ящике
сейчас N zł» — только владелец и с причиной: ожидаемое становится N, поправка ложится в «Из чего», строка
журнала — только владельцу. Отменяет событие владелец: ящик возвращается, строка остаётся с пометкой. Пока
ящика нет, коррекции не к чему — 409.
Временный сотрудник z8col; в конце он сдаёт кассу — сошлось, события уходят в прошлый период и следующих
проверок не трогают."""
from _z import *

hire_temp("z8col")
tok("z8col")  # вход в админку открывает смену
p0 = drawer_ready("z8col")
ev = lambda body, who: api("POST", "/admin/cash/events", body, who)
void = lambda eid, who: api("POST", f"/admin/cash/events/{eid}/void", {}, who)
preview = lambda: api("GET", "/admin/work/closeout", None, "z8col")[1]
chat = lambda: api("GET", "/admin/staff-chat", None, "boss")[1].get("messages", [])
owner_lines = lambda seen: [m["text"] for m in chat() if m["id"] not in seen and m.get("kind") == "system" and m.get("visible_to") == "owner"]
mine_out = lambda p: (p.get("mine") or {}).get("out_pln", 0)

section("инкассация от админа — только с заметкой «по чьему указанию» (Д11)")
s, d = ev({"kind": "collect", "amount_pln": 100, "dest": "safe"}, "z8col")
check("без заметки → 400 need_note", s == 400 and d.get("code") == "need_note", f"{s} {J(d)}")
bad = [({"kind": "collect", "amount_pln": 0, "dest": "safe", "note": "владелец"}, "bad_amount", "сумма 0"),
       ({"kind": "collect", "amount_pln": 10.005, "dest": "safe", "note": "владелец"}, "bad_amount", "три знака после запятой"),
       ({"kind": "collect", "amount_pln": 10, "dest": "pocket", "note": "владелец"}, "bad_dest", "куда — не банк и не сейф"),
       ({"kind": "spend", "amount_pln": 10, "dest": "safe", "note": "владелец"}, "bad_kind", "неизвестный вид"),
       ({"kind": "collect", "amount_pln": 10, "dest": "safe", "note": "я" * 201}, "bad_note", "заметка длиннее 200")]
for body, code, what in bad:
    s, d = ev(body, "z8col")
    check(f"{what} → 400 {code}", s == 400 and d.get("code") == code, f"{s} {J(d)}")
seen = {m["id"] for m in chat()}
s, d = ev({"kind": "collect", "amount_pln": 100, "dest": "safe", "note": "по указанию владельца"}, "z8col")
e1 = d.get("event", {})
check("инкассация 100 zł в сейф → 201: вид, сумма, куда, заметка", s == 201 and e1.get("kind") == "collect"
      and near(e1.get("amount_pln", -1), 100) and e1.get("dest") == "safe" and e1.get("note") == "по указанию владельца", f"{s} {J(d, 300)}")
p1 = preview()
check("«должно быть» −100; в «Из чего» — инкассации 100, «из них провёл ты» — ушло 100",
      near(p1.get("expected_pln", 0), p0.get("expected_pln", 0) - 100) and near(p1.get("collections_pln", 0) - p0.get("collections_pln", 0), 100)
      and near(mine_out(p1) - mine_out(p0), 100), J(p1, 300))
check("ответ называет новое «должно быть»", near(d.get("expected_pln", -1), p1.get("expected_pln", -2)), J(d, 200))
lines = owner_lines(seen)
check("владельцу — служебная строка «z8col: инкассация 100.00 zł в сейф — по указанию владельца»",
      any("z8col" in t and "инкассация 100.00 zł в сейф" in t and "по указанию владельца" in t for t in lines), J(lines))
check("журнал: строку инкассации видит и admin2", any("100.00" in i.get("text", "") for i in audit("admin2", kind="cash_collect", limit=10)))

section("коррекция «в ящике сейчас N» — только владелец и с причиной")
s, d = ev({"kind": "correct", "stated_pln": 250, "note": "пересчитал сам"}, "z8col")
check("админ → 403 owner_only", s == 403 and d.get("code") == "owner_only", f"{s} {J(d)}")
s, d = ev({"kind": "correct", "stated_pln": 250}, "boss")
check("без причины → 400 need_reason", s == 400 and d.get("code") == "need_reason", f"{s} {J(d)}")
s, d = ev({"kind": "correct", "stated_pln": -1, "note": "пересчёт"}, "boss")
check("меньше нуля → 400 bad_stated", s == 400 and d.get("code") == "bad_stated", f"{s} {J(d)}")
s, d = ev({"kind": "correct", "stated_pln": 250, "note": "пересчитал ящик вечером"}, "boss")
e2 = d.get("event", {})
p2 = preview()
delta = round(250 - p1.get("expected_pln", 0), 2)
check(f"владелец: «в ящике сейчас 250» → 201, ожидаемое стало 250, поправка {delta:+.2f}", s == 201 and near(p2.get("expected_pln", -1), 250)
      and near(e2.get("amount_pln", 0), delta) and near(e2.get("stated_pln", -1), 250), f"{s} {J(d, 300)}")
check("в «Из чего» — коррекции на ту же поправку", near(p2.get("adjust_pln", 0) - p1.get("adjust_pln", 0), delta), J(p2, 300))
check("журнал: коррекцию видит владелец, admin2 — нет", audit("boss", kind="cash_correct", limit=5)
      and not audit("admin2", kind="cash_correct", limit=5))

section("отмена события — владелец")
s, d = void(e1.get("id"), "z8col")
check("админ отменить не может → 403", s == 403, f"{s} {J(d)}")
s, d = void(e1.get("id"), "boss")
p3 = preview()
check("владелец отменил инкассацию → 200, «должно быть» снова +100", s == 200 and near(p3.get("expected_pln", -1), p2.get("expected_pln", 0) + 100), f"{s} {J(p3, 200)}")
s, d = void(e1.get("id"), "boss")
check("повторная отмена → 409 already_void", s == 409 and d.get("code") == "already_void", f"{s} {J(d)}")
s, d = void("00000000-0000-0000-0000-000000000000", "boss")
check("неизвестное событие → 404 event_not_found", s == 404 and d.get("code") == "event_not_found", f"{s} {J(d)}")
check("журнал: отмена — владельцу, admin2 её не видит", audit("boss", kind="cash_event_void", limit=5)
      and not audit("admin2", kind="cash_event_void", limit=5))

section("сдача после событий: ящик их учёл")
s, d = api("POST", "/admin/work/stop", {"counted_pln": p3.get("expected_pln")}, "z8col")
co = d.get("closeout", {})
check("z8col сдал столько, сколько должно быть → сошлось; коррекция в сдаче, отменённой инкассации нет",
      s == 200 and near(co.get("diff_pln", -1), 0) and near(co.get("adjust_pln", 0), p3.get("adjust_pln", -1))
      and near(co.get("collections_pln", -1), 0), J(co, 300))

section("ящика ещё нет — коррекции не к чему")
hidden = [r for r in rows("WITH old AS (SELECT id, opening_grosz FROM shift_closeouts WHERE opening_grosz IS NOT NULL) "
                          "UPDATE shift_closeouts s SET opening_grosz = NULL FROM old WHERE s.id = old.id "
                          "RETURNING s.id || '|' || old.opening_grosz") if "|" in r]

def unhide():
    if hidden:
        vals = ",".join(f"('{i}'::uuid, {v}::bigint)" for i, v in (h.split("|") for h in hidden))
        sql(f"UPDATE shift_closeouts s SET opening_grosz = v.o FROM (VALUES {vals}) AS v(id, o) WHERE s.id = v.id")
        hidden.clear()
at_end(unhide)
s, d = ev({"kind": "correct", "stated_pln": 250, "note": "пересчитал"}, "boss")
unhide()
check("коррекция без заведённого ящика → 409 no_drawer", s == 409 and d.get("code") == "no_drawer", f"{s} {J(d)}")
done()
