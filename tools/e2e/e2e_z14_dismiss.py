#!/usr/bin/env python3
"""З14-и1 «Увольнение закрывает смену и кассу» (№24; З-Р18; ревью 23.09 №22; уточнено основателем 16.10). Смена
уволенного оставалась открытой навсегда: «открытых смен: 1» в табеле, «на смене» в шапке, минуты сводки росли — а
сам он её уже не закроет, доступа нет. Теперь увольнение и снятие прав закрывают открытую запись табеля моментом
ухода — пометкой «закрыта увольнением» или «закрыта снятием прав», автозакрытой: часы — на проверку владельцу, в
выплату не идут, пока он её не поправит. Кассу за человека система не сдаёт (решение основателя 16.10: уволенного на
месте подменят, остальное доделают за него) — владельцу в канал строка, что осталось за ним: касса не сдана, сколько
должно быть в ящике, часы на проверке. Шторка увольнения и карточка заранее знают открытую смену, несданную кассу и
будущие смены (GET /admin/staff/:id/shift). Давняя смена (уход не отмечен, уволили на следующий день) закрывается по
графику; без открытой смены — как раньше. Временные сотрудники z14d (входит в админку — смена открывается), z14n (без
смены), z14r (на смене — снимают права) и z14s (вчерашняя открытая запись — SQL-ом); все уходят проверкой. Денег не
трогает."""
from _z import *

D, N, R, S = "z14d", "z14n", "z14r", "z14s"
for nick in (D, N, R, S):
    hire_temp(nick)
boss = uid("boss")
chat = lambda who="boss": api("GET", "/admin/staff-chat", None, who)[1].get("messages", [])
owner_lines = lambda seen: [m["text"] for m in chat() if m["id"] not in seen and m.get("kind") == "system" and m.get("visible_to") == "owner"]
preview = lambda nick, who="boss": api("GET", f"/admin/staff/{uid(nick)}/shift", None, who)
cos = lambda nick, t0: sql(f"SELECT count(*) FROM shift_closeouts WHERE user_id = '{uid(nick)}' AND created_at >= '{t0}'")
TODAY = club_day(0)
tomorrow = club_day(1)

def since_of(entry):
    return sql(f"SELECT to_char(started_at AT TIME ZONE '{TZ}', 'HH24:MI') FROM work_entries WHERE id = '{entry}'")

def fresh_audit(seen_a, nick):
    """Строки журнала про nick, появившиеся после seen_a: у каждого вида — последняя."""
    out = {}
    for x in audit(limit=30):
        if x.get("nickname") == nick and x.get("id") not in seen_a:
            out.setdefault(x.get("kind"), x)
    return out

section("увольнение посреди смены")
p = drawer_ready(D)  # вход в админку открывает смену; ящика нет — z14d заводит его пересчётом и выходит снова
entry = sql(f"SELECT id FROM work_entries WHERE user_id = '{uid(D)}' AND ended_at IS NULL")
since = since_of(entry)
shift0 = sql("SELECT id FROM shifts WHERE active ORDER BY sort, start_min LIMIT 1")
sa = api("POST", f"/admin/shifts/{shift0}/assign", {"date": tomorrow, "nickname": D}, "boss")
check("данные: у z14d открыта смена (вход в админку), в ящике должно быть столько-то; на завтра — смена в графике",
      bool(entry) and "expected_pln" in p and sa[0] in (200, 201), f"{entry} {J(p, 200)} {sa[0]} {J(sa[1], 150)}")

s, pv = preview(D)
w, cash = pv.get("open_shift") or {}, pv.get("cash") or {}
check("шторка увольнения: открыта смена — та же запись, этих суток, сколько часов ляжет (сейчас — ноль-несколько минут)",
      s == 200 and w.get("id") == entry and w.get("open") is True and w.get("same_day") is True
      and 0 <= w.get("close_minutes", -1) <= 5, f"{s} {J(pv, 500)}")
check("касса не сдана: ящик заведён, «должно быть» — как в «сдаче» у самого z14d; будущих смен — 1",
      cash.get("drawer") is True and near(cash.get("expected_pln", -1), p["expected_pln"]) and pv.get("future_shifts") == 1,
      f"{J(cash)} vs {p.get('expected_pln')} · future {pv.get('future_shifts')}")
