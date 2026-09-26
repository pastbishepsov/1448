#!/usr/bin/env python3
"""З7-и5: встречные брони одного ПК в одну и ту же долю секунды. Ограничение bookings_no_overlap (миграция
061) сверяет пересечение уже после записи, поэтому две встречные записи могут ждать друг друга — тогда база
снимает одну из них ошибкой deadlock (40P01), а не «пересечением» (23P01). Раньше на это гость и админ
получали «ошибку сервера» (500) — у полного прогона раз в несколько запусков так падала «гонка 6 броней»
e2e_booking_rules; теперь тот же отказ «этот ПК только что забронировали» (409 slot_busy), что и в обычной
гонке. Столкновение здесь — без расчёта на удачу: встречную бронь держит открытая транзакция psql. Одна её
запись — слева от слота; когда запрос сервера встал ждать её, вторая — справа, пересекается только с ним:
сервер ждёт psql, psql ждёт сервер, и через deadlock_timeout база снимает запрос сервера (он ждёт дольше).
Три пути записи: walk-in от стойки, бронь гостя из PWA, возврат отменённой брони. Гость — Guest50, ПК — PC-07,
слоты — через 20 дней. Стенд не меняется: транзакция psql откатывается, брони Guest50 снимаются, «бронь с
уровня» (booking_min_level) на время проверки — всем и возвращается как была."""
import datetime as dt, os, subprocess, threading, time
from _z import *

PC, NICK = "PC-07", "Guest50"
RACE_MSG = "Этот ПК только что забронировали на это время — выбери другой ПК или время"  # slotBusyRaceMessage


def clean():
    sql(f"UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id='{gid}'")
gid = uid(NICK)
clean()
at_end(clean)

pc = pcs()[PC]["id"]
club = sql(f"SELECT club_id FROM computers WHERE id='{pc}'")
s, d = login(NICK)  # вход PWA, без client=admin
guest = d.get("access_token")
check(f"гость {NICK} вошёл из PWA", s == 200 and guest, f"{s} {d.get('code', '')}")
dl = sql("SHOW deadlock_timeout")
dl_sec = float(dl[:-2]) / 1000 if dl.endswith("ms") else float(dl.rstrip("s") or 1)
# слоты — через 20 дней, по часу, в UTC: ни с кем из стенда не пересекаются
t0 = dt.datetime.now(dt.timezone.utc).replace(minute=0, second=0, microsecond=0) + dt.timedelta(days=20)
iso = lambda t: t.strftime("%Y-%m-%dT%H:%M:%SZ")
live = lambda t: sql(f"SELECT count(*) FROM bookings WHERE computer_id='{pc}' AND status IN ('pending','confirmed') "
                     f"AND start_time > '{iso(t - dt.timedelta(hours=2))}' AND start_time < '{iso(t + dt.timedelta(hours=2))}'")
log_40p01 = lambda: open(SRV_LOG, errors="replace").read().count("40P01") if SRV_LOG and os.path.exists(SRV_LOG) else None


