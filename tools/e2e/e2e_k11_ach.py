#!/usr/bin/env python3
"""К11-и5 «Ачивка „3 победы подряд“ выключена» (KIOSK.md, этап VII; миграция 102).
win_streak_3 сервер не считает (conditionValue: тип win_streak не поддержан) — гость видел её с прогрессом 0.
Проверяем: определение на месте и выключено; GET /me/achievements гостя её не отдаёт, а все включённые — отдаёт.
На HEAD (без 102) падают пункты 1, 1а и 2. Только чтение: стенд не меняет."""
from _z import *

G = "Guest46"
section("определение")
st = rows("SELECT is_active::text FROM achievements WHERE id = 'win_streak_3'")
check("1. win_streak_3 на месте и выключена", st == ["false"], J(st))
active = set(rows("SELECT id FROM achievements WHERE is_active"))
check("1а. остальные ачивки включены (выключена ровно одна)",
      rows("SELECT string_agg(id, ',') FROM achievements WHERE NOT is_active") == ["win_streak_3"])

section("гость")
s, d = api("GET", "/me/achievements", who=G)
got = [a.get("id") for a in (d or {}).get("achievements", [])] if s == 200 else []
check("0. /me/achievements отвечает 200", s == 200, s)
check("2. гость не видит «3 победы подряд»",
      "win_streak_3" not in got and not any("3 победы" in (a.get("title") or "") for a in d.get("achievements", [])), J(got))
check("3. остальные на месте: гость видит все включённые, по разу", sorted(got) == sorted(active) and len(got) > 0,
      f"{len(got)} из {len(active)}")
done()
