// З3-и1 (З-Р36), браузер: в ленте зала у строк прихода и ухода — ник того, кто пришёл или ушёл. Раньше
// было «табель: приход — вход в систему · День» без имени: при двух админах владелец не понимал, кто открыл
// смену. Временный сотрудник z3feed: вход как админка открывает ему смену, уход — через API; в конце уволен.
const z = require('./_z');
(async () => {
  const zid = await z.hireTemp('z3feed');
  z.atEnd(() => z.sql(`DELETE FROM work_entries WHERE user_id='${zid}'`));
  z.sql(`DELETE FROM work_entries WHERE user_id='${zid}'`);
  await z.tok('z3feed'); // вход админки — приход
  const stop = await z.api('POST', '/admin/work/stop', {}, 'z3feed');
  z.check('z3feed ушёл → 200', stop.status === 200, String(stop.status));
  const page = await z.openAdmin('boss');
  await z.sleep(1500); // лента поднимает последние строки журнала при подключении live-канала
  const lines = await page.evaluate(() => [...document.querySelectorAll('#feedList .fi')].map(e => e.textContent.replace(/\s+/g, ' ').trim()));
  const come = lines.find(l => /табель: приход/.test(l) && /z3feed/.test(l));
  const leave = lines.find(l => /табель: уход/.test(l) && /z3feed/.test(l));
  z.check('строка прихода: «табель: приход — z3feed · вход в админку»', come && /табель: приход — z3feed · вход в админку/.test(come), come || JSON.stringify(lines.slice(0, 6)));
  z.check('строка ухода: «табель: уход — z3feed · ушёл»', leave && /табель: уход — z3feed · ушёл/.test(leave), leave || JSON.stringify(lines.slice(0, 6)));
  await page.locator('#feedList').scrollIntoViewIfNeeded();
  await z.shot(page, 'z3_feed');
  await z.done();
})().catch(z.crash);
