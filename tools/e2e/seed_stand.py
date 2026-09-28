#!/usr/bin/env python3
"""Сид стенда: клуб, 2 зоны, 12 ПК, владелец boss/Boss12345, админы admin1/admin2 (Admin12345),
гости трека Ж (Guest06…Guest30 и четверо с «живыми» никами) и трека З (Guest31…Guest50), пароль
Guest12345; товары, пакеты, шаблоны смен. Гости регистрируются через API (ачивки/кейсы как в
жизни), роли и деньги — SQL. Повторный запуск на засеянной базе безопасен: заводит только
недостающих гостей и ничего не перезаписывает."""
import os, json, subprocess, time, urllib.request, random

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
random.seed(1448)

def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit("SQL FAIL: " + q[:120] + "\n" + r.stderr)
    return r.stdout.strip()

def api(method, path, body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(API + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, json.loads(resp.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")

CLUB = "00000000-0000-0000-0000-000000000001"
sql(f"""INSERT INTO clubs (id,name,address,base_rate_pln,rtp_modifier,is_active,layout_w,layout_h)
VALUES ('{CLUB}','14:48 Białołęka','Warszawa, ul. Demo 1',20.00,1.00,TRUE,12,8) ON CONFLICT (id) DO NOTHING""")
Z_STD = "00000000-0000-0000-0000-00000000aa01"
Z_VIP = "00000000-0000-0000-0000-00000000aa02"
sql(f"""INSERT INTO zones (id,club_id,name,rate_pln,sort) VALUES
('{Z_STD}','{CLUB}','Standard',20.00,1),('{Z_VIP}','{CLUB}','VIP',30.00,2) ON CONFLICT (id) DO NOTHING""")
sql("DELETE FROM zones WHERE name='Общая' AND id NOT IN ('%s','%s')" % (Z_STD, Z_VIP))
rows = []
for i in range(1, 13):
    zid = Z_VIP if i > 9 else Z_STD
    zname = "VIP" if i > 9 else "Standard"
    name = f"VIP-{i-9:02d}" if i > 9 else f"PC-{i:02d}"
    pid = f"00000000-0000-0000-0000-0000000000{i:02x}"
    x = (i - 1) % 6 * 2
    y = (i - 1) // 6 * 3
    mac = f"AA:BB:CC:DD:EE:{i:02X}"
    rows.append(f"('{pid}','{CLUB}','{name}','{zname}','available',{x},{y},'{mac}','{zid}')")
sql("INSERT INTO computers (id,club_id,name,zone,status,pos_x,pos_y,mac,zone_id) VALUES " + ",".join(rows) + " ON CONFLICT (id) DO NOTHING")

# Персонал
staff = [("boss", "Boss12345", "owner"), ("admin1", "Admin12345", "admin"), ("admin2", "Admin12345", "admin")]
for nick, pw, role in staff:
    st, r = api("POST", "/auth/register", {"nickname": nick, "password": pw})
    time.sleep(0.15)
    sql(f"UPDATE users SET role='{role}', onboarded_at=NOW(), onboarded_version=99 WHERE nickname='{nick}'")

# Гости
first = ["Jan", "Piotr", "Anna", "Kasia", "Marek", "Ola", "Tomek", "Zofia", "Paweł", "Ewa", "Kuba", "Michał", "Igor", "Egor", "Nika"]
last = ["Kowalski", "Nowak", "Wiśniewski", "Wójcik", "Kowalczyk", "Kamiński", "Lewandowski", "Zieliński", "Szymański", "Woźniak"]
guests = []
for i in range(1, 31):
    nick = f"Guest{i:02d}" if i > 5 else ["Ян", "Янек", "Яна", "s1mple", "Faker"][i - 1]
    phone = f"+4850012{i:04d}"
    body = {"nickname": nick, "password": "Guest12345", "phone": phone}
    if i % 3 == 0:
        body["email"] = f"guest{i}@example.com"
    st, r = api("POST", "/auth/register", body)
    time.sleep(0.15)
    if st not in (200, 201):
        if st != 409:  # 409 — гость уже есть (повторный запуск)
            print("register", nick, st, r)
        continue
    uid = r["user"]["id"]
    guests.append((nick, uid))
    fn, ln = random.choice(first), random.choice(last)
    if i == 2:  # два Ковальских для неоднозначности
        fn, ln = "Adam", "Kowalski"
    if i == 3:
        fn, ln = "Adam", "Kowalski"
    wallet = random.choice([0, 500, 1500, 3000, 6000, 12000])
    coins = random.choice([0, 50, 200, 800, 2500])
    xp = random.choice([0, 400, 1500, 6000, 20000])
    reg_days = random.randint(0, 120)
    sql(f"""UPDATE users SET first_name='{fn}', last_name='{ln}', wallet_grosz={wallet}, coins_balance={coins},
        xp_total={xp}, xp_current={xp % 1000}, level={1 + xp // 1000},
        registered_at=NOW()-interval '{reg_days} days', last_active_at=NOW()-interval '{random.randint(0, reg_days)} days'
        WHERE id='{uid}'""")
    if wallet:
        sql(f"""INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, created_at)
        VALUES ('{uid}','deposit',{wallet},{wallet},'seed',NOW()-interval '{reg_days} days')""")

# Гости трека З (OWNER.md, раздел «Гости»): Guest31…Guest50. Идут ПОСЛЕ гостей трека Ж и без
# random — случайные значения Guest06…Guest30 не сдвигаются. Кошелёк, монеты и опыт — как после
# регистрации (нули): что нужно проверке, она кладёт сама через API. Зарегистрированы 60–79 дней
# назад, заходили 30 дней назад — не «новые» и не «сегодняшние».
z_first = ["Oskar", "Lena", "Wiktor", "Maja", "Filip", "Hanna", "Borys", "Iga", "Szymon", "Julia"]
z_last = ["Mazur", "Krawczyk", "Kaczmarek", "Grabowski", "Pawlak", "Michalski", "Król", "Wieczorek", "Jabłoński", "Nowicki"]
for i in range(31, 51):
    nick = f"Guest{i}"
    body = {"nickname": nick, "password": "Guest12345", "phone": f"+4850012{i:04d}"}
    if i % 3 == 0:
        body["email"] = f"guest{i}@example.com"
    st, r = api("POST", "/auth/register", body)
    time.sleep(0.15)
    if st not in (200, 201):
        if st != 409:
            print("register", nick, st, r)
        continue
    uid = r["user"]["id"]
    guests.append((nick, uid))
    k = i - 31
    sql(f"""UPDATE users SET first_name='{z_first[k % 10]}', last_name='{z_last[(k + k // 10) % 10]}',
        registered_at=NOW()-interval '{i + 29} days', last_active_at=NOW()-interval '30 days' WHERE id='{uid}'""")

# Товары, пакеты, смены — только на пустую базу (повторный запуск их не дублирует)
sql(f"""INSERT INTO goods (club_id,name,category,price_pln,stock,low_stock,sort,active) VALUES
('{CLUB}','Coca-Cola 0.5','Напитки',8.00,24,5,1,TRUE),('{CLUB}','Red Bull','Напитки',12.00,3,5,2,TRUE),
('{CLUB}','Lay''s','Снеки',7.00,15,3,3,TRUE),('{CLUB}','Snickers','Снеки',5.00,0,3,4,TRUE),
('{CLUB}','Woda 0.5','Напитки',4.00,40,10,5,TRUE) ON CONFLICT (club_id, name) DO NOTHING""")
# З10-и3: остаток сида — начальной партией, как у миграции 077 (иначе партии разойдутся с остатком)
sql("""INSERT INTO stock_lots (club_id, good_id, kind, qty_in, qty_left, received_at, part)
SELECT g.club_id, g.id, 'initial', g.stock, g.stock, now(), 1 FROM goods g
WHERE g.stock > 0 AND NOT EXISTS (SELECT 1 FROM stock_lots l WHERE l.good_id = g.id)""")
# Пакеты времени
sql(f"""INSERT INTO time_packages (club_id,zone_id,name,minutes,price_pln,days_valid,sort,active)
SELECT * FROM (VALUES
('{CLUB}'::uuid,'{Z_STD}'::uuid,'3 часа Standard',180,50.00,30,1,TRUE),('{CLUB}'::uuid,'{Z_STD}'::uuid,'5 часов Standard',300,75.00,30,2,TRUE),
('{CLUB}'::uuid,'{Z_VIP}'::uuid,'3 часа VIP',180,75.00,0,3,TRUE),('{CLUB}'::uuid,'{Z_STD}'::uuid,'Ночь Standard',480,90.00,7,4,FALSE)) v
WHERE NOT EXISTS (SELECT 1 FROM time_packages WHERE club_id='{CLUB}')""")
# Смены
sql(f"""INSERT INTO shifts (club_id,name,start_min,end_min,days_mask,sort,active)
SELECT * FROM (VALUES ('{CLUB}'::uuid,'День',480,1200,127,1,TRUE),('{CLUB}'::uuid,'Ночь',1200,480,127,2,TRUE)) v
WHERE NOT EXISTS (SELECT 1 FROM shifts WHERE club_id='{CLUB}')""")
admin1 = sql("SELECT id FROM users WHERE nickname='admin1'")
day = sql("SELECT id FROM shifts WHERE name='День'")
night = sql("SELECT id FROM shifts WHERE name='Ночь'")
boss = sql("SELECT id FROM users WHERE nickname='boss'")
# сегодня — по часам клуба: сессия psql живёт в UTC, и CURRENT_DATE с 00:00 до 02:00 по Варшаве — вчера (З2-и2)
today = "(now() AT TIME ZONE 'Europe/Warsaw')::date"
sql(f"INSERT INTO shift_assignments (club_id,date,shift_id,user_id,created_by) VALUES ('{CLUB}',{today},'{day}','{admin1}','{boss}'),('{CLUB}',{today},'{night}',(SELECT id FROM users WHERE nickname='admin2'),'{boss}') ON CONFLICT (date, shift_id, user_id) DO NOTHING")
print("guests:", len(guests), "· гостей в базе:", sql("SELECT count(*) FROM users WHERE role='player'"))
print(sql("SELECT count(*) FROM users"), "users;", sql("SELECT count(*) FROM computers"), "computers")