class Rival:
    """Встречная бронь: открытая транзакция psql пишет в bookings в обход сервера и держит записи."""

    def __init__(self):
        self.p = subprocess.Popen(["psql", DB, "-qtA"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, text=True)
        self.send("BEGIN;")

    def send(self, q):
        self.p.stdin.write(q + "\n")
        self.p.stdin.flush()

    def book(self, start):
        self.send("INSERT INTO bookings (user_id, computer_id, club_id, status, start_time, duration_min, prepaid) "
                  f"VALUES ('{gid}', '{pc}', '{club}', 'confirmed', '{iso(start)}', 60, false);")

    def mark(self):
        self.send("SELECT 'mark';")
        return self.p.stdout.readline().strip() == "mark"

    def rollback(self):
        self.send("ROLLBACK;")
        self.p.stdin.close()
        err = self.p.stderr.read()
        self.p.wait(10)
        return err.strip()


def server_waits(verb):
    """Запрос сервера встал ждать чужую транзакцию — на своей записи в bookings (verb: INSERT / UPDATE)."""
    for _ in range(100):
        if sql("SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND pid <> pg_backend_pid() "
               f"AND query LIKE '{verb} %\"bookings\"%'") != "0":
            return True
        time.sleep(0.05)
    return False


def collide(slot, verb, call):
    """Слот сервера [slot, +60 мин). Встречная бронь: [−30, +30) до запроса и [+30, +90) — когда он ждёт."""
    rival = Rival()
    rival.book(slot - dt.timedelta(minutes=30))
    ready = rival.mark()
    n0, out = log_40p01(), {}
    th = threading.Thread(target=lambda: out.update(r=call()))
    start = time.time()
    th.start()
    waits = server_waits(verb)
    rival.book(slot + dt.timedelta(minutes=30))  # теперь psql ждёт сервер — круг замкнулся
    th.join(30)
    secs = time.time() - start
    err = rival.rollback()
    n1 = log_40p01()
    s, d = out.get("r", (0, {}))
    detail = f"{s} {d.get('code')} «{d.get('message')}» за {secs:.1f} с"
    check(f"встречная бронь psql поставлена, запрос сервера ждёт её на записи ({verb})", ready and waits)
    check(f"база сняла запрос сервера (deadlock): ответ не раньше deadlock_timeout {dl}, psql дописал свою бронь",
          secs >= dl_sec * 0.9 and "deadlock" not in err, f"{secs:.1f} с; psql: {err or '—'}")
    if n0 is not None:
        check("в логе сервера — deadlock (40P01) на этом запросе", n1 > n0, f"строк с 40P01: {n0}→{n1}")
    return s, d, detail


section("walk-in от стойки: POST /admin/bookings")
slot = t0
s, d, detail = collide(slot, "INSERT", lambda: api("POST", "/admin/bookings", {
    "guest": NICK, "computer_id": pc, "start_time": iso(slot), "duration_min": 60}, "admin1"))
check("отказ человеческий: 409 slot_busy «этот ПК только что забронировали» — не 500",
      s == 409 and d.get("code") == "slot_busy" and d.get("message") == RACE_MSG, detail)
check("после столкновения ни одной живой брони на слоте: psql откатился, сервер не записал", live(slot) == "0", live(slot))
s, d = api("POST", "/admin/bookings", {"guest": NICK, "computer_id": pc, "start_time": iso(slot), "duration_min": 60}, "admin1")
check("слот свободен — та же бронь сразу после → 201 (соединения сервера живы)", s == 201, f"{s} {J(d)}")
clean()

section("бронь гостя из PWA: POST /clubs/:id/bookings")
set_settings(booking_min_level=0)  # бронь из PWA — с 3-го уровня, у гостей трека З первый
slot = t0 + dt.timedelta(hours=4)
s, d, detail = collide(slot, "INSERT", lambda: api("POST", f"/clubs/{club}/bookings", {
    "computer_id": pc, "start_time": iso(slot), "duration_min": 60}, token=guest))
check("гостю — 409 slot_busy «этот ПК только что забронировали», не «ошибка сервера»",
      s == 409 and d.get("code") == "slot_busy" and d.get("message") == RACE_MSG, detail)
check("живых броней на слоте нет", live(slot) == "0", live(slot))

section("возврат отменённой брони: POST /admin/bookings/:id/restore")
slot = t0 + dt.timedelta(hours=8)
s, d = api("POST", "/admin/bookings", {"guest": NICK, "computer_id": pc, "start_time": iso(slot), "duration_min": 60}, "admin1")
bid = d.get("booking_id")
s2, _ = api("POST", f"/admin/bookings/{bid}/cancel", {}, "admin1")
check(f"бронь {NICK} создана и отменена (её и будем возвращать)", s == 201 and s2 == 200, f"{s} {s2}")
s, d, detail = collide(slot, "UPDATE", lambda: api("POST", f"/admin/bookings/{bid}/restore", {}, "admin1"))
check("возврат в столкновении → 409 slot_busy «время уже занято», не 500", s == 409 and d.get("code") == "slot_busy", detail)
check("бронь осталась отменённой, живых на слоте нет",
      sql(f"SELECT status FROM bookings WHERE id='{bid}'") == "cancelled" and live(slot) == "0", live(slot))

done()
