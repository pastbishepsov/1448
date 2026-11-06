#!/usr/bin/env python3
"""З18-и2 «Каталог: дифф команд» (№33 «а»; из №40 — корень игр и лимит импорта). target и args — команда, которую агент
выполняет на каждом ПК зала, cover_url — что видит гость. Ревью 23.09: превью импорта — «cs2 → target» без значений,
журнал — счётчики (импорт) и id (форма), старого значения нигде нет; корень игр из файла экспорта импорт молча
выбрасывал; «до 4 МБ» в коде и в доке при общем потолке 1 МБ, а тело больше 1 МБ без заявленного размера — «Пустое тело
запроса». Теперь: в плане импорта у записи — «было → стало» по target, args, cover_url (у новой — что будет), путь с
буквой диска или сетевой помечен (disk), «команды запуска меняются у N»; в журнал — строка на запись с «было → стало»
(и у правки формой); корень игр из файла — в плане, применяется по галочке (apply_games_root) в той же транзакции, с
неверным — 400 и ничего не сохранено; тело больше 1 МБ — 413 «Файл импорта — до 1 МБ». Временные записи z18cat_*
удаляются в начале и в конце, корень игр возвращается как был (SQL-ом, без строки в журнале)."""
import http.client, json
from _env import HOST, PORT
from _z import *

PREFIX = API[len(BASE):]  # "/api/v1"
IDS = ("z18cat_a", "z18cat_b", "z18cat_c", "z18cat_u", "z18cat_v", "z18cat_d", "z18cat_pad")
ROOT0 = rows("SELECT 'v:' || value FROM settings WHERE key = 'games_root'")  # [] — строки нет, ['v:'] — пустая


def tidy():
    sql("DELETE FROM catalog_apps WHERE id IN (" + ", ".join(f"'{i}'" for i in IDS) + ")")


def restore_root():
    sql("DELETE FROM settings WHERE key = 'games_root'")
    if ROOT0:
        sql("INSERT INTO settings (key, value, updated_at) VALUES ('games_root', '" + ROOT0[0][2:].replace("'", "''") + "', now())")


tidy()
at_end(tidy)
at_end(restore_root)
sql("UPDATE settings SET value = '' WHERE key = 'games_root'")
if not ROOT0:
    sql("INSERT INTO settings (key, value, updated_at) VALUES ('games_root', '', now())")

app = lambda i: sql(f"SELECT coalesce(target, '∅') || '|' || coalesce(args::text, '∅') || '|' || coalesce(cover_url, '∅') "
                    f"FROM catalog_apps WHERE id = '{i}'")
root = lambda: sql("SELECT value FROM settings WHERE key = 'games_root'")
mark = lambda: sql("SELECT clock_timestamp()")
since = lambda action, t: rows(f"SELECT details FROM admin_actions WHERE action = '{action}' AND created_at > '{t}' "
                               "ORDER BY created_at")
item = lambda d, i: next((x for x in d.get("items", []) if x.get("id") == i), {})
imp = lambda body: api("POST", "/admin/catalog/import", body, "boss")
G = {"name": "Z18", "category": "game"}


def raw(body, chunked=False):
    """POST импорта мимо urllib: (статус, JSON) или (None, {"_error"}), если сервер оборвал приём тела."""
    conn = http.client.HTTPConnection(HOST, PORT, timeout=20)
    hdr = {"Content-Type": "application/json", "Authorization": "Bearer " + tok("boss")}
    try:
        if chunked:
            hdr["Transfer-Encoding"] = "chunked"
            body = (lambda b: (b[i:i + 65536] for i in range(0, len(b), 65536)))(body)
        conn.request("POST", PREFIX + "/admin/catalog/import", body=body, headers=hdr, encode_chunked=chunked)
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


section("форма: «было → стало» в журнале")
t = mark()
s1, _ = api("POST", "/admin/catalog", {"id": "z18cat_a", **G, "target": r"{GAMES}\Z18\a.exe", "args": '["-novid"]'}, "boss")
s2, _ = api("POST", "/admin/catalog", {"id": "z18cat_a", **G, "target": r"C:\Z18\a.exe"}, "boss")
s3, _ = api("POST", "/admin/catalog", {"id": "z18cat_a", **G, "target": r"C:\Z18\a.exe"}, "boss")
lines = since("catalog_upsert", t)
check("новая — что задано; правка — «было → стало» (не присланные args — очищены); та же запись — только id",
      (s1, s2, s3) == (200, 200, 200) and lines == [
          r'z18cat_a (новая): target «{GAMES}\Z18\a.exe» · args «["-novid"]»',
          r'z18cat_a: target «{GAMES}\Z18\a.exe» → «C:\Z18\a.exe» · args «["-novid"]» → пусто',
          "z18cat_a"] and app("z18cat_a") == r"C:\Z18\a.exe|∅|∅", f"{s1} {s2} {s3} · {J(lines, 600)} · {app('z18cat_a')}")

