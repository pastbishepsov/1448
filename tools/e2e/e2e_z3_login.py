#!/usr/bin/env python3
"""З3-и1 «Смена — только входом в админку» (№19): смену админу открывает только вход в админку
(`client: "admin"` в теле /auth/login). Киоск и PWA входят тем же /auth/login без client — такой вход часов
не рождает: админ, зашедший из дома в PWA, больше не «на смене с 09:00». Повторный вход в админку записей
не плодит; в записи и в журнале — откуда вход. Временный сотрудник z3in; его записи табеля удаляются."""
from _z import *

zid = hire_temp("z3in")
at_end(lambda: sql(f"DELETE FROM work_entries WHERE user_id='{zid}'"))
sql(f"DELETE FROM work_entries WHERE user_id='{zid}'")  # чистый старт: повторный прогон
t0 = sql("SELECT now()")
opened = lambda: rows(f"SELECT note FROM work_entries WHERE user_id='{zid}' AND ended_at IS NULL")
total = lambda: int(sql(f"SELECT count(*) FROM work_entries WHERE user_id='{zid}'"))

section("вход из киоска и PWA — без client")
s, d = login("z3in")
check("вход без client → 200, токены выданы", s == 200 and d.get("access_token"), s)
check("смена не открылась, записей табеля нет", not opened() and total() == 0, opened())
s, d = login("z3in", client="kiosk")
check("client: kiosk → 200 и тоже без смены", s == 200 and not opened() and total() == 0, opened())

section("вход в админку — client: admin")
s, d = login("z3in", client="admin")
check("client: admin → 200, смена открыта", s == 200 and len(opened()) == 1, opened())
check("в записи — откуда вход: «открыта входом в админку»", opened() == ["открыта входом в админку"], opened())
s, d = login("z3in", client="admin")
check("повторный вход в админку новой записи не создаёт", s == 200 and len(opened()) == 1 and total() == 1, f"{opened()} всего {total()}")
login("z3in")
check("вход из PWA при открытой смене её не трогает", len(opened()) == 1 and total() == 1, f"{opened()} всего {total()}")
rows_ = rows(f"SELECT details FROM admin_actions WHERE admin_id='{zid}' AND action='work_start' AND created_at >= '{t0}'")
check("журнал: одна строка прихода z3in — «вход в админку»", len(rows_) == 1 and rows_[0].startswith("вход в админку"), J(rows_))
done()
