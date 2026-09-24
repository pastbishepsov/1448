// Ж5: playwright — шторка ПК «Деньги/Хватит до» (= прогноз гостя ±1 мин), «Пополнить» → карточка
// с депозитом, карточка «Сейчас за PC-09» + «Завершить сессию», «Продать» → Товары с гостем,
// «Посадить за…» из карточки, таблица гостей без персонала, фильтры/сортировка, очередь по телефону
// и выбор кандидата, продажа с кандидатами.
const { chromium } = require('playwright-core');
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const { spawn, execSync } = require('child_process');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
async function j(url, opt) { const r = await fetch(url, opt); return r.json(); }
const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
const hhmm = d => String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
(async () => {
  const login = async (l, p) => (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: l, password: p }) })).access_token;
  const a1 = await login('admin1', 'Admin12345'); const gt = await login('Guest29', 'Guest12345');
  const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + a1 };
  const hg = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + gt };
  const g = psql(`SELECT id FROM users WHERE nickname='Guest29'`), g28 = psql(`SELECT id FROM users WHERE nickname='Guest28'`);
  const pcs = (await j(API + '/admin/computers', { headers: h })).computers;
  const P = n => pcs.find(p => p.name === n);
  const endAll = async () => { for (const sid of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id IN ('${g}','${g28}') AND status='active'`).split(' ').filter(Boolean)) await j(API + '/admin/sessions/' + sid + '/end', { method: 'POST', headers: h, body: '{}' }); };
  // чистый старт
  await endAll();
  psql(`UPDATE computers SET status='available', hold_until=NULL WHERE name IN ('PC-08','PC-09')`);
  psql(`UPDATE users SET wallet_grosz=3000, coin_minutes=0 WHERE id='${g}'`); psql(`DELETE FROM wallet_transactions WHERE user_id='${g}'`); psql(`DELETE FROM minute_grants WHERE user_id='${g}'`);
  psql(`UPDATE waitlist SET status='removed', resolved_at=now() WHERE status='waiting'`);
  const agent = spawn('node', [MOCK_AGENT, P('PC-09').id, agentLog('pw09')], { stdio: 'ignore', detached: true });
  await sleep(1200);
  // Guest29 играет за PC-09: посадка + [Готов!]
  const seat = await j(API + '/admin/computers/' + P('PC-09').id + '/session', { method: 'POST', headers: h, body: JSON.stringify({ guest_id: g, planned_min: 60 }) });
  await j(API + '/me/sessions/ready', { method: 'POST', headers: hg, body: '{}' });
  await sleep(800);

  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  await page.evaluate(() => nav('hall'));
  await sleep(1500);

  // ── и1: шторка PC-09 — деньги и «хватит до» ──
  await page.evaluate(id => openPCDrawer(id), P('PC-09').id);
  await sleep(500);
  const drawer = await page.evaluate(() => ({ kv: document.querySelector('#drawerBody').textContent.replace(/\s+/g, ' '), until: (document.getElementById('dUntil') || {}).textContent, btns: [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()) }));
  check('шторка ПК: строки «Деньги» (20,00 zł/ч · кошелёк 30,00 zł) и «Хватит»', /Деньги/.test(drawer.kv) && /20,00 zł\/ч/.test(drawer.kv) && /кошелёк 30,00 zł/.test(drawer.kv) && /Хватит/.test(drawer.kv), drawer.kv.slice(0, 260));
  // прогноз гостя: как renderWallet в шелле — монеты + зонные + пакет + floor(кошелёк*60/ставка)
  const me = await j(API + '/me', { headers: hg }); const ms = await j(API + '/me/sessions', { headers: hg });
  const act = ms.sessions.find(s => s.status === 'active');
  const left = (me.coin_minutes || 0) + (ms.zone_minutes || 0) + (ms.pack_minutes || 0) + Math.floor(me.wallet_grosz * 60 / Math.round(act.effective_rate_pln * 100));
  const tillGuest = new Date(Date.now() + left * 60000);
  const m = /до (\d\d:\d\d)/.exec(drawer.until || '');
  const cand = [new Date(tillGuest.getTime() - 60000), tillGuest, new Date(tillGuest.getTime() + 60000)].map(hhmm);
  check('«хватит до HH:MM» в шторке = прогноз гостя (/me + /me/sessions) ±1 мин', m && cand.includes(m[1]), `шторка ${m && m[1]} · гость ${hhmm(tillGuest)} (ещё ${left} мин)`);
  check('шторка ПК: кнопки «Пополнить», «Пакет», «Продать», ссылка «карточка»', drawer.btns.includes('Пополнить') && drawer.btns.includes('Пакет') && drawer.btns.includes('Продать') && /карточка/.test(drawer.kv), drawer.btns.join(' | '));
  await page.click('#drawerBody button:has-text("Пополнить")');
  await sleep(900);
  const dep = await page.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, hasSum: !!document.getElementById('d_sum'), now: (document.querySelector('#drawerBody .kv') || {}).textContent.replace(/\s+/g, ' ') }));
  check('«Пополнить» → карточка Guest29 с открытой формой депозита', dep.title === 'Guest29' && dep.hasSum, JSON.stringify({ title: dep.title, hasSum: dep.hasSum }));
  // ── и2: карточка — «Сейчас за PC-09 · … хватит до», «Завершить сессию за PC-09» ──
  check('карточка: строка «Сейчас за PC-09 · Standard · идёт … до HH:MM»', /Сейчас\s*за PC-09/.test(dep.now) && /Standard/.test(dep.now) && /идёт/.test(dep.now) && /до \d\d:\d\d/.test(dep.now), dep.now.slice(0, 200));
  const cardBtns = await page.evaluate(() => [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()));
  check('карточка: «Завершить сессию за PC-09», «Продать», без «Посадить за…» (гость уже сидит)', cardBtns.some(t => /Завершить сессию за PC-09/.test(t)) && cardBtns.includes('Продать') && !cardBtns.some(t => /Посадить за/.test(t)), cardBtns.join(' | '));
  // «Продать» → Товары с гостем
  await page.click('#drawerBody button:has-text("Продать")');
  await sleep(1200);
  const goods = await page.evaluate(() => ({ tab: curTab, nick: document.getElementById('saleNick').value, chip: document.getElementById('saleGuestChip').textContent, hidden: document.getElementById('saleGuestChip').classList.contains('hidden'), sg: saleGuest && saleGuest.nickname }));
  check('«Продать» → раздел Товары, в поле «кому» Guest29, чип ✓ Guest29', goods.tab === 'goods' && goods.nick === 'Guest29' && /Guest29/.test(goods.chip) && !goods.hidden && goods.sg === 'Guest29', JSON.stringify(goods));
  // продажа плиткой уходит на выбранного гостя по id
  const stockBefore = +psql(`SELECT stock FROM goods WHERE name='Woda 0.5'`);
  await page.click('.gtile:has-text("Woda 0.5")'); await sleep(300); // Ж6: плитка кладёт в чек, продаёт кнопка
  await page.click('#cartSellBtn');
  await sleep(1200);
  const saleRow = psql(`SELECT u.nickname FROM sales s JOIN users u ON u.id=s.user_id WHERE s.name='Woda 0.5' ORDER BY s.created_at DESC LIMIT 1`);
  check('плитка «Woda 0.5» → продажа записана на Guest29', saleRow === 'Guest29' && +psql(`SELECT stock FROM goods WHERE name='Woda 0.5'`) === stockBefore - 1, saleRow);
  const lastSale = psql(`SELECT id FROM sales WHERE name='Woda 0.5' ORDER BY created_at DESC LIMIT 1`);
  await j(API + '/admin/sales/' + lastSale + '/void', { method: 'POST', headers: h, body: '{}' });
  // кандидаты в продаже: «Guest2» → список, выбор → продажа
  await page.fill('#saleNick', 'Guest2');
  await page.dispatchEvent('#saleNick', 'input');
  await sleep(200);
  const chipAfterType = await page.evaluate(() => document.getElementById('saleGuestChip').classList.contains('hidden'));
  check('напечатали другое — чип выбранного гостя снят', chipAfterType, '');
  await page.click('.gtile:has-text("Woda 0.5")'); await sleep(300); // Ж6: в чек, затем «Продать»
  await page.click('#cartSellBtn');
  await sleep(1200);
  const cands = await page.evaluate(() => [...document.querySelectorAll('#saleCands .cands button b')].map(b => b.textContent));
  check('продажа «Guest2» → кандидаты под тулбаром (2–5)', cands.length >= 2 && cands.length <= 5, cands.join(', '));
  await page.click('#saleCands .cands button:has-text("Guest28")');
  await sleep(1400);
  const afterPick = await page.evaluate(() => ({ chip: document.getElementById('saleGuestChip').textContent, cands: document.getElementById('saleCands').innerHTML.length, toast: document.getElementById('toast').textContent }));
  const saleRow2 = psql(`SELECT u.nickname FROM sales s JOIN users u ON u.id=s.user_id WHERE s.name='Woda 0.5' AND s.voided_at IS NULL ORDER BY s.created_at DESC LIMIT 1`);
  check('выбор Guest28 → продажа записана на него, чип ✓ Guest28, список кандидатов убран', saleRow2 === 'Guest28' && /Guest28/.test(afterPick.chip) && afterPick.cands === 0, JSON.stringify({ saleRow2, chip: afterPick.chip, toast: afterPick.toast }));
  const lastSale2 = psql(`SELECT id FROM sales WHERE name='Woda 0.5' AND voided_at IS NULL ORDER BY created_at DESC LIMIT 1`);
  await j(API + '/admin/sales/' + lastSale2 + '/void', { method: 'POST', headers: h, body: '{}' });

  // ── и3: таблица гостей ──
  await page.evaluate(() => nav('users'));
  await sleep(1500);
  const tbl = await page.evaluate(() => ({ heads: [...document.querySelectorAll('#usersTable th')].map(t => t.textContent.trim()), rows: [...document.querySelectorAll('#usersTable tr')].slice(1).map(r => r.children[0].textContent.trim()), txt: document.getElementById('usersTable').textContent }));
  check('таблица: колонки Ник · LVL · Кошелёк · Сейчас · Телефон · Монеты · Статус · Был активен', ['Ник', 'LVL', 'Кошелёк', 'Сейчас', 'Телефон', 'Монеты', 'Статус'].every(k => tbl.heads.some(hh => hh.startsWith(k))) && tbl.heads.some(hh => hh.startsWith('Был активен')), tbl.heads.join(' | '));
  check('персонала в списке нет (admin1, admin2, boss)', !tbl.rows.includes('admin1') && !tbl.rows.includes('admin2') && !tbl.rows.includes('boss') && tbl.rows.length > 5, tbl.rows.length + ' строк');
  check('«Сейчас» у Guest29: PC-09 · до HH:MM', /Guest29[\s\S]*PC-09[\s\S]*до \d\d:\d\d/.test(await page.evaluate(() => [...document.querySelectorAll('#usersTable tr')].find(r => /Guest29/.test(r.children[0].textContent)).textContent.replace(/\s+/g, ' '))), '');
  await page.click('#uFilters button:has-text("Сейчас за ПК")');
  await sleep(1200);
  const atPc = await page.evaluate(() => ({ rows: [...document.querySelectorAll('#usersTable tr')].slice(1).map(r => r.children[0].textContent.trim()), on: document.querySelector('#uFilters button.on').textContent, f: userFilter }));
  check('чип «Сейчас за ПК»: только Guest29, чип подсвечен, filter=at_pc', atPc.rows.length === 1 && atPc.rows[0] === 'Guest29' && atPc.on === 'Сейчас за ПК' && atPc.f === 'at_pc', JSON.stringify(atPc));
  await page.click('#uFilters button:has-text("С деньгами")');
  await sleep(1200);
  await page.click('#usersTable th:has-text("Кошелёк")');
  await sleep(1200);
  const byWallet = await page.evaluate(() => ({ vals: [...document.querySelectorAll('#usersTable tr')].slice(1).map(r => parseFloat(r.children[2].textContent.replace(',', '.'))), th: document.querySelector('#usersTable th.on').textContent.trim(), sort: userSort, dir: userDir }));
  const desc = byWallet.vals.every((v, i, a) => i === 0 || a[i - 1] >= v);
  check('чип «С деньгами» + клик по «Кошелёк»: сервер сортирует по убыванию, стрелка ↓ в заголовке', desc && byWallet.vals.length > 3 && byWallet.vals.every(v => v > 0) && /Кошелёк ↓/.test(byWallet.th) && byWallet.sort === 'wallet', JSON.stringify({ first: byWallet.vals.slice(0, 4), th: byWallet.th }));
  await page.click('#usersTable th:has-text("Кошелёк")');
  await sleep(1200);
  const asc = await page.evaluate(() => ({ vals: [...document.querySelectorAll('#usersTable tr')].slice(1).map(r => parseFloat(r.children[2].textContent.replace(',', '.'))), th: document.querySelector('#usersTable th.on').textContent.trim() }));
  check('второй клик — по возрастанию, стрелка ↑', asc.vals.every((v, i, a) => i === 0 || a[i - 1] <= v) && /Кошелёк ↑/.test(asc.th), asc.th);
  await page.click('#uFilters button:has-text("Новые за неделю")');
  await sleep(1200);
  const fresh = await page.evaluate(() => ({ th: [...document.querySelectorAll('#usersTable th')].map(t => t.textContent.trim()), f: userFilter, sort: userSort }));
  check('чип «Новые за неделю»: заголовок «Аккаунт с», сортировка админа (кошелёк) сохранилась', fresh.f === 'new' && fresh.th.some(t => t.startsWith('Аккаунт с')) && fresh.sort === 'wallet', JSON.stringify(fresh));
  await page.click('#uFilters button:has-text("Все")');
  await sleep(800);

  // ── и2: «Посадить за…» из карточки (сессию Guest29 завершим кнопкой карточки) ──
  await page.evaluate(id => openUserDrawer(id), g);
  await sleep(1200);
  await page.click('#drawerBody button:has-text("Завершить сессию за PC-09")');
  await sleep(200);
  await page.click('#drawerBody button:has-text("Ещё раз — завершить")');
  await sleep(1500);
  const afterEnd = await page.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, now: /Сейчас за/.test(document.querySelector('#drawerBody').textContent), btns: [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()), toast: document.getElementById('toast').textContent }));
  check('завершение из карточки: карточка осталась открытой, строки «Сейчас за» нет, появилась «Посадить за…»', afterEnd.title === 'Guest29' && !afterEnd.now && afterEnd.btns.some(t => /Посадить за…/.test(t)) && /Guest29: \+\d+ XP/.test(afterEnd.toast), JSON.stringify({ title: afterEnd.title, now: afterEnd.now, toast: afterEnd.toast }));
  await page.click('#drawerBody button:has-text("Посадить за…")');
  await sleep(1200);
  const seatForm = await page.evaluate(() => ({ opts: [...document.querySelectorAll('#seatPCSel option')].map(o => o.textContent), chips: [...document.querySelectorAll('#drawerBody .uform .segs button')].map(b => b.textContent) }));
  check('форма «Посадить за…»: селект свободных ПК (есть PC-09), пресеты 30/60/120/180', seatForm.opts.some(o => /PC-09/.test(o)) && seatForm.opts.length >= 5 && seatForm.chips.join(',') === '30 мин,60 мин,120 мин,180 мин', seatForm.opts.length + ' ПК');
  await page.selectOption('#seatPCSel', P('PC-09').id);
  await sleep(400);
  await page.click('#drawerBody .uform .segs button:has-text("120 мин")');
  await sleep(400);
  const seatReq = [];
  page.on('request', r => { if (r.method() === 'POST' && /\/admin\/computers\/[^/]+\/session$/.test(r.url())) seatReq.push(r.postData()); });
  await page.click('#drawerBody button:has-text("Посадить за PC-09")');
  await sleep(1800);
  const seated = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, now: (document.querySelector('#drawerBody .kv') || {}).textContent.replace(/\s+/g, ' '), title: document.getElementById('drawerTitle').textContent }));
  const planned = seatReq.length ? String((JSON.parse(seatReq[0]) || {}).planned_min) : '';
  check('посадка из карточки: тост «▶ Guest29 за «PC-09»», строка «Сейчас за PC-09 · ⏳ ждёт [Готов!]», planned_min 120', /Guest29 за «PC-09»/.test(seated.toast) && /Сейчас\s*за PC-09/.test(seated.now) && /ждёт \[Готов!\]/.test(seated.now) && planned === '120' && seatReq.length === 1 && /"guest_id"/.test(seatReq[0]), JSON.stringify({ toast: seated.toast, planned }));

  // ── и4: очередь по телефону и кандидаты ──
  const freeIds = psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM computers WHERE status='available'`).split(' ').filter(Boolean);
  psql(`UPDATE computers SET status='maintenance' WHERE status='available'`);
  try {
    await page.evaluate(() => nav('hall'));
    await sleep(1500);
    // тосты — списком (трек З, З2-и1): живое событие очереди обновляет раздел, и тост лимитера
    // «Слишком много запросов» ложился поверх «в очереди — №1», хотя гость в очередь уже встал
    await page.evaluate(() => { window.__toasts = []; const t = toast; toast = (msg, ...a) => { __toasts.push(String(msg)); return t(msg, ...a); }; });
    await page.fill('#wlNick', '+48 500 120 028');
    await page.press('#wlNick', 'Enter');
    await sleep(1500);
    const wl1 = await page.evaluate(() => ({ list: document.getElementById('wlList').textContent.replace(/\s+/g, ' '), toast: __toasts.join(' | '), inp: document.getElementById('wlNick').value }));
    check('очередь по телефону: Guest28 в списке, тост «⏳ Guest28 в очереди — №1», поле очищено', /Guest28/.test(wl1.list) && /Guest28 в очереди — №1/.test(wl1.toast) && wl1.inp === '', JSON.stringify(wl1));
    await page.fill('#wlNick', 'Guest2');
    await page.press('#wlNick', 'Enter');
    await sleep(1500);
    const wlCands = await page.evaluate(() => [...document.querySelectorAll('#wlCands .cands button b')].map(b => b.textContent));
    check('очередь «Guest2» → выбор кандидатов под полем (2–5)', wlCands.length >= 2 && wlCands.length <= 5, wlCands.join(', '));
    await page.evaluate(() => { __toasts = []; });
    await page.click('#wlCands .cands button:has-text("Guest27")');
    await sleep(1500);
    const wl2 = await page.evaluate(() => ({ list: document.getElementById('wlList').textContent.replace(/\s+/g, ' '), cands: document.getElementById('wlCands').innerHTML.length, toast: __toasts.join(' | ') }));
    check('выбор Guest27 → в очереди №2, кандидаты убраны', /Guest27/.test(wl2.list) && wl2.cands === 0 && /Guest27 в очереди — №2/.test(wl2.toast), JSON.stringify(wl2));
  } finally {
    if (freeIds.length) psql(`UPDATE computers SET status='available' WHERE id IN (${freeIds.map(i => `'${i}'`).join(',')})`);
    psql(`UPDATE waitlist SET status='removed', resolved_at=now() WHERE status='waiting'`);
  }

  check('js-ошибок нет', errors.length === 0, errors.join(' | '));
  await browser.close();
  // уборка
  await endAll();
  psql(`UPDATE computers SET status='available', hold_until=NULL WHERE name IN ('PC-08','PC-09')`);
  try { process.kill(-agent.pid); } catch (e) { try { agent.kill(); } catch (_) {} }
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
