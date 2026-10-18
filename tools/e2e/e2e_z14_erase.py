#!/usr/bin/env python3
"""З14-и4 «Личные данные уволенного» (№26; З-Р19). Карточка уволенного хранила ФИО, телефон, заметку и ставку вечно, журнал —
причину увольнения, и стереть их было нельзя даже вручную: карточка из «Уволенных» не сохранялась (409 not_staff). Теперь
её правят без смены роли (дата увольнения при этом остаётся — человек не уходит из архива), а владелец стирает личные
данные разом — POST /admin/staff/:id/erase: ФИО, телефон, заметка, ставка — пусто, причина в строках журнала об
увольнении и уходе — «—» (остальное в строке как было). Ник и игровой аккаунт, даты, должность, строки журнала о ставке
(деньги клуба) остаются; админу — 403, у работающего и у гостя без карточки — 409; повтор — «стирать нечего», без новой
строки журнала. Автоочистки по сроку нет. Временный сотрудник z14e уходит проверкой; денег не трогает."""
import re

from _z import *

E = "z14e"
hire_temp(E)
TODAY, TOMORROW, D3 = club_day(0), club_day(1), club_day(3)
WHY, WHY_LEAVE = "z14 опоздания, грубость", "z14 переезд"
CARD = {"full_name": "Ян Тестовый", "phone": "+48 500 000 001", "position": "кассир", "hired_at": club_day(-30),
        "rate_type": "hour", "rate_amount": 25, "note": "ключ от склада"}
s, d = api("PUT", f"/admin/staff/{uid(E)}/profile", CARD, "boss")
shift = sql("SELECT id FROM shifts WHERE active AND start_min < end_min AND days_mask = 127 ORDER BY start_min LIMIT 1")
sa = api("POST", f"/admin/shifts/{shift}/assign", {"date": TOMORROW, "nickname": E}, "boss")[0]
check("данные: у работающего z14e карточка с ФИО, телефоном, ставкой и заметкой; на завтра — смена в графике",
      s == 200 and (d.get("profile") or {}).get("full_name") == CARD["full_name"] and sa in (200, 201), f"{s} {sa}")
card = lambda: sql(f"SELECT full_name || '|' || phone || '|' || note || '|' || rate_type || '|' || rate_amount::text || '|' || position || '|' "
                   f"|| COALESCE(dismissed_at::text, '') FROM staff_profiles WHERE user_id = '{uid(E)}'")
erase = lambda who="boss", nick=E: api("POST", f"/admin/staff/{uid(nick)}/erase", {}, who)
mine = lambda who="boss": [x for x in audit(who, limit=200) if x.get("nickname") == E]

section("стирают только у уволенного и только владелец")
s1, d1 = erase()
s2, d2 = erase(nick="Guest45")
s3, _ = api("POST", "/admin/staff/00000000-0000-0000-0000-000000000000/erase", {}, "boss")
check("у работающего — 409 not_dismissed, у гостя без карточки — тоже, нет такого — 404",
      s1 == 409 and d1.get("code") == "not_dismissed" and s2 == 409 and d2.get("code") == "not_dismissed" and s3 == 404, f"{s1} {J(d1)} · {s2} · {s3}")
s, d = erase("admin1")
check("админу — 403 owner_only", s == 403 and d.get("code") == "owner_only", f"{s} {J(d)}")

section("уволенного — с причиной; его карточка правится")
api("POST", f"/admin/staff/{uid(E)}/dismiss", {"date": D3, "reason": WHY_LEAVE}, "boss")  # сначала назначили уход
s, d = api("POST", f"/admin/staff/{uid(E)}/dismiss", {"reason": WHY}, "boss")             # потом уволили сейчас
lines = {x.get("kind"): x.get("text") for x in reversed(mine())}  # у каждого вида — последняя
check("журнал: «уволен … · причина, снят с 1 будущих смен» и «уходит … · причина»",
      s == 200 and lines.get("staff_dismiss") == f"{E} · уволен {TODAY} · {WHY}, снят с 1 будущих смен"
      and lines.get("staff_leave") == f"{E} · уходит {D3} · {WHY_LEAVE}", J(lines, 600))
s, d = api("PUT", f"/admin/staff/{uid(E)}/profile", {**CARD, "phone": "+48 500 000 002"}, "boss")
check("карточка уволенного сохраняется (раньше — 409 not_staff); дата увольнения остаётся — он по-прежнему в «Уволенных»",
      s == 200 and (d.get("profile") or {}).get("phone") == "+48 500 000 002" and card().endswith(f"|{TODAY}")
      and sql(f"SELECT role FROM users WHERE id = '{uid(E)}'") == "player", f"{s} {J(d, 300)} · {card()}")

section("стереть личные данные")
seen = {x.get("id") for x in audit(limit=100)}
s, d = erase()
n = d.get("journal_lines", 0)
check("владелец — 200: стёрты ФИО, телефон, заметка, ставка; строк журнала с причиной — не меньше двух",
      s == 200 and d.get("erased") == ["ФИО", "телефон", "заметка", "ставка"] and n >= 2 and "nothing" not in d, f"{s} {J(d)}")
check("в карточке: ФИО, телефон, заметка — пусто, ставки нет; должность и дата увольнения на месте",
      card() == f"|||none|0.00|кассир|{TODAY}", card())
check("игровой аккаунт остаётся: ник тот же, вход гостем — 200", login(E)[0] == 200 and uid(E) != "")
lines = {x.get("kind"): x.get("text") for x in reversed(mine())}
check("журнал: причина — «—», остальное в строке как было", lines.get("staff_dismiss") == f"{E} · уволен {TODAY} · —, снят с 1 будущих смен"
      and lines.get("staff_leave") == f"{E} · уходит {D3} · —", J(lines, 600))
texts = J([x.get("text") for x in mine()], 20000)
check("причин увольнения в журнале про z14e больше нет нигде", "опоздания" not in texts and "переезд" not in texts, texts[:300])
check("строки о ставке остаются — это деньги клуба", "за час 25.00" in texts, texts[:300])
er = next((x for x in mine() if x.get("kind") == "staff_erase" and x.get("id") not in seen), {})
check("журнал владельца: «личные данные стёрты: ФИО, телефон, заметка, ставка; причина увольнения в журнале — «—» (строк: N)» от boss",
      er.get("text") == f"{E} · личные данные стёрты: ФИО, телефон, заметка, ставка; причина увольнения в журнале — «—» (строк: {n})"
      and er.get("admin") == "boss", J(er))
check("админу эта строка не видна", not [x for x in mine("admin1") if x.get("kind") == "staff_erase"])
arch = next((x for x in api("GET", "/admin/staff/archive", None, "boss")[1].get("staff", []) if x.get("nickname") == E), {})
check("«Уволенные»: z14e на месте, без ФИО, работал до сегодня", arch.get("full_name") == "" and arch.get("dismissed_at") == TODAY, J(arch))

section("повтор — стирать нечего")
before = len([x for x in mine() if x.get("kind") == "staff_erase"])
s, d = erase()
check("второй раз — 200 «стирать нечего», новой строки журнала нет", s == 200 and d.get("nothing") is True and d.get("erased") == []
      and d.get("journal_lines") == 0 and len([x for x in mine() if x.get("kind") == "staff_erase"]) == before, f"{s} {J(d)}")
done()