section("импорт: план с «было → стало»")
api("POST", "/admin/catalog", {"id": "z18cat_u", **G, "target": "steam://rungameid/2"}, "boss")
api("POST", "/admin/catalog", {"id": "z18cat_v", **G, "target": "steam://rungameid/3"}, "boss")
APPS = [
    {"id": "z18cat_a", "target": r"{GAMES}\Z18\a.exe", "args": ["-x"], "cover_url": "/covers/z18.jpg"},
    {"id": "z18cat_b", **G, "target": r"\\nas\games\b.exe"},
    {"id": "z18cat_c", **G, "target": "steam://rungameid/1", "args": ["--dir=E:\\games"]},
    {"id": "z18cat_u", "cover_url": "/covers/u.jpg"},
    {"id": "z18cat_v", "target": "steam://rungameid/3"},
]
s, d = imp({"apps": APPS, "dry_run": True})
check("план: 5 записей — 2 новых, 2 обновится, 1 без изменений; команды запуска меняются у 1 (у z18cat_u — только обложка)",
      s == 200 and (d.get("created"), d.get("updated"), d.get("unchanged"), d.get("commands")) == (2, 2, 1, 1), f"{s} {J(d, 500)}")
check("z18cat_a — «было → стало» по target, args и cover_url; {GAMES} — не путь на диске",
      item(d, "z18cat_a").get("diff") == [
          {"field": "target", "old": r"C:\Z18\a.exe", "new": r"{GAMES}\Z18\a.exe"},
          {"field": "args", "old": "", "new": '["-x"]'},
          {"field": "cover_url", "old": "", "new": "/covers/z18.jpg"}], J(item(d, "z18cat_a"), 500))
check(r"новые — что будет: \\nas\games\b.exe и --dir=E:\games в аргументах — путь на диске, steam:// — нет",
      item(d, "z18cat_b").get("diff") == [{"field": "target", "old": "", "new": r"\\nas\games\b.exe", "disk": True}]
      and item(d, "z18cat_c").get("diff") == [{"field": "target", "old": "", "new": "steam://rungameid/1"},
                                               {"field": "args", "old": "", "new": '["--dir=E:\\\\games"]', "disk": True}],
      J([item(d, "z18cat_b"), item(d, "z18cat_c")], 600))
check("только обложка — строка cover_url; без изменений — без diff; корня в запросе нет — нет и games_root",
      item(d, "z18cat_u").get("diff") == [{"field": "cover_url", "old": "", "new": "/covers/u.jpg"}]
      and "diff" not in item(d, "z18cat_v") and item(d, "z18cat_v").get("changes") == [] and "games_root" not in d,
      J([item(d, "z18cat_u"), item(d, "z18cat_v")], 500))
check("превью ничего не пишет", app("z18cat_a") == r"C:\Z18\a.exe|∅|∅" and app("z18cat_b") == "", f"{app('z18cat_a')} · {app('z18cat_b')}")

t = mark()
s, d = imp({"apps": APPS, "dry_run": False})
lines = since("catalog_import", t)
check("запись: 200, в базе новые команды",
      s == 200 and d.get("commands") == 1 and app("z18cat_a") == r'{GAMES}\Z18\a.exe|["-x"]|/covers/z18.jpg'
      and app("z18cat_b") == r"\\nas\games\b.exe|∅|∅", f"{s} {J(d, 300)} · {app('z18cat_a')} · {app('z18cat_b')}")
check("журнал: строка на обновлённую запись с «было → стало» и итог «команды запуска изменены у 1»", lines == [
    r'z18cat_a: target «C:\Z18\a.exe» → «{GAMES}\Z18\a.exe» · args пусто → «["-x"]» · cover_url пусто → «/covers/z18.jpg»',
    "z18cat_u: cover_url пусто → «/covers/u.jpg»",
    "импорт каталога: 5 записей (+2 новых, 2 обновлено, 1 без изменений); команды запуска изменены у 1"], J(lines, 700))
