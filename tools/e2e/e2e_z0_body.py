#!/usr/bin/env python3
"""З0-и2 «Точный лимит тела» (№32): потолок 1 МБ — у всех ручек с телом, без исключений по префиксу.
Большой JSON в приход склада отбивается до разбора: заявленный размер больше 1 МБ — 413 body_too_large
сразу, без заявленного размера (chunked) чтение обрывается на 1 МБ; склад и журнал склада не тронуты.
Фото позиции живёт под своим лимитом 500 КБ: 400 КБ грузится, 600 КБ — photo_too_big.
Позиция «Woda 0.5»: склад не меняется, своё фото проверка в конце снимает."""
import http.client, json, os
from _env import HOST, PORT
from _z import *

PREFIX = API[len(BASE):]  # "/api/v1"
good = sql("SELECT id FROM goods WHERE name='Woda 0.5'")
state = lambda: sql(f"SELECT stock || ' ' || (SELECT count(*) FROM stock_moves WHERE good_id=g.id) FROM goods g WHERE id='{good}'")

def raw(method, path, body, who, ctype="application/json", chunked=False):
    """Запрос мимо urllib: (статус, JSON) или (None, {"_error"}), если сервер оборвал приём тела."""
    conn = http.client.HTTPConnection(HOST, PORT, timeout=20)
    hdr = {"Content-Type": ctype, "Authorization": "Bearer " + tok(who)}
    try:
        if chunked:
            hdr["Transfer-Encoding"] = "chunked"
            body = (lambda b: (b[i:i + 65536] for i in range(0, len(b), 65536)))(body)
        conn.request(method, PREFIX + path, body=body, headers=hdr, encode_chunked=chunked)
        r = conn.getresponse()
        data = r.read()
        try:
            return r.status, json.loads(data or b"{}")
        except ValueError:
            return r.status, {"_text": data[:200].decode(errors="replace")}
    except OSError as e:
        return None, {"_error": repr(e)}
    finally:
        conn.close()

section("приход склада: 3 МБ JSON с верными полями")
big = json.dumps({"delta": 1, "reason": "supply", "note": "a" * (3 << 20)}).encode()
before = state()
s, d = raw("POST", f"/admin/goods/{good}/stock", big, "admin1")
check("заявлено 3 МБ → 413 body_too_large, до разбора", s == 413 and d.get("code") == "body_too_large", f"{s} {d.get('code') or d}")
check("склад и журнал склада не тронуты", state() == before, f"{before} → {state()}")
s, d = raw("POST", f"/admin/goods/{good}/stock", big, "admin1", chunked=True)
check("те же 3 МБ без заявленного размера → чтение обрезано на 1 МБ, отказ", s is None or 400 <= s < 500, f"{s} {d.get('code') or d}")
check("… склад и журнал склада не тронуты", state() == before, f"{before} → {state()}")
s, d = raw("POST", "/admin/zones", big, "boss")
check("та же граница у любой ручки: /admin/zones → 413 body_too_large", s == 413 and d.get("code") == "body_too_large", f"{s} {d.get('code')}")
check("сервер жив", api("GET", "/admin/goods", None, "admin1")[0] == 200)

section("фото позиции — свой лимит 500 КБ")
had_photo = sql(f"SELECT photo_at IS NOT NULL FROM goods WHERE id='{good}'") == "t"
if not had_photo:
    at_end(lambda: api("DELETE", f"/admin/goods/{good}/photo", None, "boss"))
s, d = raw("PUT", f"/admin/goods/{good}/photo", os.urandom(400 << 10), "boss", ctype="image/jpeg")
check("фото 400 КБ → 200", s == 200 and d.get("photo"), f"{s} {d.get('code')}")
s, d = raw("PUT", f"/admin/goods/{good}/photo", os.urandom(600 << 10), "boss", ctype="image/jpeg")
check("фото 600 КБ → 413 photo_too_big", s == 413 and d.get("code") == "photo_too_big", f"{s} {d.get('code')}")
done()
