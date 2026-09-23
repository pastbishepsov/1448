#!/usr/bin/env python3
"""З0-и1 «Роль из базы» (№3): увольнение и снятие роли действуют в ту же секунду — старый access
не пускает в /admin, live-канал рвётся кодом 4403, новый тикет не выдаётся, выданный раньше сокет не
открывает; /me работает, refresh даёт player; назначение действует без перелогина.
Временный сотрудник z0role, гость Guest31. Идемпотентен: в конце z0role — не персонал."""
from _z import *

ticket = lambda tk: api("POST", "/admin/ws-ticket", {}, token=tk)
g31, w31 = uid("Guest31"), wallet("Guest31")

section("увольнение: старый access уволенного")
zid = hire_temp("z0role")
t = tokens("z0role", fresh=True)
A, R = t["access"], t["refresh"]
check("до увольнения: /admin/users старым access → 200", api("GET", "/admin/users", token=A)[0] == 200)
sock, st = ws_admin(ticket(A)[1].get("ticket", ""))
check("live-канал z0role открыт (101)", st == 101, st)
early = ticket(A)[1].get("ticket", "")
s, d = dismiss("z0role")
check("увольнение → 200", s == 200, J(d))
s, d = api("GET", "/admin/users", token=A)
check("сразу после: /admin/users тем же access → 403 forbidden", s == 403 and d.get("code") == "forbidden", f"{s} {d.get('code')}")
s, d = api("POST", f"/admin/users/{g31}/deposit", {"amount_pln": 1, "method": "card"}, token=A)
check("депозит гостю тем же access → 403, кошелёк не тронут", s == 403 and wallet("Guest31") == w31, f"{s} {d.get('code')}")
check("live-канал уволенного закрыт сервером с кодом 4403", (c := ws_read(sock)[1]) == 4403, c)
check("новый тикет на канал не выдаётся → 403", ticket(A)[0] == 403)
check("тикет, взятый до увольнения, сокет не открывает", (c := ws_admin(early)[1]) in (401, 403), c)
s, d = api("GET", "/me", token=A)
check("/me тем же access работает, роль в ответе — player", s == 200 and d.get("role") == "player", f"{s} {d.get('role')}")
s, d = api("POST", "/auth/refresh", {"refresh_token": R})
A2 = d.get("access_token", "")
check("refresh старым refresh → 200, в новом access роль player", s == 200 and jwt_claims(A2).get("role") == "player", f"{s} {jwt_claims(A2).get('role')}")

section("назначение и снятие роли — без перелогина")
s, d = api("POST", "/admin/staff", {"nickname": "z0role"}, "boss")
check("назначение → 200", s == 200, J(d))
check("назначенный: /admin/users токеном с ролью player → 200", api("GET", "/admin/users", token=A2)[0] == 200)
sock2, st = ws_admin(ticket(A2)[1].get("ticket", ""))
check("… и live-канал открывается (101)", st == 101, st)
s, d = api("DELETE", f"/admin/staff/{zid}", None, "boss")
check("снятие роли → 200", s == 200, J(d))
check("снятый: /admin/users → 403 сразу", api("GET", "/admin/users", token=A2)[0] == 403)
check("… его live-канал закрыт с кодом 4403", (c := ws_read(sock2)[1]) == 4403, c)
check("… /me работает", api("GET", "/me", token=A2)[0] == 200)

section("остальной персонал как был")
s1, d1 = api("GET", "/admin/settings", None, "admin1")
check("admin1: /admin/users → 200, /admin/settings → 403 owner_only", api("GET", "/admin/users", None, "admin1")[0] == 200
      and s1 == 403 and d1.get("code") == "owner_only", f"{s1} {d1.get('code')}")
check("boss: /admin/settings → 200", api("GET", "/admin/settings", None, "boss")[0] == 200)
done()
