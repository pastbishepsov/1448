#!/usr/bin/env python3
"""З18-и3 «Картинки» (№35, №36; ETag из №40). Ревью 23.09: 30 параллельных GET обложки из базы с одного адреса — треть
429 (статика /covers не режется), а за одним NAT-адресом клуба 10 запросов в секунду делят все ПК и стойка; каждый
список каталога и товаров тянул из Postgres байты всех картинок (SELECT * GORM, BYTEA в той же таблице); ETag и
версия адреса — в секундах: картинка, перезалитая в ту же секунду, оставалась старой. Теперь: GET обложек и фото —
вне лимитера (остальное под ним), по адресу с версией — кэш на сутки, повтор с ETag — 304 без байтов, версия и ETag —
в миллисекундах; списки читают модели без байтов картинок — с 30 обложками по 400 КБ список не дольше, чем без них.
Временные записи каталога z18img_01…30 и позиция «z18img Фото» удаляются в начале и в конце."""
import http.client, os, statistics, threading, time, urllib.error, urllib.request
from _env import HOST, PORT
from _z import *

PREFIX = API[len(BASE):]  # "/api/v1"
IDS = [f"z18img_{i:02d}" for i in range(1, 31)]
GOOD = "z18img Фото"


def tidy():
    sql("DELETE FROM catalog_apps WHERE id LIKE 'z18img\\_%'")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{GOOD}'"):
        drop_good(gid)


tidy()
at_end(tidy)


def raw(method, path, body=None, ctype=None, who=None, headers=None):
    """Запрос мимо urllib: (статус, заголовки, байты)."""
    conn = http.client.HTTPConnection(HOST, PORT, timeout=30)
    hdr = dict(headers or {})
    if ctype:
        hdr["Content-Type"] = ctype
    if who:
        hdr["Authorization"] = "Bearer " + tok(who)
    try:
        conn.request(method, path if path.startswith("/api/") else PREFIX + path, body=body, headers=hdr)
        r = conn.getresponse()
        return r.status, {k.lower(): v for k, v in r.getheaders()}, r.read()
    finally:
        conn.close()
        time.sleep(0.12)


def burst(paths):
    """Разом, с одного адреса: {статус: сколько} и размеры ответов 200."""
    out, sizes, lock = {}, [], threading.Lock()

    def one(p):
        try:
            with urllib.request.urlopen(BASE + p) as r:
                st, n = r.status, len(r.read())
        except urllib.error.HTTPError as e:
            st, n = e.code, 0
        with lock:
            out[st] = out.get(st, 0) + 1
            if st == 200:
                sizes.append(n)
    th = [threading.Thread(target=one, args=(p,)) for p in paths]
    [x.start() for x in th]
    [x.join() for x in th]
    time.sleep(2.5)  # ведро лимитера — снова полное
    return out, sizes


def median_ms(path, n=9):
    out = []
    for _ in range(n):
        t = time.perf_counter()
        with urllib.request.urlopen(API + path) as r:
            r.read()
        out.append((time.perf_counter() - t) * 1000)
        time.sleep(0.15)
    return statistics.median(out)


ms_of = lambda col, table, cond: sql(f"SELECT floor(extract(epoch FROM {col}) * 1000)::bigint FROM {table} WHERE {cond}")
cover_of = lambda i: next((x.get("cover") for x in api("GET", "/catalog")[1].get("games", []) if x["id"] == i), None)

section("заготовка: 30 записей каталога, позиция с фото")
for i, a in enumerate(IDS):
    s, d = api("POST", "/admin/catalog", {"id": a, "name": f"Z18 Img {i + 1}", "category": "game", "sort": 950 + i,
                                          "target": "steam://rungameid/1"}, "boss")
    if s != 200:
        raise SystemExit(f"запись {a}: {s} {J(d)}")
s, d = api("POST", "/admin/goods", {"name": GOOD, "category": "z18", "price_pln": 4}, "boss")
GID = (d.get("good") or {}).get("id")
time.sleep(2.5)
before = median_ms("/catalog")
COVER = os.urandom(400 << 10)
puts = [raw("PUT", f"/admin/catalog/{a}/cover", COVER, "image/jpeg", "boss")[0] for a in IDS]
sp, _, _ = raw("PUT", f"/admin/goods/{GID}/photo", os.urandom(300 << 10), "image/jpeg", "boss")
check("30 обложек по 400 КБ (12 МБ в базе) и фото позиции 300 КБ", puts == [200] * 30 and sp == 200
      and int(sql("SELECT sum(octet_length(cover)) FROM catalog_apps WHERE id LIKE 'z18img\\_%'")) == 30 * (400 << 10),
      f"{set(puts)} {sp}")

