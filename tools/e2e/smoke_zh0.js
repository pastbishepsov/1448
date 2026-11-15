const { chromium } = require('playwright-core');
const { idle, watch, signIn, reenter } = require('./_z'); // З19-и14: после записи — ждём страницу, а не паузу
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const sleep = ms => new Promise(r => setTimeout(r, ms));
(async () => {
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } }); watch(page);
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await signIn(page, 'boss', 'Boss12345'); // З19-и14: вход — когда запас лимитера стенда полный, дальше — когда вход дописан, а не пауза
  await page.click('#r-settings'); await sleep(1500);
  const v0 = await page.inputValue('#set_compcap'), m0 = await page.inputValue('#set_compcapmin');
  console.log('поля видны, значения:', v0, m0);
  await page.fill('#set_compcap', '50'); await page.fill('#set_compcapmin', '60');
  await page.click('button:has-text("Сохранить")'); await idle(page); // до перезагрузки — ответ на сохранение
  await reenter(page); // перезагрузка — снова вход: тот же залп запросов
  await page.click('#r-settings'); await sleep(1500);
  const saved = [await page.inputValue('#set_compcap'), await page.inputValue('#set_compcapmin')];
  console.log('после перезагрузки:', saved[0], saved[1]);
  await page.screenshot({ path: OUT + '/zh0-settings.png' });
  await page.fill('#set_compcap', '0'); await page.fill('#set_compcapmin', '0');
  await page.click('button:has-text("Сохранить")'); await idle(page); // сброс — до закрытия браузера
  console.log('сброшено:', await page.inputValue('#set_compcap'), await page.inputValue('#set_compcapmin'), 'js errors:', errors.length, errors.join('|'));
  await browser.close();
  const ok = saved[0] === '50' && saved[1] === '60' && errors.length === 0;
  console.log(ok ? 'итого: ok' : 'итого: FAIL (потолки не сохранились или ошибки JS)');
  process.exit(ok ? 0 : 1);
})().catch(e => { console.error(e); process.exit(1); });
