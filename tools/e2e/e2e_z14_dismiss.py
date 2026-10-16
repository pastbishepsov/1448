#!/usr/bin/env python3
"""З14-и1 «Увольнение закрывает смену и кассу» (№24; З-Р18; ревью 23.09 №22). Смена уволенного оставалась открытой
навсегда: «открытых смен: 1» в табеле, минуты сводки росли с каждым просмотром, сдачи кассы не было — а сам он её
уже не закроет, доступа нет. Теперь увольнение закрывает открытую запись табеля пометкой «закрыта увольнением»
(автозакрытая: часы — на проверку владельцу, в выплату не идут, пока он её не поправит) и пишет сдачу кассы «без
пересчёта» по ящику клуба; владельцу — строка в канал, в журнале — «смена с ЧЧ:ММ закрыта» и сдача с уволенным.
Шторка увольнения заранее знает открытую смену, несданную кассу и будущие смены (GET /admin/staff/:id/dismiss).
Давняя смена (уход не отмечен, уволили на следующий день) закрывается по графику; без открытой смены увольнение —
как раньше. Временные сотрудники z14d (входит в админку — смена открывается), z14n (без смены) и z14s (вчерашняя
открытая запись — SQL-ом); все уволены проверкой. Денег не трогает."""
from _z import *

D, N, S = "z14d", "z14n", "z14s"
for nick in (D, N, S):
    hire_temp(nick)
boss = uid("boss")
chat = lambda who="boss": api("GET", "/admin/staff-chat", None, who)[1].get("messages", [])
owner_lines = lambda seen: [m["text"] for m in chat() if m["id"] not in seen and m.get("kind") == "system" and m.get("visible_to") == "owner"]
preview = lambda nick, who="boss": api("GET", f"/admin/staff/{uid(nick)}/dismiss", None, who)
TODAY = club_day(0)
tomorrow = club_day(1)

section("увольнение посреди смены")
p = drawer_ready(D)  # вход в админку открывает смену; ящика нет — z14d заводит его пересчётом и выходит снова
entry = sql(f"SELECT id FROM work_entries WHERE user_id = '{uid(D)}' AND ended_at IS NULL")
since = sql(f"SELECT to_char(started_at AT TIME ZONE '{TZ}', 'HH24:MI') FROM work_entries WHERE id = '{entry}'")
shift0 = sql("SELECT id FROM shifts WHERE active ORDER BY sort, start_min LIMIT 1")
sa = api("POST", f"/admin/shifts/{shift0}/assign", {"date": tomorrow, "nickname": D}, "boss")
check("данные: у z14d открыта смена (вход в админку), в ящике должно быть столько-то; на завтра — смена в графике",
      bool(entry) and "expected_pln" in p and sa[0] in (200, 201), f"{entry} {J(p, 200)} {sa[0]} {J(sa[1], 150)}")

s, pv = preview(D)
w, cash = pv.get("open_shift") or {}, pv.get("cash") or {}
check("шторка увольнения: открыта смена — та же запись, этих суток, начало, сколько часов ляжет (сейчас — ноль-несколько минут)",
      s == 200 and w.get("id") == entry and w.get("open") is True and w.get("same_day") is True
      and 0 <= w.get("close_minutes", -1) <= 5, f"{s} {J(pv, 500)}")
check("касса не сдана: ящик заведён, «должно быть» — как в «сдаче» у самого z14d; будущих смен — 1",
      cash.get("drawer") is True and near(cash.get("expected_pln", -1), p["expected_pln"]) and pv.get("future_shifts") == 1,
      f"{J(cash)} vs {p.get('expected_pln')} · future {pv.get('future_shifts')}")
check("админу шторка увольнения не отдаётся — 403", preview(D, "admin1")[0] == 403)

seen = {m["id"] for m in chat()}
seen_a = {x.get("id") for x in audit(limit=50)}
t0 = sql("SELECT now()::text")
s, d = api("POST", f"/admin/staff/{uid(D)}/dismiss", {"reason": "z14 проверка"}, "boss")
cw, co = d.get("closed_shift") or {}, d.get("closeout") or {}
check("«Уволить» — 200: смена закрыта той же записью — «закрыта увольнением», автозакрытая; снят с 1 будущей смены",
      s == 200 and cw.get("id") == entry and cw.get("note") == "закрыта увольнением" and cw.get("auto_closed") is True
      and cw.get("open") is False and d.get("removed_shifts") == 1, f"{s} {J(d, 400)}")
check("касса — сдача без пересчёта той же смены: «закрыта увольнением», должно быть — как в шторке, пересчитанного нет",
      co.get("skipped") is True and co.get("work_entry_id") == entry and co.get("note") == "закрыта увольнением"
      and near(co.get("expected_pln", -1), cash.get("expected_pln", -2)) and "counted_pln" not in co, J(co, 400))
