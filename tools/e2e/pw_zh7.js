// Ж7 «Мелочи» (№27, 31, часть №30): чип смены без фона, крестик Phosphor вместо прицела,
// иконка канала персонала ≠ колокол, иконка «Кухня» вместо эмодзи, ⚠-тост треугольником,
// тост депозита с «хватит ещё на …», «ПК в ремонте» при посадке, виджет даты-времени брони
// «дд.мм.гггг чч:мм» (пресеты сегодня/завтра, четверти часа, RFC3339 наружу). Скриншоты.
const { chromium } = require('playwright-core');
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const { execSync } = require('child_process');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
async function j(url, opt) { const r = await fetch(url, opt); return r.json(); }
const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
(async () => {
  const login = async (l, p) => (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: l, password: p }) })).access_token;
  const a1 = await login('admin1', 'Admin12345'); const ot = await login('boss', 'Boss12345');
  const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + a1 }, ho = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + ot };
  const g = psql(`SELECT id FROM users WHERE nickname='Guest28'`);
  const pcs = (await j(API + '/admin/computers', { headers: h })).computers; const P = n => pcs.find(p => p.name === n);
  psql(`UPDATE computers SET status='available' WHERE name IN ('PC-07','PC-08') AND id NOT IN (SELECT computer_id FROM sessions WHERE status='active')`);
  psql(`UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id='${g}'`);
  // разметка: ни одного прицела
  const html = require('fs').readFileSync(WEB_DIR + '/admin.html', 'utf8');
  check('в разметке нет прицела: ни «crosshair», ни старого пути A96.14; иконка cross — Phosphor X', !/crosshair/i.test(html) && !/A96\.14/.test(html) && /cross: `<svg[^`]*M205\.66,194\.34/.test(html), '');
  check('в разметке нет эмодзи 🍔 у кнопки «Кухня», у канала персонала иконка chat', !/r-kitchen"[^>]*>\s*<span style="font-size:17px[^>]*>🍔/.test(html) && /id="staffBtn"[^]*?data-ic="chat"/.test(html.slice(0, html.indexOf('id="helpBtn"'))), '');

  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  await page.evaluate(() => nav('hall')); await sleep(1500);
  // и1: чип смены без фона и рамки
  const chip = await page.evaluate(() => { const c = document.getElementById('shiftChip'), s = getComputedStyle(c); return { bg: s.backgroundColor, border: s.borderStyle + ' ' + s.borderWidth, txt: c.textContent.trim() }; });
  check('чип смены: без фона (transparent) и без рамки', /rgba\(0, 0, 0, 0\)|transparent/.test(chip.bg) && /none|0px/.test(chip.border), JSON.stringify(chip));
  const icons = await page.evaluate(() => ({
    staff: document.querySelector('#staffBtn svg path[d^="M128,24A104"]') !== null || /M128,24A104/.test(document.getElementById('staffBtn').innerHTML),
    bell: /M56,104|bell/.test(document.getElementById('bellBtn').innerHTML) && !/M128,24A104/.test(document.getElementById('bellBtn').innerHTML),
    kitchen: /svg/.test(document.getElementById('r-kitchen').innerHTML) && !/🍔/.test(document.getElementById('r-kitchen').innerHTML),
    crossCount: [...document.querySelectorAll('svg path')].filter(p => /A96\.14/.test(p.getAttribute('d') || '')).length,
    xCount: [...document.querySelectorAll('svg path')].filter(p => /M205\.66,194\.34/.test(p.getAttribute('d') || '')).length,
  }));
  check('канал персонала — иконка чата, колокол — только у колокола, «Кухня» — svg без эмодзи, прицелов на странице 0', icons.staff && icons.bell && icons.kitchen && icons.crossCount === 0, JSON.stringify(icons));
  await page.screenshot({ path: OUT + '/zh7_topbar.png', clip: { x: 0, y: 0, width: 1440, height: 60 } });
  // ⚠-тост — треугольник, ✕ — крестик
  await page.evaluate(() => toast('⚠ Проверка предупреждения'));
  await sleep(300);
  const warnIc = await page.evaluate(() => /M236\.8,188\.09/.test(document.getElementById('toast').innerHTML));
  check('⚠-тост рисует треугольник Phosphor Warning, не крестик', warnIc, '');
  // крестик в шторке: «Снять придержание» на PC-07
  await j(API + '/admin/computers/' + P('PC-07').id + '/hold', { method: 'POST', headers: h, body: '{}' });
  await sleep(1300);
  await page.evaluate(id => openPCDrawer(id), P('PC-07').id); await sleep(500);
  const drawerX = await page.evaluate(() => { const b = [...document.querySelectorAll('#drawerBody button')].find(x => /Снять придержание/.test(x.textContent)); return b ? /M205\.66,194\.34/.test(b.innerHTML) : null; });
  check('шторка ПК: «Снять придержание» с крестиком X', drawerX === true, String(drawerX));
  await page.screenshot({ path: OUT + '/zh7_drawer.png', clip: { x: 1040, y: 0, width: 400, height: 900 } });
  await j(API + '/admin/computers/' + P('PC-07').id + '/hold', { method: 'DELETE', headers: h });
  // и2: посадка на ПК в ремонте → computer_maintenance
  await j(API + '/admin/computers/' + P('PC-08').id + '/status', { method: 'PATCH', headers: h, body: JSON.stringify({ status: 'maintenance' }) });
  const seatM = await (await fetch(API + '/admin/computers/' + P('PC-08').id + '/session', { method: 'POST', headers: h, body: JSON.stringify({ guest_id: g, planned_min: 30 }) })).json();
  check('посадка на ПК в ремонте → computer_maintenance «ПК в ремонте …»', seatM.code === 'computer_maintenance' && /ПК в ремонте/.test(seatM.message), JSON.stringify(seatM));
  await j(API + '/admin/computers/' + P('PC-08').id + '/status', { method: 'PATCH', headers: h, body: JSON.stringify({ status: 'available' }) });
  // и2: тост депозита с часами
  await page.evaluate(id => openUserDrawer(id, 'deposit'), g); await sleep(1200);
  await page.fill('#d_sum', '50');
  await page.click('#drawerBody button:has-text("Оформить депозит")');
  await sleep(1200);
  const depToast = await page.evaluate(() => document.getElementById('toast').textContent);
  const cheapest = +psql(`SELECT rate_pln FROM zones WHERE rate_pln > 0 ORDER BY rate_pln, sort, name LIMIT 1`);
  const expMin = Math.floor(5000 * 60 / Math.round(cheapest * 100));
  const expTxt = (Math.floor(expMin / 60) ? Math.floor(expMin / 60) + ' ч' + (expMin % 60 ? ' ' + (expMin % 60) + ' мин' : '') : expMin + ' мин');
  check('тост депозита: «+50,00 zł · +N монет · хватит ещё на 2 ч 30 мин в Standard» (без сессии — по дешёвой зоне)', /\+50,00 zł/.test(depToast) && depToast.includes('хватит ещё на ' + expTxt) && /в Standard/.test(depToast), depToast + ` (ожидалось «${expTxt}»)`);
  const depId = psql(`SELECT id FROM deposits WHERE user_id='${g}' ORDER BY created_at DESC LIMIT 1`);
  await j(API + '/admin/deposits/' + depId + '/void', { method: 'POST', headers: h, body: '{}' });
  // и3: виджет даты-времени брони
  await page.evaluate(() => { closeDrawer(); nav('bookings'); }); await sleep(1200);
  await page.evaluate(() => newBookingDrawer()); await sleep(800);
  const w = await page.evaluate(() => ({ hasDT: !!document.querySelector('#drawerBody input[type="datetime-local"]'), date: document.getElementById('b_date').value, hh: document.getElementById('b_hh').value, mm: document.getElementById('b_mm').value, txt: document.getElementById('bWhenTxt').textContent, mmOpts: [...document.querySelectorAll('#b_mm option')].map(o => o.textContent).join(','), on: [...document.querySelectorAll('#drawerBody .segs button.on')].map(b => b.textContent) }));
  const now = new Date(); const t = new Date(Date.now() + 30 * 60000); t.setMinutes(t.getMinutes() - t.getMinutes() % 15, 0, 0);
  const ruToday = String(t.getDate()).padStart(2, '0') + '.' + String(t.getMonth() + 1).padStart(2, '0') + '.' + t.getFullYear();
  check('виджет: datetime-local нет; дата «дд.мм.гггг» = сегодня (или завтра после полуночи), время = +30 мин по четвертям, минуты 00/15/30/45', !w.hasDT && w.date === ruToday && +w.hh === t.getHours() && +w.mm === t.getMinutes() && w.mmOpts === '00,15,30,45', JSON.stringify(w));
  check('подпись под виджетом: «день дд.мм · чч:мм»', /^(пн|вт|ср|чт|пт|сб|вс) \d\d\.\d\d · \d\d:\d\d$/.test(w.txt), w.txt);
  await page.click('#drawerBody button:has-text("Завтра")'); await sleep(200);
  const tomorrow = new Date(); tomorrow.setDate(tomorrow.getDate() + 1);
  const ruTomorrow = String(tomorrow.getDate()).padStart(2, '0') + '.' + String(tomorrow.getMonth() + 1).padStart(2, '0') + '.' + tomorrow.getFullYear();
  const w2 = await page.evaluate(() => ({ date: document.getElementById('b_date').value, on: [...document.querySelectorAll('#drawerBody .segs button.on')].map(b => b.textContent) }));
  check('пресет «Завтра» → дата завтрашняя, чип подсвечен', w2.date === ruTomorrow && w2.on.includes('Завтра'), JSON.stringify(w2));
  await page.fill('#b_date', '31.02.2026'); await page.dispatchEvent('#b_date', 'input'); await sleep(200);
  const bad = await page.evaluate(() => document.getElementById('bWhenTxt').textContent);
  check('31.02.2026 → подсказка «дата — дд.мм.гггг» (мусор не проходит)', /дата — дд\.мм\.гггг/.test(bad), bad);
  // бронь на завтра 18:15 → RFC3339 наружу, сервер принял, в списке 18:15
  await page.fill('#b_date', ruTomorrow); await page.dispatchEvent('#b_date', 'input');
  await page.selectOption('#b_hh', '18'); await page.selectOption('#b_mm', '15');
  await page.fill('#b_nick', 'Guest28');
  const reqs = [];
  page.on('request', r => { if (r.method() === 'POST' && /\/admin\/bookings$/.test(r.url())) reqs.push(r.postData()); });
  await page.click('#drawerBody button:has-text("Забронировать")');
  await sleep(1500);
  const sent = reqs.length ? JSON.parse(reqs[0]) : {};
  const local = sent.start_time ? new Date(sent.start_time) : null;
  const bkToast = await page.evaluate(() => document.getElementById('toast').textContent);
  check('«Забронировать»: start_time — RFC3339 (UTC, с Z), в местном времени это завтра 18:15; тост «📅 Бронь: Guest28 · … · 18:15»', /Z$/.test(sent.start_time || '') && local && local.getHours() === 18 && local.getMinutes() === 15 && local.getDate() === tomorrow.getDate() && /Бронь: Guest28/.test(bkToast) && /18:15/.test(bkToast), JSON.stringify({ start_time: sent.start_time, toast: bkToast }));
  await page.evaluate(() => newBookingDrawer()); await sleep(600);
  await page.screenshot({ path: OUT + '/zh7_booking.png', clip: { x: 1040, y: 0, width: 400, height: 700 } });
  psql(`UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id='${g}'`);
  check('js-ошибок нет', errors.length === 0, errors.join(' | '));
  await browser.close();
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