t = mark()
s, d = imp({"apps": APPS, "dry_run": False})
lines = since("catalog_import", t)
check("тот же файл ещё раз — всё без изменений, ни одного diff, в журнале — только итог",
      s == 200 and d.get("unchanged") == 5 and d.get("commands") == 0 and not any("diff" in x for x in d.get("items", []))
      and lines == ["импорт каталога: 5 записей (+0 новых, 0 обновлено, 5 без изменений)"], f"{s} {J(lines)}")

section("корень игр из файла")
ONE = [{"id": "z18cat_d", **G, "target": r"{GAMES}\D\d.exe"}]
s1, d1 = imp({"apps": ONE, "games_root": r" E:\z18games ", "dry_run": True})
s2, d2 = imp({"apps": ONE, "games_root": "games", "dry_run": True})
s3, d3 = imp({"apps": ONE, "games_root": "  ", "dry_run": True})
check(r"в плане: E:\z18games — отличается от пустого корня клуба, верный; «games» — неверный с текстом; пробелы — как нет",
      (s1, s2, s3) == (200, 200, 200) and d1.get("games_root") == {"file": r"E:\z18games", "current": "", "differs": True, "ok": True}
      and d2.get("games_root", {}).get("ok") is False and d2["games_root"].get("message", "").startswith("Корень игр:")
      and "games_root" not in d3 and root() == "", f"{J(d1.get('games_root'))} · {J(d2.get('games_root'))} · {J(d3.get('games_root'))}")
s, d = imp({"apps": ONE, "games_root": r"E:\z18games", "dry_run": False})
check("без галочки — записи сохранены, корень не тронут", s == 200 and app("z18cat_d") != "" and root() == "", f"{s} · {root()!r}")
sql("DELETE FROM catalog_apps WHERE id = 'z18cat_d'")
bad = [imp({"apps": ONE, "games_root": "games", "apply_games_root": True, "dry_run": False}),
       imp({"apps": ONE, "apply_games_root": True, "dry_run": False})]
check("галочка с неверным корнем или без корня — 400 bad_games_root «… — ничего не сохранено»; записи нет, корень пуст",
      [(x[0], x[1].get("code")) for x in bad] == [(400, "bad_games_root")] * 2
      and bad[0][1].get("message", "").endswith("— ничего не сохранено")
      and bad[1][1].get("message") == "Корня игр в файле нет — ничего не сохранено" and app("z18cat_d") == "" and root() == "",
      J([x[1] for x in bad], 400))
t = mark()
s, d = imp({"apps": ONE, "games_root": r"E:\z18games", "apply_games_root": True, "dry_run": False})
cfg = since("catalog_config", t)
check(r"галочка с верным — корень E:\z18games вместе с записями, applied; в журнале «корень игр» — откуда",
      s == 200 and d.get("games_root", {}).get("applied") is True and root() == r"E:\z18games" and app("z18cat_d") != ""
      and cfg == [r'games_root: "" → "E:\\z18games" (из файла импорта)'], f"{s} {J(d.get('games_root'))} · {root()!r} · {J(cfg)}")
s, d = imp({"apps": ONE, "games_root": r"E:\z18games", "dry_run": True})
check("тот же корень — differs: false (галочки в превью не будет)", d.get("games_root", {}).get("differs") is False
      and d["games_root"].get("current") == r"E:\z18games", J(d.get("games_root")))

section("честный лимит: 1 МБ, как у всего сервера")
head = json.dumps({"dry_run": True, "apps": [{"id": "z18cat_pad", **G}]})
pad = lambda n: (head[:-1] + " " * (n - len(head)) + "}").encode()
s1, d1 = raw(pad((1 << 20) - 64))
s2, d2 = raw(pad((1 << 20) - 64), chunked=True)
check("почти 1 МБ (пробелы JSON) — 200, с размером и без", (s1, s2) == (200, 200) and d1.get("total") == 1 and d2.get("total") == 1,
      f"{s1} {s2} {d1.get('code')} {d2.get('code')}")
s1, d1 = raw(pad((1 << 20) + (10 << 10)))
s2, d2 = raw(pad((1 << 20) + (10 << 10)), chunked=True)
check("1 МБ + 10 КБ: с размером — 413 body_too_large (общая мидлварь), без размера — 413 import_too_big «Файл импорта — до 1 МБ», "
      "а не «Пустое тело запроса»", s1 == 413 and d1.get("code") == "body_too_large" and s2 == 413
      and d2.get("code") == "import_too_big" and d2.get("message") == "Файл импорта — до 1 МБ", f"{s1} {d1.get('code')} · {s2} {J(d2)}")
check("сервер жив, записи z18cat_pad нет", api("GET", "/admin/catalog", None, "boss")[0] == 200 and app("z18cat_pad") == "")
done()