section("обложки и фото — вне лимитера, остальное — под ним")
time.sleep(2.5)
covers = [cover_of(a) for a in IDS]
st, sizes = burst(covers)
check("30 параллельных GET обложек с одного адреса — 30 × 200, по 400 КБ, ни одного 429", st == {200: 30}
      and sizes == [400 << 10] * 30, J(st))
photo = next((g.get("photo") for g in api("GET", "/admin/goods", None, "boss")[1].get("goods", []) if g["id"] == GID), "")
st2, _ = burst([photo] * 30)
check("30 параллельных GET фото позиции — 30 × 200", st2 == {200: 30}, J(st2))
st3, _ = burst(["/api/v1/catalog"] * 30)
check("лимитер на месте: 30 параллельных GET /catalog — часть 429", st3.get(429, 0) > 0 and st3.get(200, 0) >= 20, J(st3))

section("кэш и версия — в миллисекундах")
v = ms_of("cover_at", "catalog_apps", "id = 'z18img_01'")
s, h, b = raw("GET", f"/catalog/z18img_01/cover?v={v}")
check("адрес обложки — ?v= момента загрузки в мс; по нему — кэш на сутки, ETag \"c<мс>\"",
      covers[0] == f"/api/v1/catalog/z18img_01/cover?v={v}" and len(v) >= 13 and s == 200
      and h.get("cache-control") == "public, max-age=86400, immutable" and h.get("etag") == f'"c{v}"',
      f"{covers[0]} · {s} {h.get('cache-control')} {h.get('etag')}")
s1, h1, _ = raw("GET", "/catalog/z18img_01/cover")
s2, h2, _ = raw("GET", f"/catalog/z18img_01/cover?v={v[:-3]}")
s3, h3, b3 = raw("GET", f"/catalog/z18img_01/cover?v={v}", headers={"If-None-Match": f'"c{v}"'})
check("без версии и со старой (в секундах) — 5 минут; повтор с ETag — 304 без тела",
      (s1, s2) == (200, 200) and h1.get("cache-control") == h2.get("cache-control") == "public, max-age=300"
      and s3 == 304 and b3 == b"", f"{h1.get('cache-control')} · {h2.get('cache-control')} · {s3} {len(b3)}")
sql("UPDATE catalog_apps SET cover_at = '2026-11-06 12:00:00.100+00' WHERE id = 'z18img_01'")
u1 = cover_of("z18img_01")
_, ha, _ = raw("GET", u1)
sql("UPDATE catalog_apps SET cover = decode('ffd8ff', 'hex') || cover, cover_at = '2026-11-06 12:00:00.900+00' "
    "WHERE id = 'z18img_01'")
u2 = cover_of("z18img_01")
s4, hb, b4 = raw("GET", u2, headers={"If-None-Match": ha.get("etag", "")})
check("перезалита в ту же секунду (0,100 → 0,900): новый адрес и ETag, старый ETag — 200 с новыми байтами",
      u1 != u2 and u2.endswith("?v=1793966400900") and ha.get("etag") != hb.get("etag") and s4 == 200
      and len(b4) == (400 << 10) + 3, f"{u1} → {u2} · {ha.get('etag')} → {hb.get('etag')} · {s4} {len(b4)}")
gv = ms_of("photo_at", "goods", f"id = '{GID}'")
s5, h5, _ = raw("GET", f"{photo}")
s6, _, b6 = raw("GET", f"{photo}", headers={"If-None-Match": f'"g{gv}"'})
check("фото позиции — так же: ?v= в мс, сутки, ETag \"g<мс>\", 304",
      photo == f"/api/v1/goods/{GID}/photo?v={gv}" and s5 == 200 and h5.get("cache-control") == "public, max-age=86400, immutable"
      and h5.get("etag") == f'"g{gv}"' and s6 == 304 and b6 == b"", f"{photo} · {s5} {h5.get('cache-control')} {h5.get('etag')} · {s6}")

section("списки — без байтов картинок")
after = median_ms("/catalog")
check(f"GET /catalog с 30 обложками по 400 КБ — {after:.1f} мс, без них — {before:.1f} мс: не дольше, чем вдвое + 5 мс",
      after <= 2 * before + 5, f"{before:.1f} → {after:.1f} мс")
s, d = api("GET", "/me/menu", None, "Guest48")
mine = [x for x in d.get("menu", []) if x.get("id") == GID]
check("в меню гостя у позиции — тот же адрес фото с версией в мс", s == 200 and [x.get("photo") for x in mine] == [photo]
      and photo.endswith("?v=" + gv),
      f"{s} {J(mine)}")
done()
