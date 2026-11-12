// З24-и6 «Сезонные пакеты — экран» (№55), браузер. Временная зона «z24w Зона» (10 zł/ч), гость z24wn («новый»:
// регистрация — сейчас, сессий нет), пакет «z24w Завтра» с продажей с завтрашнего дня (API). Владелец, «Экономика» →
// «+ Пакет»: «z24w Будни», 120 мин, 15 zł, продаётся по дату через 30 дней, 1 на гостя, только новым, тратится — все
// дни, кроме сегодняшнего, 00:00–17:00 — в списке пометки «сезонный» и «только новым», условия словами; «Правка» —
// поля на месте, «на гостя» 2 — журнал «на гостя: 1 → 2». Админ, карточка z24wn → «Пакет»: «z24w Будни» с пометками и
// условиями под ним, «z24w Завтра» нет (не продаётся); выдача картой — в пакетах гостя «тратится … · сейчас не
// тратится»; третья выдача — тост «не больше 2 на гостя, у гостя уже 2». Телефон 360 — форма пакета без прокрутки
// вбок. В конце выдачи, пакеты «z24w …» и зона удаляются SQL-ом.
const z = require('./_z');
(async () => {
  const G = 'z24wn';
  if (!z.uid(G)) await z.api('POST', '/auth/register', { nickname: G, password: z.GUEST_PASS });
  const U = z.uid(G);
  const wipe = () => { z.sql(`DELETE FROM pack_spends WHERE user_package_id IN (SELECT id FROM user_packages WHERE user_id = '${U}')`);
    z.sql(`DELETE FROM user_packages WHERE user_id = '${U}'`); z.sql(`DELETE FROM time_packages WHERE name LIKE 'z24w %'`);
    z.sql(`DELETE FROM zones WHERE name LIKE 'z24w %'`); };
  wipe();
  z.atEnd(wipe);
  z.sql(`UPDATE users SET registered_at = now() WHERE id = '${U}'`);
  const now = new Date(), ymd = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const dru = d => `${String(d.getDate()).padStart(2, '0')}.${String(d.getMonth() + 1).padStart(2, '0')}.${d.getFullYear()}`;
  const plus = n => new Date(now.getFullYear(), now.getMonth(), now.getDate() + n);
  const todayBit = (now.getDay() + 6) % 7; // пн = 0
  const zid = (((await z.api('POST', '/admin/zones', { name: 'z24w Зона', rate_pln: 10 }, 'boss')).data || {}).zone || {}).id;
  const tmr = (((await z.api('POST', '/admin/packages', { name: 'z24w Завтра', zone_id: zid, minutes: 60, price_pln: 5, sell_from: ymd(plus(1)) }, 'boss')).data || {}).package || {}).id;
  z.check('зона «z24w Зона», пакет «z24w Завтра» с продажей с завтрашнего дня (API), гость z24wn — новый', !!zid && !!tmr && !!U, JSON.stringify([zid, tmr, U]));
  const DAYS = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'];
  const left = [0, 1, 2, 3, 4, 5, 6].filter(i => i !== todayBit);
  const daysText = (() => { // как daysMaskText на сервере: подряд идущие — «Пн–Ср», одиночные — через запятую
    const out = []; let i = 0;
    while (i < 7) { if (!left.includes(i)) { i++; continue; } let j = i; while (j + 1 < 7 && left.includes(j + 1)) j++;
      out.push(j - i >= 2 ? `${DAYS[i]}–${DAYS[j]}` : j === i ? DAYS[i] : `${DAYS[i]}, ${DAYS[j]}`); i = j + 1; }
    return out.join(', '); })();
  const SEASON = `продаётся по ${dru(plus(30))} · 1 на гостя · только новым · тратится ${daysText} 00:00–17:00`;

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'settings'); await z.sleep(800);
  const has = await boss.evaluate(() => typeof packSeasonHtml === 'function');
  if (!has) { // сезонности в форме нет (до З24-и6) — FAIL по каждой проверке, а не обрыв
    for (const n of ['форма и список', 'правка', 'выдача', 'пакеты гостя', 'лимит', 'телефон 360']) z.check(n + ': сезонности в админке нет', false);
    return z.done();
  }
  await boss.evaluate(() => packEdit('')); await z.sleep(700);
  await boss.fill('#pk_name', 'z24w Будни');
  await boss.click('#drawer .uform .segs button:has-text("z24w Зона")'); await z.sleep(400);
  await boss.fill('#pk_min', '120'); await boss.fill('#pk_price', '15');
  await boss.fill('#pk_st', ymd(plus(30))); await boss.fill('#pk_pg', '1'); await boss.check('#pk_new');
  await boss.evaluate(b => { pkDays(127); const c = document.querySelector(`.pkDay[data-b="${b}"]`); if (c) c.checked = false; }, todayBit);
  await boss.fill('#pk_ss', '00:00'); await boss.fill('#pk_se', '17:00');
  await z.shot(boss, 'z24_pack_form');
  await boss.evaluate(() => [...document.querySelectorAll('#drawer .uform button')].find(b => b.textContent.trim() === 'Сохранить').click());
  await z.sleep(1500);
  const row = () => boss.evaluate(() => { const r = [...document.querySelectorAll('#packsTable tr')].find(t => t.cells[0] && /z24w Будни/.test(t.cells[0].textContent));
    return r ? { marks: [...r.cells[0].querySelectorAll('.pkMark')].map(m => m.textContent), season: (r.cells[0].querySelector('.pkSeason') || {}).textContent || '' } : null; });
  const r1 = await row();
  const db1 = z.sql(`SELECT concat_ws('|', COALESCE(sell_from::text, ''), sell_to, per_guest, new_only, spend_days_mask, spend_start_min, spend_end_min) FROM time_packages WHERE name = 'z24w Будни'`);
  z.check(`«+ Пакет» с сезонностью — в списке «сезонный», «только новым» и «${SEASON}»`,
    !!r1 && JSON.stringify(r1.marks) === JSON.stringify(['сезонный', 'только новым']) && r1.season === SEASON
    && db1 === `|${ymd(plus(30))}|1|t|${127 & ~(1 << todayBit)}|0|1020`, JSON.stringify([r1, db1]));
  const pid = z.sql("SELECT id FROM time_packages WHERE name = 'z24w Будни'");
  await boss.evaluate(id => packEdit(id), pid); await z.sleep(700);
  const f = await boss.evaluate(() => ({ st: document.getElementById('pk_st').value, pg: document.getElementById('pk_pg').value, nw: document.getElementById('pk_new').checked,
    days: [...document.querySelectorAll('.pkDay')].filter(c => c.checked).length, ss: document.getElementById('pk_ss').value, se: document.getElementById('pk_se').value }));
  await boss.fill('#pk_pg', '2');
  await boss.evaluate(() => [...document.querySelectorAll('#drawer .uform button')].find(b => b.textContent.trim() === 'Сохранить').click());
  await z.sleep(1500);
  const upd = z.sql("SELECT details FROM admin_actions WHERE action = 'package_update' AND details LIKE 'z24w Будни:%' ORDER BY created_at DESC LIMIT 1");
  z.check('«Правка»: поля на месте (по дату, 1, только новым, 6 дней, 00:00–17:00); «на гостя» 2 — журнал «z24w Будни: на гостя: 1 → 2»',
    f.st === ymd(plus(30)) && f.pg === '1' && f.nw && f.days === 6 && f.ss === '00:00' && f.se === '17:00' && upd === 'z24w Будни: на гостя: 1 → 2',
    JSON.stringify([f, upd]));

  // выдача у стойки
  await z.sleep(2500);
  const adm = await z.openAdmin('admin1');
  await adm.evaluate(id => openUserDrawer(id, 'pack'), U); await z.sleep(2000);
  const desk = () => adm.evaluate(() => ({ btns: [...document.querySelectorAll('#drawer .uform .segs')][0]
      ? [...[...document.querySelectorAll('#drawer .uform .segs')][0].querySelectorAll('button')].map(b => b.innerText.replace(/\s+/g, ' ').trim()) : [],
    season: (document.getElementById('packSeason') || {}).textContent || '' }));
  await adm.evaluate(n => { const b = [...document.querySelectorAll('#drawer .uform .segs button')].find(x => x.textContent.startsWith(n)); if (b) b.click(); }, 'z24w Будни');
  await z.sleep(400);
  const d1 = await desk();
  const SEASON2 = SEASON.replace('1 на гостя', '2 на гостя');
  z.check('карточка гостя → «Пакет»: «z24w Будни» с пометками «сезонный», «только новым», условия под ним; «z24w Завтра» нет',
    d1.btns.some(t => t === 'z24w Будни сезонный только новым') && !d1.btns.some(t => /z24w Завтра/.test(t)) && d1.season === SEASON2, JSON.stringify(d1));
  const sell = async () => { await adm.evaluate(() => { const b = [...document.querySelectorAll('#drawer .uform .segs button')].find(x => x.textContent.trim() === 'Карта'); if (b) b.click(); });
    await z.sleep(300);
    await adm.evaluate(() => { const b = [...document.querySelectorAll('#drawer .uform button')].find(x => /Выдать пакет/.test(x.textContent)); if (b) b.click(); });
    await z.sleep(1500); return (await z.text(adm, '#toast')) || ''; };
  const t1 = await sell();
  await adm.evaluate(id => openUserDrawer(id, 'pack'), U); await z.sleep(1500);
  await adm.evaluate(() => { uTab = 'pack'; renderUserDrawer(); }); await z.sleep(600);
  const mine = await adm.evaluate(() => [...document.querySelectorAll('#drawer table.hist tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => /z24w Будни/.test(t)));
  z.check(`выдача картой — тост «z24w Будни — 120 мин»; в пакетах гостя — «тратится ${daysText} 00:00–17:00 · сейчас не тратится»`,
    /z24w Будни — 120 мин на счету/.test(t1) && mine.length === 1 && mine[0].includes(`тратится ${daysText} 00:00–17:00 · сейчас не тратится`), JSON.stringify([t1, mine]));
  await adm.evaluate(id => openUserDrawer(id, 'pack'), U); await z.sleep(1500);
  await adm.evaluate(n => { const b = [...document.querySelectorAll('#drawer .uform .segs button')].find(x => x.textContent.startsWith(n)); if (b) b.click(); }, 'z24w Будни');
  await z.sleep(300);
  const t2 = await sell();
  await adm.evaluate(id => openUserDrawer(id, 'pack'), U); await z.sleep(1500);
  await adm.evaluate(n => { const b = [...document.querySelectorAll('#drawer .uform .segs button')].find(x => x.textContent.startsWith(n)); if (b) b.click(); }, 'z24w Будни');
  await z.sleep(300);
  const t3 = await sell();
  const n = +z.sql(`SELECT count(*) FROM user_packages WHERE user_id = '${U}' AND voided_at IS NULL`);
  z.check('вторая выдача — можно (2 на гостя), третья — тост «не больше 2 на гостя, у гостя уже 2», выдач — 2',
    /120 мин на счету/.test(t2) && t3.includes('Пакет «z24w Будни» — не больше 2 на гостя, у гостя уже 2') && n === 2, JSON.stringify([t2, t3, n]));

  // телефон 360
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'settings'); await z.sleep(800);
  await ph.evaluate(id => packEdit(id), pid); await z.sleep(800);
  await ph.evaluate(() => { const e = document.getElementById('pk_dmask'); if (e) e.scrollIntoView({ block: 'center' }); }); await z.sleep(300);
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
    r: [...document.querySelectorAll('#drawer .uform .input, #drawer .uform button, .pkDay')].map(e => Math.round(e.getBoundingClientRect().right)) }));
  await z.shot(ph, 'z24_pack_phone');
  z.check('телефон 360: форма пакета с сезонностью — поля и кнопки в экране, вбок не прокручивается',
    fit.r.length > 15 && fit.r.every(x => x <= fit.w) && fit.sw <= fit.w, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
