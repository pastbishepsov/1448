// Ж1-и3: гостевые клиенты — PWA (/app) показывает уведомление deposit_void на трёх языках,
// в истории отменённый депозит помечен, в журнале кошелька операция подписана;
// шелл (/shell) — строка уведомления через свой словарь.
const { chromium } = require('playwright-core');
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
(async () => {
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const page = await browser.newPage({ viewport: { width: 420, height: 860 } });
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(BASE + '/app', { waitUntil: 'load' });
  await sleep(800);
  await page.fill('#nick', 'Guest27'); await page.fill('#pass', 'Guest12345'); await page.keyboard.press('Enter');
  await sleep(2500);
  // депозит + отмена руками админа через API (в этой же странице — просто fetch)
  const ids = await page.evaluate(async () => {
    const lg = await (await fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: 'admin1', password: 'Admin12345' }) })).json();
    const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + lg.access_token };
    const meId = me.id;
    const dep = await (await fetch('/api/v1/admin/users/' + meId + '/deposit', { method: 'POST', headers: h, body: JSON.stringify({ amount_pln: 33, method: 'card' }) })).json();
    await new Promise(r => setTimeout(r, 200));
    const v = await (await fetch('/api/v1/admin/deposits/' + dep.deposit_id + '/void', { method: 'POST', headers: h })).json();
    return { dep, v };
  });
  check('депозит 33 zł оформлен и отменён через API', ids.dep.deposit_id && ids.v.voided, JSON.stringify(ids.v));
  // тексты уведомления на трёх языках — через словарь страницы
  const texts = await page.evaluate(() => {
    const n = { type: 'deposit_void', payload: { amount_pln: 33, refund_pln: 33, coins: 330, wallet_pln: 0 } };
    const outT = {};
    for (const l of ['ru', 'en', 'pl']) { STR = Object.assign({}, STR_RU, LANGS[l]); outT[l] = ntfText(n); }
    STR = Object.assign({}, STR_RU, LANGS.ru);
    return outT;
  }).catch(e => ({ err: e.message }));
  check('ru: уведомление об отмене депозита', /Депозит 33 zł отменён/.test(texts.ru || ''), JSON.stringify(texts));
  check('en: top-up voided', /Top-up 33 zł voided/.test(texts.en || ''), texts.en);
  check('pl: wpłata anulowana', /Wpłata 33 zł anulowana/.test(texts.pl || ''), texts.pl);
  // живой поток: уведомление приходит опросом и попадает в ленту
  await page.evaluate(() => loadNotifs());
  await sleep(1500);
  const feed = await page.evaluate(() => (typeof ntfFeed !== 'undefined' ? ntfFeed : []).map(x => x.t));
  check('лента уведомлений PWA содержит отмену депозита', feed.some(t => /отменён у стойки/.test(t)), feed.slice(0, 2).join(' | '));
  // история и кошелёк
  const hist = await page.evaluate(async () => {
    const d = await api('/me/deposits'); const w = await api('/me/wallet');
    return { dep: d.deposits[0], tx: w.transactions[0], label: STR['wtx_' + w.transactions[0].kind], histVoided: STR.hist_voided };
  });
  check('/me/deposits: первый депозит помечен voided_at', !!hist.dep.voided_at, JSON.stringify(hist.dep));
  check('журнал кошелька: deposit_void подписан «Отмена пополнения»', hist.tx.kind === 'deposit_void' && hist.label === 'Отмена пополнения', hist.label);
  check('история: подпись «отменено» есть в словаре', hist.histVoided === 'отменено', hist.histVoided);
  check('js-ошибок в PWA нет', errors.length === 0, errors.join(' | '));
  // шелл: словарь на трёх языках
  const p2 = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  const e2 = [];
  p2.on('pageerror', e => e2.push(e.message));
  await p2.goto(BASE + '/shell', { waitUntil: 'load' });
  await sleep(1500);
  const sh = await p2.evaluate(() => {
    const n = { type: 'deposit_void', payload: { amount_pln: 33, refund_pln: 12.5, coins: 330, wallet_pln: 4 } };
    const o = {};
    for (const l of ['ru', 'en', 'pl']) { STR = Object.assign({}, SH_RU, SH_LANGS[l]); o[l] = notifText(n); }
    STR = Object.assign({}, SH_RU, SH_LANGS.ru);
    return o;
  }).catch(e => ({ err: e.message }));
  // З16-и1е: суммы в уведомлениях — как ценники киоска: «12,50», а не «12.5», как пришло с сервера
  check('shell ru', /Депозит 33 zł отменён у стойки — с кошелька снято 12,50 zł, в кошельке 4 zł/.test(sh.ru || ''), JSON.stringify(sh));
  check('shell en', /Top-up 33 zł voided at the desk/.test(sh.en || ''), sh.en);
  check('shell pl', /Wpłata 33 zł anulowana przy ladzie/.test(sh.pl || ''), sh.pl);
  check('js-ошибок в шелле нет', e2.length === 0, e2.join(' | '));
  await browser.close();
  console.log(out.join('\n'));
  const fails = out.filter(l => l.startsWith('FAIL')).length;
  console.log(`\nитого: ${out.length - fails} ok, ${fails} fail`);
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); console.log(out.join('\n')); process.exit(1); });
