// З19-и3 «Табло, удаление ПК и записей каталога» (№40), браузер: табло /board?token=… опрашивает /board/state с токеном в
// заголовке X-Board-Token — в адресе запроса токена нет (не оседает в журнале запросов и истории прокси); зал на табло
// отрисован. Токен табло выпускается на время проверки, в конце — как был.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const tok0 = z.rows("SELECT value FROM settings WHERE key = 'board_token'")[0];
  z.atEnd(() => { z.sql("DELETE FROM settings WHERE key = 'board_token'");
    if (tok0 !== undefined) z.sql(`INSERT INTO settings (key, value) VALUES ('board_token', '${tok0}')`); });
  const T = ((await z.api('POST', '/admin/board/token', {}, 'boss')).data || {}).token || '';
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close().catch(() => {}));
  const tv = await (await br.newContext({ viewport: { width: 1920, height: 1080 } })).newPage();
  tv.on('pageerror', e => z.jsErrors.push('табло: ' + e.message));
  const reqs = [];
  tv.on('request', r => { if (r.url().includes('/board/state')) reqs.push({ url: r.url(), hdr: r.headers()['x-board-token'] || '' }); });
  await tv.goto(z.BASE + '/board?token=' + T, { waitUntil: 'load' }); await z.sleep(2000);
  const st = await tv.evaluate(() => ({ off: document.getElementById('off').classList.contains('show'), pcs: document.querySelectorAll('#zones .pc').length }));
  z.check('табло: /board/state — без токена в адресе, токен — заголовком X-Board-Token; зал отрисован, заглушки нет',
    !!T && reqs.length > 0 && reqs.every(r => !/token=/.test(r.url) && r.hdr === T) && !st.off && st.pcs > 0,
    JSON.stringify({ reqs: reqs.slice(0, 2).map(r => ({ url: r.url.replace(T, '<T>'), hdr: r.hdr === T })), st }));
  await z.shot(tv, 'z19_board');
  await z.done();
})().catch(z.crash);
