// Ж1-и3: playwright — тост «Вернуть» после депозита, вкладка «Депозиты» с пометкой и
// arm-отменой, журнал с deposit_void, отчёт «Деньги» (владелец) — потери по депозитам.
const { chromium } = require('playwright-core');
const { toastAfter } = require('./_z'); // З19-и13: тост этого действия, а не #toast через паузу
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
(async () => {
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  // карточка Guest28 → форма депозита
  const guestId = await page.evaluate(async () => (await api('/admin/users?q=Guest28')).users.find(u => u.nickname === 'Guest28').id);
  // чистый старт: живые депозиты Guest28 за клубные сутки (их оставляют e2e_zone_minutes/club_day) отменяем —
  // иначе счёт кнопок «Отменить» зависит от порядка прогонов
  await page.evaluate(async id => { const c = await api('/admin/users/' + id); for (const d of (c.deposits || [])) if (!d.voided_at) { try { await api('/admin/deposits/' + d.id + '/void', { method: 'POST', body: '{}' }); } catch (e) {} } }, guestId);
  await sleep(500);
  await page.evaluate(id => openUserDrawer(id, 'deposit'), guestId);
  await sleep(800);
  const before = await page.evaluate(() => ({ w: uData.user.wallet_grosz, c: uData.user.coins_balance }));
  await page.fill('#d_sum', '70');
  const toast1 = await toastAfter(page, () => page.click('text=Оформить депозит'), '70,00 zł');
  const hasUndo = await page.locator('#toast button:has-text("Вернуть")').count();
  check('тост после депозита с кнопкой «Вернуть»', hasUndo === 1 && /70,00 zł/.test(toast1), toast1);
  await page.screenshot({ path: OUT + '/zh1_toast.png' });
  const toast2 = await toastAfter(page, () => page.click('#toast button:has-text("Вернуть")'), 'отменён');
  check('после «Вернуть» — тост об отмене со снятой суммой', /отменён/.test(toast2) && /снято 70,00 zł/.test(toast2), toast2);
  const after = await page.evaluate(() => ({ w: uData.user.wallet_grosz, c: uData.user.coins_balance }));
  check('карточка обновилась: кошелёк и монеты как до депозита', after.w === before.w && after.c === before.c, JSON.stringify({ before, after }));
  // вкладка «Депозиты»: отменённый с пометкой, без кнопки; живой — с кнопкой
  await page.evaluate(() => uFormToggle('deposit'));
  await sleep(300);
  await page.fill('#d_sum', '25');
  await page.click('text=Оформить депозит');
  await sleep(900);
  await page.evaluate(() => uTabSet('deposits'));
  await sleep(300);
  const tab = await page.evaluate(() => document.querySelector('.drawer .hist, #drawer .hist, .hist').outerHTML);
  check('вкладка: строка «отменён · возвращено 70 из 70»', /отменён · возвращено 70 из 70/.test(tab), tab.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').slice(0, 200));
  const btns = await page.locator('.hist button:has-text("Отменить")').count();
  check('кнопка «Отменить» только у живого депозита (1 шт)', btns === 1, String(btns));
  await page.screenshot({ path: OUT + '/zh1_tab.png' });
  // первый клик — выбор «как вернули» (З11-и2): главная — «Вернуть … как платили», второй клик по ней — отмена
  await page.click('.hist button:has-text("Отменить")');
  await sleep(200);
  const armed = await page.evaluate(() => document.querySelector('.hist .rfPick button.danger') && document.querySelector('.hist .rfPick button.danger').textContent);
  check('первый клик раскрывает выбор («Вернуть 25,00 zł наличными» — как платили)', armed === 'Вернуть 25,00 zł наличными', String(armed));
  await page.click('.hist .rfPick button.danger');
  await sleep(1200);
  const rows2 = await page.evaluate(() => [...document.querySelectorAll('.hist tr')].map(r => r.textContent.replace(/\s+/g, ' ').trim()));
  check('после второго клика оба депозита отменены, кнопок нет', rows2.filter(r => /отменён/.test(r)).length >= 2 && (await page.locator('.hist button:has-text("Отменить")').count()) === 0, rows2.slice(0, 3).join(' | '));
  // журнал: строка «отмена депозита» в фильтре «деньги»
  await page.evaluate(() => nav('grants'));
  await sleep(1000);
  await page.evaluate(() => { audKind = 'money'; renderAudit(true); });
  await sleep(300);
  const aud = await page.evaluate(() => [...document.querySelectorAll('#auditTable tr')].slice(1, 6).map(r => r.textContent.replace(/\s+/g, ' ').trim()));
  check('журнал: «отмена депозита» с деталями возврата', aud.some(r => /отмена депозита/.test(r) && /снято с кошелька 25.00 zł/.test(r)), aud.slice(0, 3).join(' | '));
  await page.screenshot({ path: OUT + '/zh1_audit.png' });
  check('js-ошибок нет', errors.length === 0, errors.join(' | '));
  // владелец: отчёт «Деньги» — потери с депозитной строкой
  await page.evaluate(() => logout());
  await sleep(800);
  await page.fill('#nick', 'boss'); await page.fill('#pass', 'Boss12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  await page.evaluate(() => nav('stats'));
  await sleep(1500);
  const rep = await page.evaluate(() => document.getElementById('repBody') ? document.getElementById('repBody').textContent.replace(/\s+/g, ' ') : '');
  check('отчёт «Деньги»: строка «— по ошибочным депозитам»', /по ошибочным депозитам/.test(rep), rep.slice(0, 160));
  await page.screenshot({ path: OUT + '/zh1_money.png' });
  check('js-ошибок нет (владелец)', errors.length === 0, errors.join(' | '));
  await browser.close();
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
