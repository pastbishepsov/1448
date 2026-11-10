// З18-и6 «Карточка клуба — экраны» (№34, З-Р23), браузер. Владелец, «Экономика» → «Клуб»: карточка — имя, адрес,
// телефон, Telegram, Instagram, часы работы («Круглосуточно» или по дням: выходной, часы, круглые сутки), клубный тариф
// и «ПК без зоны» — с сервера; «Сохранить карточку» без правок — тост «Правок нет», запросов нет; правка подсвечена,
// «Изменено: N»; сохранение — один PATCH только с изменённым, Telegram «@…» сервер приводит к имени, в журнале —
// «карточка клуба» строкой «было → стало». «ПК без зоны: запрещён», пока z18cp-1 без зоны, — тост с его именем, запрет
// остаётся правкой; дали зону — включается, у зон — «ПК без зоны запрещены». Палитра «часы работы» → карточка. Телефон
// 360 — карточка в экране, прокрутки вбок нет. PWA 390×844 — цена часа по зонам («Standard 20 zł/час · VIP 30 zł/час»),
// а не клубный тариф; APP_VER — как app_version манифеста. Карточка клуба на время — без телефона, соцсетей и часов, в
// конце — SQL-ом как была; ПК z18cp-1 удаляется в начале и в конце; pc_zone_required — как была.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const CLUB = z.sql('SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1');
  const ORIG = z.sql(`SELECT row_to_json(c)::text FROM clubs c WHERE id = '${CLUB}'`), was = JSON.parse(ORIG);
  const COLS = 'name, address, phone, telegram, instagram, working_hours, base_rate_pln', q = s => '$z18cp$' + s + '$z18cp$';
  const zr0 = z.rows("SELECT value FROM settings WHERE key = 'pc_zone_required'")[0];
  const PCS = "DELETE FROM computers WHERE name LIKE 'z18cp-%'";
  z.sql(PCS);
  z.atEnd(() => z.sql(PCS));
  z.atEnd(() => z.sql(`UPDATE clubs SET (${COLS}) = (SELECT ${COLS} FROM json_populate_record(NULL::clubs, ${q(ORIG)})) WHERE id = '${CLUB}'`));
  z.atEnd(() => { z.sql("DELETE FROM settings WHERE key = 'pc_zone_required'");
    if (zr0 !== undefined) z.sql(`INSERT INTO settings (key, value) VALUES ('pc_zone_required', '${zr0}')`); });
  z.sql("DELETE FROM settings WHERE key = 'pc_zone_required'");
  z.sql(`UPDATE clubs SET phone = NULL, telegram = NULL, instagram = NULL, working_hours = NULL WHERE id = '${CLUB}'`);
  const P1 = ((await z.api('POST', '/admin/computers', { name: 'z18cp-1' }, 'boss')).data || {}).computer_id;
  const STD = z.sql("SELECT id FROM zones WHERE name = 'Standard'");
  const card = async () => (await z.api('GET', `/clubs/${CLUB}`)).data || {};
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  // нового на HEAD нет — проверка упадёт, прогон нет
  const tap = (p, sel) => p.click(sel, { timeout: 4000 }).catch(() => {});
  const put = (p, sel, v) => p.fill(sel, v, { timeout: 4000 }).catch(() => {});
  const pick = (p, sel, v) => p.selectOption(sel, v, { timeout: 4000 }).catch(() => {});

  const boss = await z.openAdmin('boss');
  const reqs = [];
  boss.on('request', r => { if (/\/admin\/clubs\/|\/admin\/settings$/.test(r.url()) && ['PATCH', 'PUT'].includes(r.method())) reqs.push([r.method(), r.postData() || '']); });
  const toasts = [];
  await boss.exposeFunction('z18cpToast', t => toasts.push(t));
  await boss.evaluate(() => { const t0 = window.toast; window.toast = (m, ...a) => { window.z18cpToast(String(m)); return t0(m, ...a); }; });
  await z.nav(boss, 'settings');
  await boss.waitForSelector('#club_name', { timeout: 15000 }).catch(() => {});
  await z.sleep(800);
  const form = () => boss.evaluate(() => { const v = id => (document.getElementById(id) || {}).value;
    return { name: v('club_name'), addr: v('club_addr'), phone: v('club_phone'), tg: v('club_tg'), ig: v('club_ig'), hmode: v('club_hmode'),
      days: !document.getElementById('clubDays')?.classList.contains('hidden'), rate: v('club_rate'), zr: v('club_zonereq'),
      hint: (document.getElementById('clubHint') || {}).textContent, zones: (document.getElementById('zonesHint') || {}).textContent,
      changed: [...document.querySelectorAll('#clubWrap .changed')].map(e => e.id) }; });
  const f0 = await form();
  z.check(`«Экономика» → «Клуб»: имя «${was.name}», адрес, тариф ${+was.base_rate_pln} — с сервера; телефона и соцсетей нет; часы — «Круглосуточно»; `
    + 'ПК без зоны — «Можно»; «Правок нет»', f0.name === was.name && f0.addr === was.address && +f0.rate === +was.base_rate_pln && f0.phone === ''
    && f0.tg === '' && f0.ig === '' && f0.hmode === '0' && !f0.days && f0.zr === '0' && f0.hint === 'Правок нет', JSON.stringify(f0));
  await z.shot(boss, 'z18_club_card');

  await tap(boss, '#clubSave'); await z.sleep(600);
  z.check('«Сохранить карточку» без правок — тост «Правок нет», запросов нет', reqs.length === 0 && /Правок нет/.test(toasts[toasts.length - 1] || ''),
    JSON.stringify({ reqs, t: toasts.slice(-1) }));

  await put(boss, '#club_name', 'z18cp Клуб'); await put(boss, '#club_phone', '+48 22 765 43 21'); await put(boss, '#club_tg', '@z18cp_club');
  await pick(boss, '#club_hmode', '1');
  for (const d of ['mon', 'tue', 'wed', 'thu', 'fri']) {
    await pick(boss, '#club_dm_' + d, 'hours'); await put(boss, '#club_do_' + d, '10:00'); await put(boss, '#club_dc_' + d, '23:00');
  }
  await pick(boss, '#club_dm_sun', 'off');
  await put(boss, '#club_rate', '22.5');
  const f1 = await form();
  z.check('правка: имя, телефон, Telegram, часы по дням (пн–пт 10:00–23:00, сб круглые сутки, вс выходной), тариф 22,5 — подсвечены, '
    + '«Изменено: 5»', f1.hint === 'Изменено: 5' && f1.days && JSON.stringify(f1.changed.sort()) === JSON.stringify(['club_hmode', 'club_name', 'club_phone', 'club_rate', 'club_tg']),
    JSON.stringify(f1));
  await tap(boss, '#clubSave'); await z.sleep(1500);
  const sent = reqs.map(([m, b]) => [m, Object.keys(JSON.parse(b || '{}')).sort().join(',')]);
  const c1 = await card(), f2 = await form();
  const H = { mon: { open: '10:00', close: '23:00' }, tue: { open: '10:00', close: '23:00' }, wed: { open: '10:00', close: '23:00' },
    thu: { open: '10:00', close: '23:00' }, fri: { open: '10:00', close: '23:00' }, sat: { open: '00:00', close: '24:00' } };
  z.check('«Сохранить карточку» — один PATCH только с изменённым (имя, телефон, Telegram, часы, тариф), без PUT; на сервере — то же, '
    + 'Telegram — «z18cp_club»; в форме — с сервера, «Правок нет»', JSON.stringify(sent) === JSON.stringify([['PATCH', 'base_rate_pln,name,phone,telegram,working_hours']])
    && c1.name === 'z18cp Клуб' && c1.phone === '+48 22 765 43 21' && c1.telegram === 'z18cp_club' && c1.base_rate_pln === 22.5
    && JSON.stringify(c1.working_hours) === JSON.stringify(Object.fromEntries(Object.keys(c1.working_hours || {}).map(k => [k, H[k]])))
    && Object.keys(c1.working_hours || {}).sort().join() === Object.keys(H).sort().join() && f2.tg === 'z18cp_club' && f2.hint === 'Правок нет'
    && /Карточка клуба сохранена/.test(toasts[toasts.length - 1] || ''), JSON.stringify({ sent, c1, f2: [f2.tg, f2.hint], t: toasts.slice(-1) }));

  await z.nav(boss, 'grants'); await z.sleep(1200);
  const jr = await boss.evaluate(() => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => t.includes('z18cp Клуб')).slice(0, 2));
  z.check('«Журнал»: «карточка клуба» — «имя … → «z18cp Клуб»», часы словами, тариф', jr.some(t => t.includes('карточка клуба') && t.includes(`имя «${was.name}» → «z18cp Клуб»`)
    && t.includes('часы круглосуточно → пн–пт 10:00–23:00, сб 00:00–24:00, выходной: вс') && t.includes('клубный тариф')), JSON.stringify(jr));

  await z.nav(boss, 'settings'); await boss.waitForSelector('#club_zonereq', { timeout: 15000 }).catch(() => {}); await z.sleep(800);
  reqs.length = 0;
  await pick(boss, '#club_zonereq', '1'); await tap(boss, '#clubSave'); await z.sleep(1500);
  const f3 = await form(), zr1 = (await z.api('GET', '/admin/settings', undefined, 'boss')).data.settings.pc_zone_required;
  z.check('«ПК без зоны: запрещён», пока z18cp-1 без зоны, — только PUT, тост с его именем; запрет не включён и остаётся правкой («Изменено: 1»)',
    reqs.length === 1 && reqs[0][0] === 'PUT' && /z18cp-1/.test(toasts[toasts.length - 1] || '') && zr1 === 0 && f3.zr === '1' && f3.hint === 'Изменено: 1',
    JSON.stringify({ reqs, t: toasts.slice(-1), zr1, f3: [f3.zr, f3.hint] }));
  await z.api('PATCH', `/admin/computers/${P1}`, { zone_id: STD }, 'boss');
  await tap(boss, '#clubSave'); await z.sleep(1500);
  const f4 = await form(), zr2 = (await z.api('GET', '/admin/settings', undefined, 'boss')).data.settings.pc_zone_required;
  z.check('z18cp-1 в зоне — «Сохранить карточку»: запрет включён, «Правок нет»; у зон — «ПК без зоны запрещены»',
    zr2 === 1 && f4.zr === '1' && f4.hint === 'Правок нет' && /^ПК без зоны запрещены/.test(f4.zones || ''), JSON.stringify({ zr2, f4: [f4.zr, f4.hint, f4.zones] }));
  await z.shot(boss, 'z18_club_zonereq');

  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'часы работы'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(2500);
  const pg = await boss.evaluate(() => ({ tab: curTab, focus: (document.activeElement || {}).id }));
  z.check('палитра: «часы работы» → «Карточка клуба», Enter — «Экономика», курсор в имени клуба', items[0] === 'Карточка клуба' && pg.tab === 'settings'
    && pg.focus === 'club_name', JSON.stringify({ items, pg }));

  await boss.setViewportSize({ width: 360, height: 780 }); await z.sleep(800);
  await boss.evaluate(() => { const e = document.getElementById('clubDays'); if (e) e.scrollIntoView({ block: 'center' }); });
  const ph = await boss.evaluate(() => { const els = [...document.querySelectorAll('#clubWrap .input, #clubWrap .btn')];
    return { n: els.length, out: els.filter(e => e.getBoundingClientRect().right > innerWidth + 0.5).map(e => e.id), sw: document.documentElement.scrollWidth }; });
  await z.sleep(300);
  await z.shot(boss, 'z18_club_phone');
  z.check('телефон 360: поля и кнопки карточки клуба — в экране, прокрутки вбок нет', ph.n > 20 && ph.out.length === 0 && ph.sw <= 360, JSON.stringify(ph));

  await boss.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const zones = ((await z.api('GET', '/clubs')).data.clubs || []).find(c => c.id === CLUB).zones || [];
  const WANT = zones.map(x => `${x.name} ${(+x.rate_pln).toLocaleString('ru-RU')} zł/час`).join(' · ');
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close().catch(() => {}));
  const a = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })).newPage();
  a.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await a.goto(z.BASE + '/app', { waitUntil: 'load' });
  await a.fill('#nick', 'Guest48'); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
  await a.waitForFunction(() => { const m = document.getElementById('clubMeta'); return m && m.textContent.trim() !== ''; }, null, { timeout: 15000 }).catch(() => {});
  const pwa = await a.evaluate(async () => ({ meta: (document.getElementById('clubMeta') || {}).textContent || '', app: typeof APP_VER === 'undefined' ? null : APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  await z.shot(a, 'z18_club_pwa');
  z.check(`PWA: цена часа по зонам — «${WANT}», а не клубный тариф 22,5; APP_VER = app_version манифеста`,
    zones.length > 0 && norm(pwa.meta).startsWith(norm(WANT)) && !/22,5/.test(pwa.meta) && pwa.app && pwa.app === pwa.mf, JSON.stringify({ pwa, WANT }));
  await z.done();
})().catch(z.crash);
