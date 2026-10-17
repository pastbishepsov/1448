#!/usr/bin/env python3
"""З14-и2 «Отложенное увольнение» (№25 «а»; З-Р18). «Последний рабочий день 30.12» снимал роль в ту же секунду, а в
архиве стояло «работал до 30.12». Теперь дата позже сегодняшних клубных суток — увольнение отложенное: роль и доступ
остаются до конца того дня, дата ложится в карточку (leave_on), в списке — «уходит 30.12» и «Отменить увольнение».
Со следующего дня adminMiddleware не пускает (403 staff_left) и вход в админку смену уже не открывает, а фоновый
прогон (раз в 10 минут; POST /admin/staff/leave/run — то же сейчас) завершает увольнение: роль — гость, дата
увольнения — последний день, смены графика после неё сняты, незакрытая смена закрыта по графику с уведомлением
владельцу (З14-и1а), строка журнала — от имени того, кто назначил. «Уволить» с сегодняшней датой при назначенном —
увольнение сейчас, дата ухода снимается. Временные сотрудники z14l (назначают, отменяют, назначают снова; день
проходит — SQL-ом) и z14m (назначен — увольняют сейчас); оба уходят проверкой. Денег не трогает."""
from _z import *

L, M = "z14l", "z14m"
for nick in (L, M):
    hire_temp(nick)
ids = f"'{uid(L)}', '{uid(M)}'"
reset = lambda: sql(f"UPDATE staff_profiles SET leave_on = NULL, leave_by = NULL, leave_reason = '' WHERE user_id IN ({ids})")
reset()  # хвост упавшего прогона
at_end(lambda: sql(f"DELETE FROM shift_assignments WHERE user_id IN ({ids})"))
at_end(reset)
TODAY, D3, D4, D5, yday = club_day(0), club_day(3), club_day(4), club_day(5), club_day(-1)
ru = lambda d: f"{d[8:10]}.{d[5:7]}.{d[:4]}"
day_shift = sql("SELECT id || '|' || start_min || '|' || end_min FROM shifts WHERE active AND start_min < end_min AND days_mask = 127 "
                "ORDER BY start_min LIMIT 1")
if not check("в графике есть дневная смена на все дни", bool(day_shift)):
    done()
sid, smin, emin = day_shift.split("|")
chat = lambda who="boss": api("GET", "/admin/staff-chat", None, who)[1].get("messages", [])
owner_lines = lambda seen: [m["text"] for m in chat() if m["id"] not in seen and m.get("kind") == "system" and m.get("visible_to") == "owner"]
prof = lambda nick: sql(f"SELECT leave_on, leave_by = (SELECT id FROM users WHERE nickname = 'boss'), leave_reason, dismissed_at "
                        f"FROM staff_profiles WHERE user_id = '{uid(nick)}'")
role = lambda nick: sql(f"SELECT role FROM users WHERE nickname = '{nick}'")
assign = lambda nick, day: api("POST", f"/admin/shifts/{sid}/assign", {"date": day, "nickname": nick}, "boss")[0]
listed = lambda nick: next((s for s in api("GET", "/admin/staff", None, "boss")[1].get("staff", []) if s.get("nickname") == nick), None)

def fresh_audit(seen_a, nick):
    """Строки журнала про nick, появившиеся после seen_a: у каждого вида — последняя."""
    out = {}
    for x in audit(limit=40):
        if x.get("nickname") == nick and x.get("id") not in seen_a:
            out.setdefault(x.get("kind"), x)
    return out

section("будущая дата — увольнение отложенное")
tl = tok(L)  # вход в админку открывает смену
entry = sql(f"SELECT id FROM work_entries WHERE user_id = '{uid(L)}' AND ended_at IS NULL")
check("данные: смены на 3-й и 4-й день от сегодня в графике; у z14l открыта смена (вход в админку)",
      assign(L, D3) in (200, 201) and assign(L, D4) in (200, 201) and bool(entry), entry)
seen_a = {x.get("id") for x in audit(limit=60)}
s, d = api("POST", f"/admin/staff/{uid(L)}/dismiss", {"date": D3}, "boss")
check(f"«Уволить» с последним днём {ru(D3)} — 200, deferred: уходит {D3}; после этой даты в графике — 1 смена",
      s == 200 and d.get("deferred") is True and d.get("leave_on") == D3 and d.get("shifts_after") == 1, f"{s} {J(d)}")
check("роль осталась admin, увольнения в карточке нет — только дата ухода и кто назначил (владелец)",
      role(L) == "admin" and prof(L) == f"{D3}|t||", f"{role(L)} · {prof(L)}")
s, w = api("GET", "/admin/work/me", None, token=tl)
check("сам z14l работает как раньше: админка пускает, смена открыта той же записью",
      s == 200 and sql(f"SELECT ended_at IS NULL FROM work_entries WHERE id = '{entry}'") == "t", f"{s} {J(w, 200)}")