check("админу смена сотрудника не отдаётся — 403", preview(D, "admin1")[0] == 403)

seen, seen_a = {m["id"] for m in chat()}, {x.get("id") for x in audit(limit=50)}
t0 = sql("SELECT now()::text")
s, d = api("POST", f"/admin/staff/{uid(D)}/dismiss", {"reason": "z14 проверка"}, "boss")
cw = d.get("closed_shift") or {}
check("«Уволить» — 200: смена закрыта той же записью — «закрыта увольнением», автозакрытая; снят с 1 будущей смены",
      s == 200 and cw.get("id") == entry and cw.get("note") == "закрыта увольнением" and cw.get("auto_closed") is True
      and cw.get("open") is False and d.get("removed_shifts") == 1, f"{s} {J(d, 400)}")
check("кассу за него никто не сдал: сдачи от z14d нет", "closeout" not in d and cos(D, t0) == "0", f"{J(d, 200)} · {cos(D, t0)}")
row = sql(f"SELECT ended_at IS NOT NULL, auto_closed, note, minutes = floor(extract(epoch FROM ended_at - started_at) / 60)::int, "
          f"edited_by = '{boss}', ended_at >= '{t0}'::timestamptz FROM work_entries WHERE id = '{entry}'")
check("в базе: запись табеля закрыта моментом увольнения, минуты по нему, пометка, правил владелец; открытых у z14d нет",
      row == "t|t|закрыта увольнением|t|t|t" and sql(f"SELECT count(*) FROM work_entries WHERE user_id = '{uid(D)}' AND ended_at IS NULL") == "0", row)
ts = api("GET", f"/admin/work?from={TODAY}&to={TODAY}", None, "boss")[1]
te = next((e for e in ts.get("entries", []) if e.get("id") == entry), {})
check("табель: запись не открыта, автозакрытая «закрыта увольнением», сдачи кассы у неё нет",
      te.get("open") is False and te.get("auto_closed") is True and te.get("note") == "закрыта увольнением" and "closeout" not in te, J(te, 300))
lines = owner_lines(seen)
want = (f"⚠ {D} уволен посреди смены (с {since}) — осталось за него: касса не сдана — в ящике должно быть {cash.get('expected_pln', 0):.2f} zł, "
        "пересчитайте (расхождение до пересчёта ляжет на следующую сдачу); часы — смена закрыта увольнением, ")
check("владельцу в канал — что осталось: «касса не сдана — в ящике должно быть …, пересчитайте; часы — … в табеле на проверку»",
      len(lines) == 1 and lines[0].startswith(want) and lines[0].endswith(" в табеле на проверку"), J(lines))
check("админу эта строка не видна", not any(m.get("text", "").startswith(f"⚠ {D} уволен") for m in chat("admin1")))
au = fresh_audit(seen_a, D)
sd = au.get("staff_dismiss") or {}
check("журнал: «уволен … · z14 проверка, смена с ЧЧ:ММ закрыта (… — на проверку), касса не сдана, снят с 1 будущих смен»; "
      "строки сдачи кассы нет", f"z14 проверка, смена с {since} закрыта (" in sd.get("text", "")
      and "— на проверку), касса не сдана, снят с 1 будущих смен" in sd.get("text", "") and "shift_close" not in au, J(au, 400))
check("админу журнал этого не показывает", not [x for x in audit("admin1", limit=50) if x.get("nickname") == D and x.get("kind") == "staff_dismiss"])

section("«Снять права» посреди смены — так же")
tok(R)  # вход в админку открывает смену
er = sql(f"SELECT id FROM work_entries WHERE user_id = '{uid(R)}' AND ended_at IS NULL")
since_r = since_of(er)
seen, seen_a = {m["id"] for m in chat()}, {x.get("id") for x in audit(limit=50)}
t0 = sql("SELECT now()::text")
s, d = api("DELETE", f"/admin/staff/{uid(R)}", None, "boss")
cw = d.get("closed_shift") or {}
check("«Снять права» — 200, роль player: смена закрыта — «закрыта снятием прав», автозакрытая; сдачи нет",
      s == 200 and d.get("role") == "player" and cw.get("id") == er and cw.get("note") == "закрыта снятием прав"
      and cw.get("auto_closed") is True and cos(R, t0) == "0", f"{s} {J(d, 400)}")
