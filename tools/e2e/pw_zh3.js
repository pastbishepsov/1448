// Ж3: playwright — лента зала не пуста сразу после входа (бэкфилл из журнала),
// клик по KPI «Выручка сегодня» у админа открывает журнал с фильтром «Деньги»,
// журнал: чипы шлют kind на сервер, «Ещё 100» дотягивает страницу, у владельца — выбор дня.
const { chromium } = require('playwright-core');
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
const { execSync } = require('child_process');
const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
(async () => {
  const admin1 = psql(`SELECT id FROM users WHERE nickname='admin1'`);
  // 130 действий сегодня — чтобы у журнала была вторая страница
  psql(`DELETE FROM admin_actions WHERE details LIKE 'pw-zh3%'`);
  psql(`INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '${admin1}', 'pc_unhold', 'pw-zh3 #' || i, now() - (i || ' seconds')::interval FROM generate_series(1, 130) i`);
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  await page.evaluate(() => nav('hall'));
  await sleep(1200);
  // и4: лента после входа (без единого живого события) не пуста
  const feed1 = await page.evaluate(() => ({ n: feed.length, audit: feed.filter(m => m.type === 'audit').length, ids: new Set(feed.map(m => m.id)).size, html: document.getElementById('feedList').textContent.slice(0, 160) }));
  check('лента зала сразу после входа: 30 строк из журнала, все с id, без дублей', feed1.n === 30 && feed1.audit === 30 && feed1.ids === 30, JSON.stringify(feed1));
  // перезагрузка страницы — лента снова не пуста
  await page.reload({ waitUntil: 'load' });
  await sleep(2500);
  const feed2 = await page.evaluate(() => ({ n: feed.length, text: document.getElementById('feedList').textContent.replace(/\s+/g, ' ').slice(0, 120) }));
  check('после перезагрузки лента не пуста', feed2.n > 0 && !/События появятся здесь/.test(feed2.text), JSON.stringify(feed2));
  // реконнект сокета — бэкфилл не дублирует строки
  await page.evaluate(() => { if (ws) ws.close(); });
  await sleep(6500);
  const feed3 = await page.evaluate(() => ({ n: feed.length, ids: new Set(feed.map(m => m.id)).size, wsOpen }));
  check('после реконнекта сокета лента без дублей (id уникальны), сокет живой', feed3.n === feed3.ids && feed3.wsOpen, JSON.stringify(feed3));
  await page.screenshot({ path: OUT + '/zh3_feed.png' });
  // и2: клик по KPI у админа → журнал с фильтром «Деньги»
  await page.click('#kRevBox');
  await sleep(1500);
  const kpi = await page.evaluate(() => ({ tab: curTab, kind: audKind, chip: document.querySelector('#s-grants .segs button.on') && document.querySelector('#s-grants .segs button.on').textContent, title: document.getElementById('kRevBox').title,
    kinds: [...new Set(audData.map(x => x.kind))], money: AUD_GROUPS.money }));
  check('KPI «Выручка сегодня» у админа → Журнал с чипом «Деньги», строки только денежных видов', kpi.tab === 'grants' && kpi.kind === 'money' && kpi.chip === 'Деньги' && kpi.kinds.every(k => kpi.money.includes(k)), JSON.stringify(kpi).slice(0, 300));
  const RH = psql(`SELECT lpad(COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0'), 2, '0')`); // З18-и4б: граница стенда
  check('подсказка KPI называет состав и час суток', kpi.title.includes(`клубные сутки с ${RH}:00`) && /пополнения/.test(kpi.title) && /товары/.test(kpi.title), kpi.title.slice(0, 160));
  // и3: «Все» — 100 строк + кнопка «Ещё 100»; клик — вторая страница
  await page.click('#s-grants .segs button[data-k="all"]');
  await sleep(1200);
  const pg1 = await page.evaluate(() => ({ rows: document.querySelectorAll('#auditTable table tr').length - 1, more: !!document.getElementById('audMoreBtn'), hasMore: audHasMore, before: audBefore }));
  check('журнал «Все»: 100 строк и кнопка «Ещё 100»', pg1.rows === 100 && pg1.more && pg1.hasMore && pg1.before, JSON.stringify(pg1));
  await page.click('#audMoreBtn');
  await sleep(1500);
  const pg2 = await page.evaluate(() => ({ rows: document.querySelectorAll('#auditTable table tr').length - 1, ids: new Set(audData.map(x => x.id)).size, n: audData.length }));
  check('после «Ещё 100» строк больше 100, id уникальны', pg2.rows > 100 && pg2.ids === pg2.n, JSON.stringify(pg2));
  // рефреш подмешивает свежее сверху, не роняя хвост
  await page.evaluate(() => renderAudit());
  await sleep(1200);
  const pg3 = await page.evaluate(() => ({ n: audData.length, first: audData[0] && audData[0].created_at }));
  check('плановый рефреш журнала сохраняет долистанный хвост', pg3.n >= pg2.n, JSON.stringify(pg3));
  const dayPicker = await page.evaluate(() => { const el = document.getElementById('audDay'); return el && !el.closest('.ownerOnly').classList.contains('hidden'); });
  check('у админа выбора дня нет', dayPicker === false, String(dayPicker));
  await page.screenshot({ path: OUT + '/zh3_journal.png' });
  check('js-ошибок нет (админ)', errors.length === 0, errors.join(' | '));
  // владелец: выбор дня
  await page.evaluate(() => logout());
  await sleep(800);
  await page.fill('#nick', 'boss'); await page.fill('#pass', 'Boss12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  await page.evaluate(() => nav('grants'));
  await sleep(1200);
  const ownerAll = await page.evaluate(() => ({ n: audData.length, picker: !document.getElementById('audDay').closest('.ownerOnly').classList.contains('hidden') }));
  check('владелец: журнал «все дни», выбор дня виден', ownerAll.n === 100 && ownerAll.picker, JSON.stringify(ownerAll));
  const yday = new Date(Date.now() - 86400000).toISOString().slice(0, 10);
  await page.evaluate(d => audDaySet(d), yday);
  await sleep(1500);
  const ownerDay = await page.evaluate(() => ({ day: audDay, note: document.querySelector('#auditTable .muted') && document.querySelector('#auditTable .muted').textContent, allOn: document.getElementById('audAllDays').classList.contains('on'),
    inWindow: audData.every(x => true), n: audData.length }));
  check(`владелец: день ${yday} — подпись «Клубные сутки …», кнопка «Все дни» снята`, ownerDay.day === yday && /Клубные сутки/.test(ownerDay.note || '') && !ownerDay.allOn, JSON.stringify(ownerDay).slice(0, 200));
  await page.evaluate(() => audDaySet(''));
  await sleep(1000);
  const back = await page.evaluate(() => ({ day: audDay, allOn: document.getElementById('audAllDays').classList.contains('on') }));
  check('«Все дни» возвращает журнал за всё время', back.day === '' && back.allOn, JSON.stringify(back));
  check('js-ошибок нет (владелец)', errors.length === 0, errors.join(' | '));
  await browser.close();
  psql(`DELETE FROM admin_actions WHERE details LIKE 'pw-zh3%'`);
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
const AUD_MONEY = null; // Ж6: денежные виды берём из самой страницы (AUD_GROUPS.money) — список растёт со спринтами