st = listed(L) or {}
s, pf = api("GET", f"/admin/staff/{uid(L)}/profile", None, "boss")
s2, pv = api("GET", f"/admin/staff/{uid(L)}/shift", None, "boss")
check("список персонала — leave_on, карточка — leave_on, шторка увольнения — leave_on и сегодняшние клубные сутки",
      st.get("leave_on") == D3 and pf.get("profile", {}).get("leave_on") == D3
      and pv.get("leave_on") == D3 and pv.get("club_today") == TODAY, f"{J(st, 200)} · {J(pf, 300)} · {J(pv, 300)}")
s, cp = api("PUT", f"/admin/staff/{uid(L)}/profile", {"position": "кассир", "hired_at": club_day(-30)}, "boss")
check("карточку правят — дата ухода остаётся (и в ответе, и в базе)", s == 200 and cp.get("profile", {}).get("leave_on") == D3
      and prof(L) == f"{D3}|t||", f"{s} {J(cp, 300)} · {prof(L)}")
sl = fresh_audit(seen_a, L).get("staff_leave") or {}
check(f"журнал: «z14l · уходит {D3}, снимется со смен после этой даты: 1», от владельца",
      sl.get("text") == f"{L} · уходит {D3}, снимется со смен после этой даты: 1" and sl.get("admin") == "boss", J(sl, 300))
check("админу журнал этого не показывает; назначить, отменить и прогнать — только владельцу (403)",
      not [x for x in audit("admin1", limit=50) if x.get("nickname") == L and x.get("kind", "").startswith("staff_")]
      and api("DELETE", f"/admin/staff/{uid(L)}/leave", None, "admin1")[0] == 403
      and api("POST", "/admin/staff/leave/run", {}, "admin1")[0] == 403
      and api("POST", f"/admin/staff/{uid(L)}/dismiss", {"date": D5}, "admin1")[0] == 403)

section("отменить и назначить снова")
s, d = api("DELETE", f"/admin/staff/{uid(L)}/leave", None, "boss")
check(f"«Отменить увольнение» — 200, отменено {D3}; даты в карточке больше нет, в списке — тоже",
      s == 200 and d.get("cancelled") == D3 and prof(L) == "|||" and "leave_on" not in (listed(L) or {"leave_on": 1}), f"{s} {J(d)} · {prof(L)}")
s, d = api("DELETE", f"/admin/staff/{uid(L)}/leave", None, "boss")
check("второй раз — 409 not_leaving «Увольнение не назначено — отменять нечего»",
      s == 409 and d.get("code") == "not_leaving" and d.get("message") == "Увольнение не назначено — отменять нечего", f"{s} {J(d)}")
sc = fresh_audit(seen_a, L).get("staff_leave_cancel") or {}
check(f"журнал: «z14l · увольнение {D3} отменено»", sc.get("text") == f"{L} · увольнение {D3} отменено", J(sc, 300))
api("POST", f"/admin/staff/{uid(L)}/dismiss", {"date": D3}, "boss")
s, d = api("POST", f"/admin/staff/{uid(L)}/dismiss", {"date": D5, "reason": "z14 уходит сам"}, "boss")
check(f"назначить снова и передумать с датой: новая дата заменяет — уходит {D5}, причина в карточке; после неё смен нет",
      s == 200 and d.get("leave_on") == D5 and d.get("shifts_after") == 0 and prof(L) == f"{D5}|t|z14 уходит сам|" and role(L) == "admin",
      f"{s} {J(d)} · {prof(L)}")

section("последний день прошёл (SQL): доступа нет, прогон завершает увольнение")
start_hm = f"{(int(smin) + 60) // 60:02d}:{(int(smin) + 60) % 60:02d}"
end_hm = f"{int(emin) // 60:02d}:{int(emin) % 60:02d}"
mins = int(emin) - int(smin) - 60
# смена последнего дня не закрыта: ушёл, не отметившись; в графике — вчерашняя (последний день) и сегодняшняя
sql(f"UPDATE work_entries SET date = '{yday}', started_at = {at_club(yday, start_hm)}, shift_id = '{sid}' WHERE id = '{entry}'")
check("данные: вчера и сегодня — смены в графике", assign(L, yday) in (200, 201) and assign(L, TODAY) in (200, 201))
after = sql(f"SELECT count(*) FROM shift_assignments WHERE user_id = '{uid(L)}' AND date > '{yday}'")
seen, seen_a = {m["id"] for m in chat()}, {x.get("id") for x in audit(limit=60)}
sql(f"UPDATE staff_profiles SET leave_on = '{yday}' WHERE user_id = '{uid(L)}'")  # окно до прогона — доли секунды
s, d = api("GET", "/admin/work/me", None, token=tl)
raced = role(L) == "player"  # фоновый прогон (раз в 10 минут) успел раньше ручного — тогда 403 обычный
check("тот же токен — 403 staff_left «Последний рабочий день прошёл — доступа в админку больше нет»; роль ещё admin",
      s == 403 and (raced and d.get("code") == "forbidden" or d.get("code") == "staff_left"
                    and d.get("message") == "Последний рабочий день прошёл — доступа в админку больше нет"), f"{s} {J(d)} · фон успел: {raced}")