lines = owner_lines(seen)
check("владельцу в канал — «z14r снят с прав посреди смены (с ЧЧ:ММ) — осталось за него: касса не сдана …; часы — смена "
      "закрыта снятием прав …»", len(lines) == 1 and lines[0].startswith(f"⚠ {R} снят с прав посреди смены (с {since_r}) — осталось за него: касса не сдана — ")
      and "; часы — смена закрыта снятием прав, " in lines[0], J(lines))
sm = fresh_audit(seen_a, R).get("staff_demote") or {}
check("журнал: «z14r → player, смена с ЧЧ:ММ закрыта (… — на проверку), касса не сдана»",
      sm.get("text", "").startswith(f"{R} → player, смена с {since_r} закрыта (") and sm.get("text", "").endswith("— на проверку), касса не сдана"), J(sm, 300))

section("без открытой смены — как раньше")
seen = {m["id"] for m in chat()}
s, pv = preview(N)
check("шторка: открытой смены нет, кассы в ней нет", s == 200 and pv.get("open_shift") is None and "cash" not in pv, J(pv, 300))
s, d = api("POST", f"/admin/staff/{uid(N)}/dismiss", {}, "boss")
check("«Уволить» — 200 без «смена закрыта»; владельцу ничего не приходит — доделывать за ним нечего",
      s == 200 and "closed_shift" not in d and not [x for x in owner_lines(seen) if N in x], f"{s} {J(d, 300)}")

section("давняя смена: ушёл не отметившись, уволили на следующий день")
yday = club_day(-1)
sh = sql("SELECT id || '|' || start_min || '|' || end_min FROM shifts WHERE active AND start_min < end_min ORDER BY start_min LIMIT 1")
if not check("в графике есть дневная смена", bool(sh)):
    done()
sid, smin, emin = sh.split("|")
start_hm = f"{(int(smin) + 60) // 60:02d}:{(int(smin) + 60) % 60:02d}"
end_hm = f"{int(emin) // 60:02d}:{int(emin) % 60:02d}"
club = sql(f"SELECT club_id FROM work_entries WHERE id = '{entry}'")
old = sql(f"INSERT INTO work_entries (club_id, user_id, shift_id, date, started_at, note, created_by) VALUES "
          f"('{club}', '{uid(S)}', '{sid}', '{yday}', {at_club(yday, start_hm)}, 'открыта входом в админку', '{uid(S)}') RETURNING id")
mins = int(emin) - int(smin) - 60
s, pv = preview(S)
w = pv.get("open_shift") or {}
check(f"шторка: смена вчерашних суток — не этих, закроется по графику: {mins // 60} ч {mins % 60:02d} м",
      s == 200 and w.get("id") == old and w.get("same_day") is False and w.get("close_minutes") == mins, J(pv, 400))
seen = {m["id"] for m in chat()}
s, d = api("POST", f"/admin/staff/{uid(S)}/dismiss", {}, "boss")
row = sql(f"SELECT ended_at = {at_club(yday, end_hm)}, minutes, auto_closed, note FROM work_entries WHERE id = '{old}'")
check("закрыта концом смены по графику, часы по нему, пометка «закрыта увольнением»",
      s == 200 and row == f"t|{mins}|t|закрыта увольнением" and (d.get("closed_shift") or {}).get("id") == old, row)
lines = owner_lines(seen)
dd = yday[8:10] + "." + yday[5:7]
check(f"в канал — с датой: «z14s уволен, а смена с {dd} {start_hm} осталась незакрытой — осталось за него: … часы — смена закрыта "
      f"по графику, {mins // 60} ч {mins % 60:02d} м в табеле на проверку»",
      len(lines) == 1 and lines[0].startswith(f"⚠ {S} уволен, а смена с {dd} {start_hm} осталась незакрытой — осталось за него: касса не сдана")
      and lines[0].endswith(f"; часы — смена закрыта по графику, {mins // 60} ч {mins % 60:02d} м в табеле на проверку"), J(lines))
done()
