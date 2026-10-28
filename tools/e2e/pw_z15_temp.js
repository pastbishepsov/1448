// З15-и6б «Смена при входе временным паролем», браузер (№30). Вход временным паролем открывает экран «z15y, задай свой
// пароль» с подсказкой «Смена откроется, когда задашь свой пароль», а смену — нет: «Выйти» с него не оставляет открытой
// записи табеля. «Задать и войти» — админка, тост, в шапке «на смене с …», в табеле — запись «открыта входом в админку»
// с этой минуты. Временный z15y: пароль и флаг временного пароля в конце — как у временных сотрудников (SQL), его записи
// табеля удаляются в начале и в конце; денег не трогает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const Y = 'z15y', OWN = 'Z15y-own-1';
  const yid = await z.hireTemp(Y);
  const drop = () => z.sql(`DELETE FROM work_entries WHERE user_id = '${yid}'`);
  drop();
  z.atEnd(drop);
  z.atEnd(() => z.sql(`UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE nickname='admin1'), must_change_password=false
    WHERE id = '${yid}'`)); // пароль — как у временных сотрудников
  z.sql(`UPDATE users SET must_change_password = true WHERE id = '${yid}'`); // пароль STAFF_PASS — временный
  const openCnt = () => z.sql(`SELECT count(*) FROM work_entries WHERE user_id = '${yid}' AND ended_at IS NULL`);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  // своё окно: openAdmin ждёт админку, а после входа временным её быть не должно
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const p = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  p.on('pageerror', e => z.jsErrors.push(`${Y}: ${e.message}`));
  await p.goto(z.BASE + '/admin', { waitUntil: 'load' });
  const screen = () => p.evaluate(() => { const g = id => document.getElementById(id), vis = id => !!g(id) && !g(id).classList.contains('hidden');
    return { app: vis('appView'), login: vis('loginCard'), pw: vis('pwCard'), head: (g('pwHead') || {}).textContent,
      hint: (g('pwHint') || {}).textContent }; });
  const enter = async pass => { await p.fill('#nick', Y); await p.fill('#pass', pass); await p.keyboard.press('Enter'); await z.sleep(2500); };

  await enter(z.STAFF_PASS);
  let s = await screen();
  z.check('вход временным паролем — экран «z15y, задай свой пароль» с «Смена откроется, когда задашь свой пароль.»; смена не '
    + 'открылась', s.pw && !s.app && s.head === `${Y}, задай свой пароль`
    && norm(s.hint).endsWith('Смена откроется, когда задашь свой пароль.') && openCnt() === '0', JSON.stringify({ s, open: openCnt() }));
  await z.shot(p, 'z15_temp_screen');
  await p.click('#pwCard button:has-text("Выйти")');
  await z.sleep(800);
  s = await screen();
  z.check('«Выйти» с экрана пароля — снова вход, открытой записи табеля нет', s.login && !s.pw && !s.app && openCnt() === '0',
    JSON.stringify({ s, open: openCnt() }));

  await enter(z.STAFF_PASS);
  const t1 = z.sql('SELECT now()::text');
  await p.fill('#pw_new', OWN);
  await p.fill('#pw_new2', OWN);
  await p.click('#pwCard button:has-text("Задать и войти")');
  await p.waitForSelector('#rail', { state: 'visible', timeout: 10000 }).catch(() => {});
  await z.sleep(1500);
  const toast = norm(await z.text(p, '#toast'));
  await p.evaluate(() => { const t = document.getElementById('tour'); if (t && !t.classList.contains('hidden')) tourSkip(); });
  s = await screen();
  const chip = norm(await z.text(p, '#shiftTxt'));
  const row = z.sql(`SELECT count(*) || '|' || COALESCE(max(note), '') FROM work_entries WHERE user_id = '${yid}' AND ended_at IS NULL AND started_at >= '${t1}'`);
  z.check('«Задать и войти» — админка, тост «Пароль задан — дальше входи им», в шапке «на смене с …»; в табеле — запись '
    + '«открыта входом в админку» с этой минуты', s.app && toast === 'Пароль задан — дальше входи им'
    && /^на смене с \d\d:\d\d$/.test(chip) && openCnt() === '1' && row === '1|открыта входом в админку',
    JSON.stringify({ s, toast, chip, row }));
  await z.shot(p, 'z15_temp_entered');
  z.check('ошибок JS на странице нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