s, lg = login(L, client="admin")
opened = sql(f"SELECT string_agg(id::text, ',') FROM work_entries WHERE user_id = '{uid(L)}' AND ended_at IS NULL")
check("вход в админку смену уже не открывает: незакрытая — только вчерашняя (её закроет прогон)",
      raced or s == 200 and opened == entry, f"{s} · {opened}")
s, d = api("POST", "/admin/staff/leave/run", {}, "boss")
check("прогон — 200, завершено 1", s == 200 and (d.get("finished") == 1 or raced and d.get("finished") == 0), f"{s} {J(d)} · фон успел: {raced}")
check(f"роль — player, уволен {yday} (последний день), даты ухода, кто назначил и причины в карточке больше нет",
      role(L) == "player" and prof(L) == f"|||{yday}", f"{role(L)} · {prof(L)}")
row = sql(f"SELECT ended_at = {at_club(yday, end_hm)}, minutes, auto_closed, note, edited_by = (SELECT id FROM users WHERE nickname = 'boss') "
          f"FROM work_entries WHERE id = '{entry}'")
check(f"вчерашняя смена закрыта концом по графику ({end_hm}), {mins // 60} ч {mins % 60:02d} м на проверку, «закрыта увольнением», "
      "правил владелец", row == f"t|{mins}|t|закрыта увольнением|t", row)
kept = sql(f"SELECT string_agg(date::text, ',' ORDER BY date) FROM shift_assignments WHERE user_id = '{uid(L)}'")
check(f"в графике осталась только смена последнего дня ({yday}); снято 3 — сегодня, на 3-й и 4-й день", kept == yday and after == "3", f"{kept} · {after}")
arch = next((x for x in api("GET", "/admin/staff/archive", None, "boss")[1].get("staff", []) if x.get("nickname") == L), {})
check(f"архив: z14l, работал до {yday}; в списке персонала его нет", arch.get("dismissed_at") == yday and listed(L) is None, J(arch, 200))
dd = yday[8:10] + "." + yday[5:7]
sd = fresh_audit(seen_a, L).get("staff_dismiss") or {}
want = (f"{L} · уволен {yday} (последний рабочий день прошёл) · z14 уходит сам, смена с {dd} {start_hm} закрыта "
        f"({mins // 60} ч {mins % 60:02d} м — на проверку), касса не сдана, снят с 3 будущих смен")
check("журнал — от имени назначившего (boss): «… уволен (последний рабочий день прошёл) · причина, смена с … закрыта …, снят с 3 будущих смен»",
      sd.get("text") == want and sd.get("admin") == "boss", f"{J(sd, 400)}\n     ждали: {want}")
lines = owner_lines(seen)
check(f"владельцу в канал — что осталось: «z14l уволен, а смена с {dd} {start_hm} осталась незакрытой — … касса не сдана …; часы — "
      "смена закрыта по графику …»", len(lines) == 1 and lines[0].startswith(f"⚠ {L} уволен, а смена с {dd} {start_hm} осталась незакрытой — "
      "осталось за него: касса не сдана") and lines[0].endswith(f"; часы — смена закрыта по графику, {mins // 60} ч {mins % 60:02d} м в табеле на проверку"),
      J(lines))
s, d = api("POST", "/admin/staff/leave/run", {}, "boss")
s2, d2 = api("GET", "/admin/work/me", None, token=tl)
check("прогон ещё раз — завершено 0; токен z14l — 403 forbidden (роль снята)", s == 200 and d.get("finished") == 0
      and s2 == 403 and d2.get("code") == "forbidden", f"{s} {J(d)} · {s2} {J(d2)}")

section("назначено, но уволили сейчас — дата ухода снимается")
api("POST", f"/admin/staff/{uid(M)}/dismiss", {"date": D3}, "boss")
pending = prof(M)
s, d = api("POST", f"/admin/staff/{uid(M)}/dismiss", {}, "boss")
check(f"«Уволить» с сегодняшней датой при назначенном {ru(D3)} — 200 сразу: роль player, уволен {TODAY}, даты ухода нет",
      pending.startswith(f"{D3}|t|") and s == 200 and "deferred" not in d and d.get("dismissed_at") == TODAY and role(M) == "player"
      and prof(M) == f"|||{TODAY}", f"{pending} · {s} {J(d)} · {prof(M)}")
s, d = api("DELETE", f"/admin/staff/{uid(M)}/leave", None, "boss")
check("отменять уже нечего — 409 not_leaving", s == 409 and d.get("code") == "not_leaving", f"{s} {J(d)}")
done()
