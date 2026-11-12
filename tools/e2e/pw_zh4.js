// Ж4: playwright — форма посадки («на сколько», пометка про ПК без связи), плитка ⏻ с отсчётом
// второй строкой, шторка ожидания шелла, «⚠ занят без сессии» + «Освободить ПК» arm-паттерном,
// «Продлить» у придержания, рамка «зовёт» по ПК гостя, легенда, схема ужимается при шторке.
const { chromium } = require('playwright-core');
const { toastAfter } = require('./_z'); // З19-и13: тост этого действия, а не #toast через паузу
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const { spawn, execSync } = require('child_process');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
async function j(url, opt) { const r = await fetch(url, opt); return r.json(); }
const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
(async () => {
  const a1 = (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: 'admin1', password: 'Admin12345' }) })).access_token;
  const gt = (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: 'Guest29', password: 'Guest12345' }) })).access_token;
  const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + a1 };
  const g = psql(`SELECT id FROM users WHERE nickname='Guest29'`);
  const pcs = (await j(API + '/admin/computers', { headers: h })).computers;
  const P = n => pcs.find(p => p.name === n);
  // чистый старт
  for (const sid of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='${g}' AND status='active'`).split(' ').filter(Boolean)) await j(API + '/admin/sessions/' + sid + '/end', { method: 'POST', headers: h, body: '{}' });
  psql(`UPDATE computers SET status='available', hold_until=NULL WHERE name IN ('PC-06','PC-07','PC-08','PC-09')`);
  psql(`UPDATE users SET wallet_grosz=3000 WHERE id='${g}'`); psql(`DELETE FROM wallet_transactions WHERE user_id='${g}'`);
  psql(`UPDATE chat_messages SET read_staff=true WHERE user_id='${g}'`);
  const agent = spawn('node', [MOCK_AGENT, P('PC-09').id, agentLog('pw09')], { stdio: 'ignore', detached: true });
  await sleep(1200);
  psql(`UPDATE computers SET status='in_session' WHERE name='PC-07'`); // залипший

  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  await page.evaluate(() => nav('hall'));
  await sleep(1500);
  // легенда
  const legend = await page.evaluate(() => document.querySelector('#s-hall .muted[style*="flex-wrap"]').textContent.replace(/\s+/g, ' '));
  check('легенда: ⏳ ждёт [Готов!] · ⏻ просыпается · 📝 регистрация · ⚠ без сессии', /⏳ ждёт \[Готов!\]/.test(legend) && /⏻ просыпается/.test(legend) && /📝 регистрация/.test(legend) && /⚠ без сессии/.test(legend), legend.slice(0, 200));
  // залипший PC-07
  const stuck = await page.evaluate(() => { const c = [...document.querySelectorAll('.hpc')].find(x => x.querySelector('.nm').textContent === 'PC-07'); return { cls: c.className, txt: c.textContent.replace(/\s+/g, ' ') }; });
  check('плитка PC-07: «⚠ занят / без сессии», класс stuck', /stuck/.test(stuck.cls) && /⚠ занят/.test(stuck.txt) && /без сессии/.test(stuck.txt), JSON.stringify(stuck));
  await page.evaluate(id => openPCDrawer(id), P('PC-07').id);
  await sleep(500);
  const layout = await page.evaluate(() => ({ open: document.body.classList.contains('drawer-open'), pad: getComputedStyle(document.querySelector('.content')).paddingRight }));
  check('открытая шторка: body.drawer-open, у .content padding-right 424px', layout.open && layout.pad === '424px', JSON.stringify(layout));
  const relBtn = await page.locator('#drawerBody button:has-text("Освободить ПК")').count();
  check('шторка залипшего ПК: кнопка «Освободить ПК»', relBtn === 1, String(relBtn));
  await page.click('#drawerBody button:has-text("Освободить ПК")');
  await sleep(200);
  const armed = await page.evaluate(() => [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()).find(t => /Ещё раз — освободить/.test(t)));
  check('первый клик взводит: «Ещё раз — освободить»', !!armed, String(armed));
  await toastAfter(page, () => page.click('#drawerBody button:has-text("Ещё раз — освободить")'), 'освобождён'); // снимок ниже — с этим тостом
  const afterRel = await page.evaluate(() => ({ toast: document.getElementById('toast').textContent, st: (hallData.find(p => p.name === 'PC-07') || {}).status, stuck: (hallData.find(p => p.name === 'PC-07') || {}).stuck, open: document.body.classList.contains('drawer-open') }));
  check('второй клик: PC-07 освобождён, тост, шторка закрыта', /освобождён/.test(afterRel.toast) && afterRel.st === 'available' && !afterRel.stuck && !afterRel.open, JSON.stringify(afterRel));
  // форма посадки на PC-08 (офлайн)
  await page.evaluate(id => openPCDrawer(id), P('PC-08').id);
  await sleep(500);
  const form = await page.evaluate(() => ({ chips: [...document.querySelectorAll('#drawerBody .uform .segs button')].map(b => b.textContent), note: [...document.querySelectorAll('#drawerBody .uform .muted')].map(x => x.textContent.replace(/\s+/g, ' ')).join(' | '), plan: seatPlan }));
  check('форма посадки: пресеты 30/60/120/180, по умолчанию 30', form.chips.join(',') === '30 мин,60 мин,120 мин,180 мин' && form.plan === 30, JSON.stringify(form.chips));
  check('форма посадки на ПК без связи: пометка «⏻ ПК не на связи — включу по сети и подожду его 7 мин»', /ПК не на связи — включу по сети и подожду его 7 мин/.test(form.note), form.note.slice(0, 200));
  await page.click('#drawerBody .uform .segs button:has-text("60 мин")');
  await sleep(300);
  await page.fill('#seatNick', 'Guest29');
  const seatToast = await toastAfter(page, () => page.click('#drawerBody button:has-text("Начать сессию")'), 'Guest29 за');
  check('тост посадки честный: «⏻ Guest29 за «PC-08»: ПК не на связи — ждём его 7 мин … WoL»', /Guest29 за «PC-08»/.test(seatToast) && /ждём его 7 мин/.test(seatToast) && /WoL/.test(seatToast), seatToast);
  await page.evaluate(() => closeDrawer());
  await sleep(800);
  const tile8 = await page.evaluate(() => { const c = [...document.querySelectorAll('.hpc')].find(x => x.querySelector('.nm').textContent === 'PC-08'); return { cls: c.className, inf: c.querySelector('.inf').textContent, inf2: c.querySelector('.inf2') && c.querySelector('.inf2').textContent }; });
  check('плитка PC-08: «⏻ Guest29» + вторая строка «m:ss», класс wake', /wake/.test(tile8.cls) && /⏻ Guest29/.test(tile8.inf) && /^\d+:\d\d$/.test(tile8.inf2 || ''), JSON.stringify(tile8));
  const planned = psql(`SELECT COUNT(*) FROM admin_actions WHERE action='session_start' AND details LIKE '%Guest29 за PC-08%WoL: sent%' AND created_at > now() - interval '1 minute'`);
  check('журнал: посадка с WoL: sent', +planned >= 1, planned);
  await page.screenshot({ path: OUT + '/zh4_hall.png' });
  await page.evaluate(id => openPCDrawer(id), P('PC-08').id);
  await sleep(500);
  const dr8 = await page.evaluate(() => document.getElementById('drawerBody').textContent.replace(/\s+/g, ' '));
  check('шторка PC-08: «⏻ ждёт включения ПК — m:ss · деньги не идут», «Отменить посадку»', /ждёт включения ПК — \d+:\d\d · деньги не идут/.test(dr8) && /Отменить посадку/.test(dr8), dr8.slice(0, 220));
  await page.screenshot({ path: OUT + '/zh4_drawer.png' });
  await page.evaluate(() => closeDrawer());
  // придержание с «Продлить»
  await page.evaluate(id => openPCDrawer(id), P('PC-06').id);
  await sleep(400);
  await page.click('#drawerBody button:has-text("Занять под регистрацию")');
  await sleep(1200);
  await page.evaluate(id => openPCDrawer(id), P('PC-06').id);
  await sleep(500);
  const holdBtns = await page.evaluate(() => [...document.querySelectorAll('#drawerBody button')].map(b => b.textContent.trim()));
  check('придержанный ПК: кнопки «Продлить» и «Снять придержание»', holdBtns.some(t => /Продлить/.test(t)) && holdBtns.some(t => /Снять придержание/.test(t)), holdBtns.join(' | '));
  const holdBefore = psql(`SELECT EXTRACT(EPOCH FROM hold_until)::int FROM computers WHERE name='PC-06'`);
  await sleep(1100);
  const holdToast = await toastAfter(page, () => page.click('#drawerBody button:has-text("Продлить")'), 'продлено');
  const holdAfter = psql(`SELECT EXTRACT(EPOCH FROM hold_until)::int FROM computers WHERE name='PC-06'`);
  check('«Продлить»: срок сдвинулся вперёд, шторка осталась, тост «продлено»', +holdAfter > +holdBefore && /продлено/.test(holdToast) && (await page.evaluate(() => drawerPCId)) === P('PC-06').id, `${holdBefore} → ${holdAfter} · ${holdToast}`);
  await page.evaluate(() => closeDrawer());
  await j(API + '/admin/computers/' + P('PC-06').id + '/hold', { method: 'DELETE', headers: h });
  // рамка «зовёт» — по ПК гостя: снимаем посадку с PC-08, сажаем на PC-09 (агент онлайн), гость зовёт
  const s8 = psql(`SELECT id FROM sessions WHERE user_id='${g}' AND status='active'`);
  await j(API + '/admin/sessions/' + s8 + '/seat-cancel', { method: 'POST', headers: h, body: '{}' });
  await j(API + '/admin/computers/' + P('PC-09').id + '/session', { method: 'POST', headers: h, body: JSON.stringify({ guest: 'Guest29' }) });
  psql(`UPDATE chat_messages SET created_at = created_at - interval '2 minutes' WHERE user_id='${g}'`);
  await j(API + '/me/chat', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + gt }, body: JSON.stringify({ kind: 'call' }) });
  await sleep(1500);
  await page.evaluate(async () => { await loadBell(); await renderHall(); });
  await sleep(500);
  const call = await page.evaluate(id => ({ has: callPCs.has(id), cls: ([...document.querySelectorAll('.hpc')].find(x => x.querySelector('.nm').textContent === 'PC-09') || {}).className }), P('PC-09').id);
  check('рамка «зовёт» на PC-09 — ПК, где гость сидит сейчас', call.has && /call/.test(call.cls || ''), JSON.stringify(call));
  await page.screenshot({ path: OUT + '/zh4_call.png' });
  check('js-ошибок нет', errors.length === 0, errors.join(' | '));
  await browser.close();
  // уборка
  for (const sid of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='${g}' AND status='active'`).split(' ').filter(Boolean)) await j(API + '/admin/sessions/' + sid + '/end', { method: 'POST', headers: h, body: '{}' });
  psql(`UPDATE chat_messages SET read_staff=true WHERE user_id='${g}'`);
  psql(`UPDATE computers SET status='available', hold_until=NULL WHERE name IN ('PC-06','PC-07','PC-08','PC-09')`);
  try { process.kill(-agent.pid); } catch (e) { try { agent.kill(); } catch (_) {} }
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
