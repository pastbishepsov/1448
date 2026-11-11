// Ж6: playwright — корзина (плитки складывают, «Продать» одним чеком, чек одной строкой с
// раскрытием, «Вернуть чек»/позицию), «При выходе» в долг гостю за ПК, карточка «Должен …» +
// «Рассчитать», заказ кухни от стойки, сдача кассы при уходе (ожидаемая сумма, расхождение,
// без пересчёта), сводка смены и табель владельца с кассой.
const { chromium } = require('playwright-core');
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const { spawn, execSync } = require('child_process');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
async function j(url, opt) { const r = await fetch(url, opt); return r.json(); }
const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
(async () => {
  const login = async (l, p) => (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: l, password: p }) })).access_token;
  const a1 = await login('admin1', 'Admin12345'); const gt = await login('Guest29', 'Guest12345'); const ot = await login('boss', 'Boss12345');
  const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + a1 };
  const hg = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + gt };
  const ho = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + ot };
  const g = psql(`SELECT id FROM users WHERE nickname='Guest29'`);
  const pcs = (await j(API + '/admin/computers', { headers: h })).computers;
  const P = n => pcs.find(p => p.name === n);
  const endAll = async () => { for (const sid of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='${g}' AND status='active'`).split(' ').filter(Boolean)) await j(API + '/admin/sessions/' + sid + '/end', { method: 'POST', headers: h, body: '{}' }); };
  await endAll();
  await j(API + '/admin/users/' + g + '/settle', { method: 'POST', headers: ho, body: JSON.stringify({ method: 'card' }) }); // чистый старт: долгов нет
  psql(`UPDATE computers SET status='available' WHERE name='PC-09'`);
  psql(`UPDATE users SET wallet_grosz=3000, coin_minutes=0 WHERE id='${g}'`); psql(`DELETE FROM wallet_transactions WHERE user_id='${g}'`);
  const agent = spawn('node', [MOCK_AGENT, P('PC-09').id, agentLog('pw09')], { stdio: 'ignore', detached: true });
  await sleep(1200);
  await j(API + '/admin/computers/' + P('PC-09').id + '/session', { method: 'POST', headers: h, body: JSON.stringify({ guest_id: g, planned_min: 60 }) });
  await j(API + '/me/sessions/ready', { method: 'POST', headers: hg, body: '{}' });
  const stock = n => +psql(`SELECT stock FROM goods WHERE name='${n}'`);
  const woda0 = stock('Woda 0.5'), lays0 = stock(`Lay''s`);

  // трек З, З8-и1: касса — ящик клуба; на стенде без истории ящика первая сдача — нулевая точка, а проверке нужна
  // недостача — ящик заводит admin1 сдачей на 500 zł (вход в админку ниже снова откроет смену)
  await j(API + '/admin/work/start', { method: 'POST', headers: h, body: '{}' });
  if ((await j(API + '/admin/work/closeout', { headers: h })).baseline) await j(API + '/admin/work/stop', { method: 'POST', headers: h, body: JSON.stringify({ counted_pln: 500 }) });

  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(2500);

  // ── и2: корзина ──
  await page.evaluate(() => nav('goods'));
  await sleep(1500);
  await page.click('.gtile:has-text("Woda 0.5")'); await sleep(200);
  await page.click('.gtile:has-text("Woda 0.5")'); await sleep(200);
  await page.click('.gtile:has-text("Lay\'s")'); await sleep(300);
  const cart1 = await page.evaluate(() => ({ hidden: document.getElementById('cartBox').classList.contains('hidden'), rows: [...document.querySelectorAll('.cartrow')].map(r => r.textContent.replace(/\s+/g, ' ').trim()), total: document.getElementById('cartTotal').textContent, badge: (([...document.querySelectorAll('.gtile')].find(t => t.querySelector('b') && t.querySelector('b').textContent === 'Woda 0.5') || {}).querySelector ? [...document.querySelectorAll('.gtile')].find(t => t.querySelector('b').textContent === 'Woda 0.5').querySelector('.inq') : null) ? [...document.querySelectorAll('.gtile')].find(t => t.querySelector('b').textContent === 'Woda 0.5').querySelector('.inq').textContent : null, btn: document.getElementById('cartSellBtn').textContent.trim() }));
  const wodaPrice = +psql(`SELECT price_pln FROM goods WHERE name='Woda 0.5'`), laysPrice = +psql(`SELECT price_pln FROM goods WHERE name='Lay''s'`);
  const expTotal = (2 * wodaPrice + laysPrice).toFixed(2).replace('.', ',');
  check('корзина: две плитки Woda сложились (×2) + Lay\'s, итог = 2×Woda + Lay\'s, бейдж на плитке', !cart1.hidden && cart1.rows.length === 2 && /Woda 0.5.*2/.test(cart1.rows[0]) && cart1.total === expTotal + ' zł' && cart1.badge === '2' && /Продать/.test(cart1.btn), JSON.stringify(cart1));
  await page.click('.cartrow:has-text("Woda") .qty button:first-child'); await sleep(200); // −1
  const cart2 = await page.evaluate(() => [...document.querySelectorAll('.cartrow')].map(r => r.textContent.replace(/\s+/g, ' ').trim()));
  check('«−» в корзине: Woda ×1', /Woda 0.5.*·.*1/.test(cart2[0]) && !/2 /.test(cart2[0]), cart2.join(' | '));
  await page.click('.cartrow:has-text("Woda") .qty button:last-child'); await sleep(200); // +1 → 2
  await page.click('#cartSellBtn');
  await sleep(1500);
  const afterSell = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, cartHidden: document.getElementById('cartBox').classList.contains('hidden'), rows: [...document.querySelectorAll('#salesTable tr')].slice(1).map(r => r.textContent.replace(/\s+/g, ' ').trim()) }));
  check('«Продать» → тост «✓ чек из 2 позиций — сумма», корзина скрыта, склад Woda −2, Lay\'s −1', /чек из 2 позиций/.test(afterSell.toast) && afterSell.cartHidden && stock('Woda 0.5') === woda0 - 2 && stock(`Lay''s`) === lays0 - 1, afterSell.toast);
  check('«Продажи смены»: чек одной строкой «▸ Чек · 2 позиции Woda 0.5 ×2, Lay\'s» с «Вернуть чек»', afterSell.rows.some(r => /▸ Чек · 2 позиции/.test(r) && /Woda 0.5 ×2, Lay's/.test(r) && /Вернуть чек/.test(r)), afterSell.rows.slice(0, 2).join(' | '));
  await page.click('#salesTable tr:has-text("Чек · 2 позиции")');
  await sleep(400);
  const opened = await page.evaluate(() => ({ rcp: [...document.querySelectorAll('#salesTable tr.rcp')].map(r => r.textContent.replace(/\s+/g, ' ').trim()), head: (document.querySelector('#salesTable tr:has(b)') || {}).textContent }));
  check('клик по чеку раскрывает 2 позиции с кнопками «Вернуть»', opened.rcp.length === 2 && opened.rcp.every(r => /Вернуть/.test(r)), opened.rcp.join(' | '));
  // вернуть одну позицию (Lay's)
  await page.click('#salesTable tr.rcp:has-text("Lay\'s") button');
  await sleep(200);
  await page.click('#salesTable tr.rcp:has-text("Lay\'s") button');
  await sleep(1400);
  const afterPos = await page.evaluate(() => [...document.querySelectorAll('#salesTable tr')].slice(1).map(r => r.textContent.replace(/\s+/g, ' ').trim()));
  check('возврат позиции: Lay\'s на складе +1, чек помечен «часть отменена», Woda на месте', stock(`Lay''s`) === lays0 && stock('Woda 0.5') === woda0 - 2 && afterPos.some(r => /часть отменена/.test(r)), afterPos.slice(0, 3).join(' | '));
  await page.click('#salesTable tr:has-text("Чек · 2 позиции") button:has-text("Вернуть чек")');
  await sleep(200);
  await page.click('#salesTable tr:has-text("Чек · 2 позиции") .rfPick button.danger'); // З11-и2: «Вернуть N zł наличными» — как платили
  await sleep(1400);
  check('«Вернуть чек» → «Вернуть … наличными»: Woda вернулась на склад', stock('Woda 0.5') === woda0, String(stock('Woda 0.5')));

  // ── и1: «При выходе» — в долг гостю за ПК ──
  await page.click('#s-goods .segs button[data-m="later"]');
  await page.fill('#saleNick', 'Guest29'); await page.dispatchEvent('#saleNick', 'input');
  await page.click('.gtile:has-text("Woda 0.5")'); await sleep(300);
  const cartLater = await page.evaluate(() => ({ hint: document.getElementById('cartHint').textContent, btn: document.getElementById('cartSellBtn').textContent.trim() }));
  check('способ «При выходе»: подпись «в долг — укажи гостя…/рассчитается при выходе», кнопка «В долг»', /в долг/.test(cartLater.hint) && /В долг/.test(cartLater.btn), JSON.stringify(cartLater));
  await page.click('#cartSellBtn');
  await sleep(1500);
  const later = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, rows: [...document.querySelectorAll('#salesTable tr')].slice(1).map(r => r.textContent.replace(/\s+/g, ' ').trim()), total: document.getElementById('salesTotal').textContent }));
  check('в долг → тост «🧾 В долг: Woda — сумма · Guest29 · должен всего …», строка «в долг · рассчитать · Отменить» (З11-и4: «списать» — это списание долга владельцем), в итогах «в долг: … (не выручка)»', /В долг: Woda/.test(later.toast) && /должен всего/.test(later.toast) && later.rows.some(r => /Woda 0.5/.test(r) && /в долг/.test(r) && /рассчитать/.test(r) && /Отменить/.test(r)) && /в долг: .*не выручка/.test(later.total), later.toast + ' || ' + later.total);
  // ── и3: заказ кухни от стойки ──
  await page.evaluate(() => nav('kitchen'));
  await sleep(1200);
  await page.click('#s-kitchen button:has-text("Новый заказ")');
  await sleep(600);
  await page.fill('#kitGuest', 'Guest2'); await page.dispatchEvent('#kitGuest', 'input');
  await page.selectOption('#kitNew select', { label: /Lay's/.test((await page.locator('#kitNew select option').allTextContents()).join()) ? (await page.locator('#kitNew select option').allTextContents()).find(t => /Lay's/.test(t)) : undefined }).catch(() => {});
  await page.click('#kitNew button:has-text("Принять заказ")');
  await sleep(1400);
  const kCands = await page.evaluate(() => [...document.querySelectorAll('#kitCands .cands button b')].map(b => b.textContent));
  check('заказ кухни «Guest2» → кандидаты под полем', kCands.length >= 2 && kCands.includes('Guest29'), kCands.join(', '));
  await page.click('#kitCands .cands button:has-text("Guest29")');
  await sleep(1600);
  const kAfter = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, board: document.getElementById('kitBoard').textContent.replace(/\s+/g, ' ') }));
  check('выбор Guest29 → заказ принят: тост «🍔 … Guest29 · к PC-09 · при выходе (должен …)», карточка в очереди со статусом «принят»', /Guest29/.test(kAfter.toast) && /к PC-09/.test(kAfter.toast) && /при выходе/.test(kAfter.toast) && /принят/.test(kAfter.board) && /Guest29/.test(kAfter.board), kAfter.toast);
  await page.click('#kitBoard .card:has-text("Guest29") button:has-text("✓ Выдан")');
  await sleep(1400);
  const kUnpaid = await page.evaluate(() => document.getElementById('kitBoard').textContent.replace(/\s+/g, ' '));
  check('заказ выдан → «Ждут оплаты», ссылка «рассчитать всё в карточке»', /Ждут оплаты/.test(kUnpaid) && /рассчитать всё в карточке/.test(kUnpaid), kUnpaid.slice(0, 200));
  // ── карточка: «Должен …» и «Рассчитать» ──
  await page.click('#kitBoard a:has-text("рассчитать всё в карточке")');
  await sleep(1500);
  const card = await page.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, kv: document.querySelector('#drawerBody').textContent.replace(/\s+/g, ' '), btns: [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()) }));
  const debt = +psql(`SELECT COALESCE(SUM(total_pln),0) FROM sales WHERE user_id='${g}' AND paid_at IS NULL AND voided_at IS NULL`) + +psql(`SELECT COALESCE(SUM(total_pln),0) FROM kitchen_orders WHERE user_id='${g}' AND status='done' AND paid_at IS NULL`);
  const debtTxt = debt.toFixed(2).replace('.', ',');
  check('карточка Guest29: строка «Должен 🧾 X zł — кухня … · бар …», форма расчёта с позициями (🍔 и 🛒) и кнопкой «Рассчитать X zł · наличные»', card.title === 'Guest29' && new RegExp('Должен.*' + debtTxt.replace(',', '[,.]')).test(card.kv) && /кухня/.test(card.kv) && /бар/.test(card.kv) && /🍔/.test(card.kv) && /🛒/.test(card.kv) && card.btns.some(t => t.startsWith('Рассчитать ' + debtTxt) && /наличные/.test(t)), JSON.stringify({ title: card.title, debtTxt, btns: card.btns.filter(t => /Рассчитать/.test(t)) }));
  await page.click('#drawerBody .uform .segs button:has-text("Карта")');
  await sleep(300);
  const goodsBefore = (await j(API + '/admin/reports/money?preset=today', { headers: ho })).totals.goods_pln;
  await page.click('#drawerBody .uform button:has-text("Рассчитать")');
  await sleep(200);
  await page.click('#drawerBody .uform button:has-text("Ещё раз — рассчитать")');
  await sleep(1800);
  const settled = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, kv: document.querySelector('#drawerBody').textContent.replace(/\s+/g, ' ') }));
  const goodsAfter = (await j(API + '/admin/reports/money?preset=today', { headers: ho })).totals.goods_pln;
  check('«Рассчитать» картой: тост «✓ Guest29 рассчитался: X zł · карта · кухня … · бар …», строки «Должен» больше нет, выручка +X', /рассчитался: /.test(settled.toast) && /карта/.test(settled.toast) && /кухня/.test(settled.toast) && /бар/.test(settled.toast) && !/Должен/.test(settled.kv) && Math.abs(goodsAfter - goodsBefore - debt) < 0.005, settled.toast + ` | ${goodsBefore}→${goodsAfter} (+${debt})`);

  // ── и4: сдача кассы ──
  // в кассе admin1 должны быть наличные (свежие клубные сутки → ноль, а тест вычитает 20): депозит налом
  const depCash = await j(API + '/admin/users/' + g + '/deposit', { method: 'POST', headers: h, body: JSON.stringify({ amount_pln: 40, method: 'cash' }) });
  const prev = await j(API + '/admin/work/closeout', { headers: h });
  await page.evaluate(() => openMyShift());
  await sleep(1200);
  const shift = await page.evaluate(() => ({ kv: document.querySelector('#drawerBody').textContent.replace(/\s+/g, ' '), hasInput: !!document.getElementById('coCounted'), btns: [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()) }));
  const expTxt = (+prev.expected_pln).toFixed(2).replace('.', ',');
  check('«Моя смена»: «В ящике должно быть X zł» = /admin/work/closeout, разбивка, поле «Пересчитано», кнопки «Сдать смену» и «Уйти без пересчёта»', shift.kv.includes('В ящике должно быть' + expTxt) || new RegExp('В ящике должно быть\\s*' + expTxt.replace('.', '\\.')).test(shift.kv), shift.kv.slice(0, 260));
  check('поле пересчёта и обе кнопки на месте', shift.hasInput && shift.btns.includes('Сдать смену') && shift.btns.includes('Уйти без пересчёта') && /пополнения/.test(shift.kv) && /товары и кухня/.test(shift.kv), shift.btns.join(' | '));
  await page.fill('#coCounted', String((+prev.expected_pln - 20).toFixed(2)));
  await page.dispatchEvent('#coCounted', 'input');
  await sleep(200);
  const diffTxt = await page.evaluate(() => ({ diff: document.getElementById('coDiff').textContent, danger: document.getElementById('coSubmit').classList.contains('danger') }));
  check('ввод на 20 меньше → «Недостача −20,00 zł — уйдёт владельцу», кнопка красная', /Недостача −20,00 zł/.test(diffTxt.diff) && diffTxt.danger, JSON.stringify(diffTxt));
  await page.fill('#coNote', 'сдал смену Ане');
  const staffBefore = new Set((await j(API + '/admin/staff-chat', { headers: ho })).messages.map(m => m.id));
  await page.click('#coSubmit'); await sleep(200);
  await page.click('#drawerBody button:has-text("Ещё раз — сдать смену")');
  await sleep(1800);
  const closed = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, chip: document.getElementById('shiftTxt').textContent, open: document.getElementById('drawer').classList.contains('open') }));
  const staffAfter = (await j(API + '/admin/staff-chat', { headers: ho })).messages.filter(m => !staffBefore.has(m.id));
  check('сдача смены: тост «Смена закрыта · … · расхождение −20,00 zł — владелец увидит», чип «смена не открыта», шторка закрыта', /Смена закрыта/.test(closed.toast) && /расхождение −20,00 zł/.test(closed.toast) && /не открыта/.test(closed.chip) && !closed.open, JSON.stringify(closed));
  // трек З, З1-и2 (Д4): заметка — от admin1 всем, деньги — служебной строкой «только владельцу»
  check('владельцу в канал персонала: заметка «Передал смену: сдал смену Ане» и служебная строка с «расхождение −20.00 zł»', staffAfter.length === 2
    && staffAfter[0].kind === 'text' && staffAfter[0].text === 'Передал смену: сдал смену Ане'
    && staffAfter[1].kind === 'system' && staffAfter[1].visible_to === 'owner' && /расхождение −20\.00 zł/.test(staffAfter[1].text), JSON.stringify(staffAfter.map(m => m.text)));
  // владелец: сводка смены и табель
  await j(API + '/admin/work/start', { method: 'POST', headers: h, body: '{}' }); // admin1 обратно на смену (стенд)
  const page2 = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page2.on('pageerror', e => errors.push('owner: ' + e.message));
  await page2.goto(BASE + '/admin', { waitUntil: 'load' });
  await page2.fill('#nick', 'boss'); await page2.fill('#pass', 'Boss12345'); await page2.keyboard.press('Enter');
  await sleep(2500);
  await page2.evaluate(() => nav('stats'));
  await sleep(1500);
  const hasShiftTab = await page2.evaluate(() => typeof renderShift === 'function');
  // трек З, З19-и7: сдача — в сводке дня своей записи табеля (ночью по графику — день начала ночной смены): листаем к нему
  const coDay = psql(`SELECT date FROM shift_closeouts WHERE user_id = (SELECT id FROM users WHERE nickname = 'admin1') ORDER BY created_at DESC LIMIT 1`);
  if (hasShiftTab){ await page2.evaluate(d => { if (d) shiftDate = d; renderShift(); }, coDay); await sleep(1500); }
  const sh = await page2.evaluate(() => ({ co: (document.getElementById('shCloseouts') || {}).textContent, hidden: (document.getElementById('shCloseouts') || {classList:{contains:()=>true}}).classList.contains('hidden') }));
  check('сводка смены владельца: блок касс — admin1 с недостачей −20,00 zł и заметкой', !sh.hidden && /admin1/.test(sh.co || '') && /−20,00 zł/.test(sh.co || '') && /сдал смену Ане/.test(sh.co || ''), (sh.co || '').replace(/\s+/g, ' ').slice(0, 220));
  await page2.evaluate(() => nav('staff'));
  await sleep(1500);
  if (await page2.evaluate(() => typeof renderTimesheet === 'function')){ await page2.evaluate(() => renderTimesheet()); await sleep(1500); }
  const ts = await page2.evaluate(() => (document.getElementById('tsTable') || {}).textContent.replace(/\s+/g, ' '));
  check('табель владельца: у записи admin1 «касса −20,00 zł»', /admin1/.test(ts) && /касса −20,00 zł/.test(ts), (ts || '').slice(0, 200));

  check('js-ошибок нет', errors.length === 0, errors.join(' | '));
  await browser.close();
  // уборка
  if (depCash && depCash.deposit_id) await j(API + '/admin/deposits/' + depCash.deposit_id + '/void', { method: 'POST', headers: ho, body: '{}' });
  await endAll();
  await j(API + '/admin/users/' + g + '/settle', { method: 'POST', headers: ho, body: JSON.stringify({ method: 'card' }) });
  psql(`UPDATE computers SET status='available' WHERE name='PC-09'`);
  // трек З, З14-и2а: «в долг» (Woda) и заказ кухни (Lay's) рассчитаны картой и остаются продажами — склад они уносят
  // насовсем. Lay's докладывает до 30 e2e_zh6, Woda — никто: каждый прогон оставлял на стенде на одну меньше, 17.10 она
  // кончилась и уронила e2e_z6_kitchen. Остатки — как до проверки, корректировкой владельца (не SQL: партии, З10-и3);
  // продажи и выручка дня — как были.
  for (const [n, s0] of [['Woda 0.5', woda0], [`Lay''s`, lays0]]) {
    const cur = stock(n);
    if (cur !== s0) await j(API + '/admin/goods/' + psql(`SELECT id FROM goods WHERE name='${n}'`) + '/stock', { method: 'POST', headers: ho,
      body: JSON.stringify({ delta: s0 - cur, reason: 'adjust', note: 'pw_zh6 уборка: остаток как до проверки' }) });
  }
  check('уборка: остаток «Woda 0.5» и «Lay\'s» — как до проверки (проданное в долг и заказ кухни склад стенда не уносят)',
    stock('Woda 0.5') === woda0 && stock(`Lay''s`) === lays0, `Woda ${stock('Woda 0.5')} из ${woda0} · Lay's ${stock(`Lay''s`)} из ${lays0}`);
  try { process.kill(-agent.pid); } catch (e) { try { agent.kill(); } catch (_) {} }
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
