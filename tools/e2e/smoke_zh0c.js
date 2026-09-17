const { chromium } = require('playwright-core');
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const sleep = ms => new Promise(r => setTimeout(r, ms));
(async () => {
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [], refreshes = [], wsUrls = [];
  page.on('pageerror', e => errors.push(e.message));
  page.on('response', r => { if (r.url().endsWith('/auth/refresh')) refreshes.push(r.status()); });
  page.on('websocket', ws => wsUrls.push(ws.url()));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', 'admin1'); await page.fill('#pass', 'Admin12345'); await page.keyboard.press('Enter');
  await sleep(3500);
  const live = await page.evaluate(() => ({ wsOpen, liveTxt: document.getElementById('liveTxt').textContent }));
  console.log('WS:', JSON.stringify(live), 'urls:', wsUrls.map(u => u.replace(/ticket=.*/, 'ticket=…')).join(' '));
  console.log('в URL сокета нет eyJ:', wsUrls.every(u => !u.includes('eyJ')), '| есть ticket=:', wsUrls.every(u => u.includes('ticket=')));
  // гонка refresh: протухший access + параллельные запросы с рассинхроном
  const results = [];
  for (const stagger of [0, 3, 6, 9, 12, 20, 50]) {
    refreshes.length = 0;
    const res = await page.evaluate(async (st) => {
      localStorage.setItem('1448a_access', 'expired.token.value');
      const pa = api('/admin/work/me').then(() => 'ok').catch(e => 'ERR:' + e.message);
      await new Promise(r => setTimeout(r, st));
      const pb = api('/admin/staff-chat').then(() => 'ok').catch(e => 'ERR:' + e.message);
      const pc = api('/admin/overview').then(() => 'ok').catch(e => 'ERR:' + e.message);
      return { out: [await pa, await pb, await pc], loggedOut: !document.getElementById('loginView').classList.contains('hidden') };
    }, stagger);
    await sleep(250);
    results.push(`${stagger}ms → ${res.out.join(',')} refresh:${refreshes.join(',')} loggedOut:${res.loggedOut}`);
    if (res.loggedOut) break;
  }
  console.log(results.join('\n'));
  // живой канал переживает реконнект: рвём сокет — админка берёт новый тикет
  await page.evaluate(() => ws.close());
  await sleep(6500);
  const live2 = await page.evaluate(() => ({ wsOpen, liveTxt: document.getElementById('liveTxt').textContent }));
  console.log('после обрыва:', JSON.stringify(live2), 'сокетов всего:', wsUrls.length);
  const loginShown = await page.locator('#loginView').isVisible();
  console.log('login screen:', loginShown, 'js errors:', errors.length, errors.join('|'));
  await browser.close();
  const ok = !results.some(r => /loggedOut:true/.test(r)) && live2.wsOpen && !loginShown && errors.length === 0;
  console.log(ok ? 'итого: ok' : 'итого: FAIL (разлогин в гонке refresh, сокет не поднялся или ошибки JS)');
  process.exit(ok ? 0 : 1);
})().catch(e => { console.error(e); process.exit(1); });
