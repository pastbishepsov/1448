#!/usr/bin/env python3
"""З3-и3 «Ручной табель — только персоналу и без пересечений» (№21; двойной «Я пришёл» из №30): владелец
ведёт табель руками только на персонал или бывший персонал с кадровой карточкой (Д12 — дописать последнюю
смену уволенного); запись на гостя — 409 not_staff (раньше 201: стенд ревью записал табель на Guest06).
Запись, которая пересекает другую запись того же человека, — 409 overlap и при добавлении, и при правке;
встык — можно. Шесть одновременных «Я пришёл» дают одну запись: 201 и 409 already_open, а не 500.
Временные z3man (в штате) и z3ex (уволен, карточка есть), гость Guest33; записи табеля удаляются."""
import socket, threading, urllib.parse
from _env import HOST, PORT
from _z import *

zid, xid = hire_temp("z3man"), hire_temp("z3ex")
clean = lambda: sql(f"DELETE FROM work_entries WHERE user_id IN ('{zid}','{xid}')")
at_end(clean)
clean()
dismiss("z3ex")  # бывший персонал: роль player, карточка с датой увольнения
D = club_day(-4)
add = lambda nick, start, end: api("POST", "/admin/work", {"nickname": nick, "date": D, "start": start, "end": end}, "boss")
g33 = uid("Guest33")

section(f"кому можно вести табель руками ({D})")
before = sql(f"SELECT count(*) FROM work_entries WHERE user_id='{g33}'")
s, d = add("Guest33", "10:00", "12:00")
check("запись на гостя → 409 not_staff", s == 409 and d.get("code") == "not_staff", f"{s} {J(d)}")
check("… и в табеле гостя ничего не появилось", sql(f"SELECT count(*) FROM work_entries WHERE user_id='{g33}'") == before)
s, d = add("z3ex", "10:00", "12:00")
check("бывшему сотруднику с карточкой (Д12) → 201", s == 201, f"{s} {J(d)}")
s, d = add("z3man", "09:00", "13:00")
check("сотруднику в штате 09–13 → 201", s == 201, f"{s} {J(d)}")

section("пересечения")
s, d = add("z3man", "12:00", "14:00")
check("12–14 поверх 09–13 → 409 overlap, в тексте — с какой записью", s == 409 and d.get("code") == "overlap"
      and "09:00–13:00" in d.get("message", ""), f"{s} {J(d)}")
s, d = add("z3man", "13:00", "17:00")
e2 = d.get("entry", {}).get("id")
check("13–17 встык к 09–13 → 201: граница — не пересечение", s == 201 and e2, f"{s} {J(d)}")
s, d = api("PATCH", f"/admin/work/{e2}", {"start": "08:00", "end": "10:00"}, "boss")
check("правка 13–17 на 08–10 (поверх 09–13) → 409 overlap", s == 409 and d.get("code") == "overlap", f"{s} {J(d)}")
s, d = api("PATCH", f"/admin/work/{e2}", {"start": "13:30", "end": "17:00"}, "boss")
check("правка той же записи без пересечения → 200: сама с собой запись не пересекается", s == 200, f"{s} {J(d)}")
s, d = add("z3man", "22:00", "02:00")
check("ночная 22–02 после дневных → 201", s == 201, f"{s} {J(d)}")
s, d = api("PATCH", f"/admin/work/{e2}", {"start": "13:30", "end": "17:00", "nickname": "Guest33"}, "boss")
check("правка чужой записи ником в теле цель не меняет → 200", s == 200 and sql(f"SELECT user_id FROM work_entries WHERE id='{e2}'") == zid, f"{s}")

section("шесть одновременных «Я пришёл»")
def burst(token, n=6):
    """n запросов «Я пришёл» разом: соединения открыты заранее, байты уходят по общему сигналу — иначе
    запросы выстраиваются в очередь ещё на стороне клиента и гонки у сервера не бывает."""
    req = (f"POST {urllib.parse.urlparse(API).path}/admin/work/start HTTP/1.1\r\nHost: {HOST}:{PORT}\r\n"
           f"Authorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: 2\r\n"
           "Connection: close\r\n\r\n{}").encode()
    socks = [socket.create_connection((HOST, PORT), timeout=10) for _ in range(n)]
    codes, go = [0] * n, threading.Barrier(n)
    def run(i):
        go.wait()
        socks[i].sendall(req)
        head = socks[i].recv(64)
        codes[i] = int(head.split(b" ")[1]) if head.startswith(b"HTTP/1.1 ") else -1
        socks[i].close()
    threads = [threading.Thread(target=run, args=(i,)) for i in range(n)]
    [x.start() for x in threads]
    [x.join() for x in threads]
    return sorted(codes)
t = tok("z3man")  # вход в админку открывает смену — снимаем её, чтобы гонка шла с нуля
sql(f"DELETE FROM work_entries WHERE user_id='{zid}' AND ended_at IS NULL")
codes = burst(t)
check("ответы: один 201, остальные 409 already_open — без 500", codes == [201] + [409] * 5, codes)
check("открытая запись ровно одна", sql(f"SELECT count(*) FROM work_entries WHERE user_id='{zid}' AND ended_at IS NULL") == "1")
done()