row = sql(f"SELECT ended_at IS NOT NULL, auto_closed, note, minutes = floor(extract(epoch FROM ended_at - started_at) / 60)::int, "
          f"edited_by = '{boss}', ended_at >= '{t0}'::timestamptz FROM work_entries WHERE id = '{entry}'")
check("в базе: запись табеля закрыта моментом увольнения, минуты по нему, пометка, правил владелец; открытых у z14d нет",
      row == "t|t|закрыта увольнением|t|t|t" and sql(f"SELECT count(*) FROM work_entries WHERE user_id = '{uid(D)}' AND ended_at IS NULL") == "0", row)
crow = sql(f"SELECT user_id = '{uid(D)}', skipped, counted_grosz IS NULL, date = '{TODAY}', opening_grosz IS NOT NULL "
           f"FROM shift_closeouts WHERE id = '{co.get('id') or '00000000-0000-0000-0000-000000000000'}'")
check("в базе: сдача — от z14d, без пересчёта, днём смены, точкой ящика (с остатком на начало)", crow == "t|t|t|t|t", crow)
ts = api("GET", f"/admin/work?from={TODAY}&to={TODAY}", None, "boss")[1]
te = next((e for e in ts.get("entries", []) if e.get("id") == entry), {})
check("табель: запись не открыта, «закрыта увольнением», у неё сдача без пересчёта",
      te.get("open") is False and te.get("auto_closed") is True and te.get("note") == "закрыта увольнением"
      and (te.get("closeout") or {}).get("skipped") is True, J(te, 300))
lines = owner_lines(seen)
want = f"🧮 {D} уволен: смена с {since} закрыта увольнением — "
check("владельцу в канал — строка «z14d уволен: смена с ЧЧ:ММ закрыта увольнением … касса не сдана, без пересчёта: в ящике должно быть …»",
      len(lines) == 1 and lines[0].startswith(want) and "в табеле на проверку; касса не сдана, без пересчёта: в ящике должно быть "
      f"{co.get('expected_pln', 0):.2f} zł — пересчитай" in lines[0], J(lines))
check("админу эта строка не видна", not any(m.get("text", "").startswith(f"🧮 {D} уволен") for m in chat("admin1")))
au = {}
for x in audit(limit=30):  # только строки этого увольнения
    if x.get("nickname") == D and x.get("id") not in seen_a:
        au.setdefault(x.get("kind"), x)
sd, sc = au.get("staff_dismiss") or {}, au.get("shift_close") or {}
check("журнал: «уволен … · z14 проверка, смена с ЧЧ:ММ закрыта (… — на проверку), касса без пересчёта, снят с 1 будущих смен»",
      f"z14 проверка, смена с {since} закрыта (" in sd.get("text", "") and "— на проверку), касса без пересчёта, снят с 1 будущих смен" in sd.get("text", ""),
      J(sd, 300))
check("журнал: сдача кассы — у уволенного, от владельца: «… · без пересчёта · закрыта увольнением»",
      sc.get("admin") == "boss" and sc.get("text", "").endswith(" · без пересчёта · закрыта увольнением"), J(sc, 300))
check("админу журнал этого не показывает", not [x for x in audit("admin1", limit=50) if x.get("nickname") == D and x.get("kind") in ("staff_dismiss", "shift_close")])

section("без открытой смены — как раньше")
n_co = sql(f"SELECT count(*) FROM shift_closeouts WHERE user_id = '{uid(N)}'")
s, pv = preview(N)
check("шторка: открытой смены нет, кассы в ней нет", s == 200 and pv.get("open_shift") is None and "cash" not in pv, J(pv, 300))
s, d = api("POST", f"/admin/staff/{uid(N)}/dismiss", {}, "boss")
check("«Уволить» — 200 без «смена закрыта» и без сдачи; новых сдач у z14n нет",
      s == 200 and "closed_shift" not in d and "closeout" not in d and sql(f"SELECT count(*) FROM shift_closeouts WHERE user_id = '{uid(N)}'") == n_co,
      f"{s} {J(d, 300)}")

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
co = d.get("closeout") or {}
check("закрыта концом смены по графику, часы по нему, пометка; сдача без пересчёта — днём той смены",
      s == 200 and row == f"t|{mins}|t|закрыта увольнением" and co.get("skipped") is True and co.get("date") == yday, f"{row} {J(co, 200)}")
lines = owner_lines(seen)
dd = yday[8:10] + "." + yday[5:7]
check(f"в канал — с датой: «смена с {dd} {start_hm} закрыта увольнением — {mins // 60} ч {mins % 60:02d} м в табеле на проверку»",
      len(lines) == 1 and lines[0].startswith(f"🧮 {S} уволен: смена с {dd} {start_hm} закрыта увольнением — {mins // 60} ч {mins % 60:02d} м в табеле на проверку"),
      J(lines))
done()
