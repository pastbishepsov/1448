// Ж2: playwright — карточка гостя («Запас минут» по зонам, форма выдачи с зоной ПК по
// умолчанию и пометкой), отчёт «Монеты» («Должны временем»), PWA (блок минут по зонам),
// шелл (блок запаса словами зоны и прогноз «хватит до» без минут чужой зоны).
const { chromium } = require('playwright-core');
const { idle, watch, signIn, reenter } = require('./_z'); // З19-и14: после записи — ждём страницу, а не паузу
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
async function j(url, opt) { const r = await fetch(url, opt); return r.json(); }
(async () => {
  // подготовка через API: Guest29 — 40 мин Standard за монеты + 7 мин в старом пуле; сидит за VIP-02
  const a1 = (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: 'admin1', password: 'Admin12345' }) })).access_token;
  const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + a1 };
  const users = (await j(API + '/admin/users?q=Guest29', { headers: h })).users;
  const g = users.find(u => u.nickname === 'Guest29');
  const zones = (await j(API + '/admin/zones', { headers: h })).zones;
  const std = zones.find(z => z.name === 'Standard'), vip = zones.find(z => z.name === 'VIP');
  const pcs = (await j(API + '/admin/computers', { headers: h })).computers;
  const pcVip = pcs.find(p => p.name === 'VIP-02');
  const { execSync } = require('child_process');
  const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
  for (const sid of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='${g.id}' AND status='active'`).split(' ').filter(Boolean)) {
    await j(API + '/admin/sessions/' + sid + '/end', { method: 'POST', headers: h, body: '{}' });
  }
  psql(`DELETE FROM minute_grants WHERE user_id='${g.id}'`);
  psql(`UPDATE users SET coin_minutes=7, coins_balance=5000, wallet_grosz=3000 WHERE id='${g.id}'`);
  psql(`DELETE FROM wallet_transactions WHERE user_id='${g.id}'`);
  const red = await j(API + '/admin/users/' + g.id + '/redeem', { method: 'POST', headers: h, body: JSON.stringify({ zone_id: std.id, minutes: 40 }) });
  check('подготовка: выдано 40 мин Standard', red.zone === 'Standard' && red.minutes === 40, JSON.stringify(red));
  const seat = await j(API + '/admin/computers/' + pcVip.id + '/session', { method: 'POST', headers: h, body: JSON.stringify({ guest: 'Guest29' }) });
  check('подготовка: Guest29 посажен за VIP-02', !!seat.session_id, JSON.stringify(seat));

  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } }); watch(page);
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await signIn(page, 'admin1', 'Admin12345'); // З19-и14: вход — когда запас лимитера стенда полный, дальше — когда вход дописан, а не пауза
  await page.evaluate(id => openUserDrawer(id), g.id);
  await sleep(900);
  const kv = await page.evaluate(() => document.querySelector('#drawerBody .kv').textContent.replace(/\s+/g, ' '));
  check('карточка: «Запас минут 40 мин Standard · 7 мин без зоны»', /Запас минут\s*40 мин Standard · 7 мин без зоны/.test(kv), kv.slice(0, 260));
  await page.screenshot({ path: OUT + '/zh2_card.png' });
  await page.evaluate(() => uFormToggle('redeem'));
  await sleep(600);
  const form = await page.evaluate(() => ({
    zone: redeemZone, on: document.querySelector('.uform .segs button.on') && document.querySelector('.uform .segs button.on').textContent,
    note: [...document.querySelectorAll('.uform .muted')].map(x => x.textContent.replace(/\s+/g, ' ')).join(' | '),
  }));
  check('форма выдачи: зона по умолчанию — VIP (гость за VIP-02)', form.zone === vip.id && /VIP/.test(form.on || ''), JSON.stringify(form).slice(0, 200));
  check('форма выдачи: пометка «минуты годны только в выбранной зоне … гость сейчас за VIP-02 — VIP»', /годны только в выбранной зоне/.test(form.note) && /VIP-02 — VIP/.test(form.note), form.note.slice(0, 200));
  await page.screenshot({ path: OUT + '/zh2_redeem.png' });
  check('js-ошибок нет (админ)', errors.length === 0, errors.join(' | '));
  // владелец: отчёт «Монеты»
  await page.evaluate(() => logout());
  await idle(page); // З19-и14: после записи — ждём страницу, а не паузу
  await signIn(page, 'boss', 'Boss12345');
  await page.evaluate(() => nav('stats'));
  await sleep(1200);
  await page.evaluate(() => { repTab = 'coins'; renderReports(); });
  await sleep(1500);
  const rep = await page.evaluate(() => { const k = [...document.querySelectorAll('#repBody .kpi')].find(x => /Должны временем/.test(x.textContent)); return k ? { text: k.textContent.replace(/\s+/g, ' '), title: k.title } : null; });
  check('отчёт «Монеты»: KPI «Должны временем» с разбивкой по зонам в подсказке', rep && /Standard 40 мин/.test(rep.title) && /без зоны — \d+ мин/.test(rep.title), JSON.stringify(rep));
  await page.screenshot({ path: OUT + '/zh2_coins.png' });
  check('js-ошибок нет (владелец)', errors.length === 0, errors.join(' | '));

  // PWA: блок «Минуты за монеты и компенсации» в истории
  const p2 = await browser.newPage({ viewport: { width: 420, height: 860 } }); watch(p2);
  const e2 = []; p2.on('pageerror', e => e2.push(e.message));
  await p2.goto(BASE + '/app', { waitUntil: 'load' });
  await sleep(800);
  await p2.fill('#nick', 'Guest29'); await p2.fill('#pass', 'Guest12345'); await p2.keyboard.press('Enter');
  await p2.waitForFunction(() => typeof me === 'object' && !!me, null, { timeout: 15000 }).catch(() => {}); await idle(p2); // З19-и14: вход гостя — по факту, а не паузой
  const pwa = await p2.evaluate(async () => { await loadPacks(); if (typeof renderHist === 'function') renderHist(); const el = document.getElementById('histCard'); return { rows: minutesByZone, html: el ? el.textContent.replace(/\s+/g, ' ') : '' }; });
  check('PWA: minutes_by_zone получен (Standard 40 + пул 7)', JSON.stringify(pwa.rows).includes('"minutes":40') && JSON.stringify(pwa.rows).includes('"from_pool":true'), JSON.stringify(pwa.rows));
  check('PWA: блок «Минуты за монеты и компенсации» с «40 мин · Standard» и «7 мин · везде»', /Минуты за монеты и компенсации/.test(pwa.html) && /Standard 40 мин/.test(pwa.html) && /7 мин · везде/.test(pwa.html), pwa.html.slice(0, 300));
  const pwaLang = await p2.evaluate(() => { const o = {}; for (const l of ['en', 'pl']) { STR = Object.assign({}, STR_RU, LANGS[l]); o[l] = reserveBlockHtml().replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim(); } STR = Object.assign({}, STR_RU, LANGS.ru); return o; });
  check('PWA en/pl: «40 min · Standard», «7 min · anywhere / wszędzie»', /Standard 40 min/.test(pwaLang.en) && /7 min · anywhere/.test(pwaLang.en) && /7 min · wszędzie/.test(pwaLang.pl), JSON.stringify(pwaLang).slice(0, 300));
  await p2.screenshot({ path: OUT + '/zh2_pwa.png', fullPage: false });
  check('js-ошибок нет (PWA)', e2.length === 0, e2.join(' | '));

  // Шелл: блок запаса словами зоны и прогноз без минут чужой зоны
  const p3 = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  const e3 = []; p3.on('pageerror', e => e3.push(e.message));
  await p3.goto(BASE + '/shell', { waitUntil: 'load' });
  await sleep(1500);
  const sh = await p3.evaluate(() => {
    minutesByZone = [{ zone: 'Standard', minutes: 40 }, { zone: '', minutes: 7, from_pool: true }];
    sessZone = 'VIP';
    const blockVip = reserveBlock().replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
    sessZone = 'Standard';
    const blockStd = reserveBlock().replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
    // прогноз: 30 zł на VIP (30 zł/ч → 60 мин) + пул 7 + zoneMinutesHere 0 = 67 мин; Standard-минуты не входят
    me = { wallet_grosz: 3000, coin_minutes: 7 }; activeSession = { effective_rate_pln: 30 }; zoneMinutesHere = 0; packMinutesHere = 0; bookingDeadline = null;
    renderWallet();
    const etaVip = document.getElementById('walletEta').textContent;
    zoneMinutesHere = 40; activeSession = { effective_rate_pln: 20 }; renderWallet(); // на Standard: 90 + 7 + 40 = 137 мин
    const etaStd = document.getElementById('walletEta').textContent;
    return { blockVip, blockStd, etaVip, etaStd, now: Date.now() };
  });
  check('шелл на VIP: «40 мин · Standard — в другой зоне, здесь не тратятся», «7 мин · везде»', /40 мин · Standard — в другой зоне, здесь не тратятся/.test(sh.blockVip) && /7 мин · везде/.test(sh.blockVip), sh.blockVip);
  check('шелл на Standard: «40 мин · Standard — в этой зоне»', /40 мин · Standard — в этой зоне/.test(sh.blockStd), sh.blockStd);
  const hm = (ms) => { const d = new Date(ms); return String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0'); };
  const near = (eta, mins) => [mins - 1, mins, mins + 1].some(m => eta.includes(hm(sh.now + m * 60000)));
  check(`шелл прогноз на VIP «хватит до» ≈ +67 мин (Standard-минуты не входят): ${sh.etaVip}`, near(sh.etaVip, 67), sh.etaVip);
  check(`шелл прогноз на Standard ≈ +137 мин (зонные минуты входят): ${sh.etaStd}`, near(sh.etaStd, 137), sh.etaStd);
  const shLang = await p3.evaluate(() => { const o = {}; for (const l of ['en', 'pl']) { STR = Object.assign({}, SH_RU, SH_LANGS[l]); sessZone = 'VIP'; o[l] = reserveBlock().replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim(); } STR = Object.assign({}, SH_RU, SH_LANGS.ru); return o; });
  check('шелл en/pl: «another zone, not spent here» / «w innej strefie, tu nie działają»', /another zone, not spent here/.test(shLang.en) && /w innej strefie, tu nie działają/.test(shLang.pl), JSON.stringify(shLang).slice(0, 300));
  check('js-ошибок нет (шелл)', e3.length === 0, e3.join(' | '));
  await browser.close();
  // уборка
  await j(API + '/admin/sessions/' + seat.session_id + '/seat-cancel', { method: 'POST', headers: h, body: '{}' });
  psql(`DELETE FROM minute_grants WHERE user_id='${g.id}'`);
  psql(`UPDATE users SET coin_minutes=0, coins_balance=5000, wallet_grosz=0 WHERE id='${g.id}'`);
  psql(`DELETE FROM wallet_transactions WHERE user_id='${g.id}'`);
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
