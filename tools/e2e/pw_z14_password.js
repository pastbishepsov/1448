// З14-и3 «Пароли персонала», браузер (№23; З-Р17). Владелец: «Персонал» → «Карточка» → «Сбросить пароль» → «Точно
// сбросить?» — на месте кнопки временный пароль один раз, «Скопировать», подсказка; открытая админка сотрудника при
// первом же запросе выходит на экран входа: «Пароль менялся — войди новым…». Вход временным — не в админку, а на экран
// «z14t, задай свой пароль» (временный второй раз не спрашивают; несовпадение — «Новые пароли не совпадают»), после —
// админка и тост. Перезагрузка на этом экране — он же, но временный спрашивают. «Сменить пароль» — по нику в шапке:
// шторка, тост, админка работает дальше. Наём — подсказка «пароль временный». Журнал «Персонал» — «пароль сброшен» и
// «пароль сменён». Телефон 360 — экран смены пароля без прокрутки вбок. Данные: временные z14t и z14u; уходят к концу
// проверки, пароли — как у временных сотрудников; денег не трогает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const T = 'z14t', U = 'z14u', NEWT = 'Z14t-own-1', NEWU = 'Z14u-own-1', NEWU2 = 'Z14u-own-2';
  const ut = await z.hireTemp(T), uu = await z.hireTemp(U);
  z.atEnd(() => z.sql(`UPDATE users SET password_hash=(SELECT password_hash FROM users WHERE nickname='admin1'), must_change_password=false
    WHERE nickname IN ('${T}', '${U}')`)); // пароли — как у временных сотрудников
  const flag = u => z.sql(`SELECT must_change_password FROM users WHERE id = '${u}'`);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const TEMP_RE = /^[abcdefghjkmnpqrtuvwxyz2346789]{4}(-[abcdefghjkmnpqrtuvwxyz2346789]{4}){2}$/;
  // окно без входа: страница входа админки (свой браузер — openAdmin ждёт админку, а здесь её быть не должно)
  const blank = async (nick, viewport = { width: 1440, height: 900 }) => {
    const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
    z.atEnd(() => br.close());
    const p = await (await br.newContext({ viewport })).newPage(); z.watch(p);
    p.on('pageerror', e => z.jsErrors.push(`${nick}: ${e.message}`));
    await p.goto(z.BASE + '/admin', { waitUntil: 'load' });
    return p;
  };
  const enterAs = async (p, nick, pass) => { await p.fill('#nick', nick); await p.fill('#pass', pass); await p.keyboard.press('Enter'); await z.idle(p); }; // З19-и14: после записи — ждём страницу, а не паузу
  // тур знакомства стартует через 0,9 с после входа в админку — пропускаем, как openAdmin
  const skipTour = async p => { await z.sleep(1200); await p.evaluate(() => { const t = document.getElementById('tour'); if (t && !t.classList.contains('hidden')) tourSkip(); }); };
  const screen = p => p.evaluate(() => { const g = id => document.getElementById(id), vis = id => !!g(id) && !g(id).classList.contains('hidden');
    return { app: vis('appView'), login: vis('loginCard'), pw: vis('pwCard'), cur: vis('pw_cur'), head: (g('pwHead') || {}).textContent,
      err: (g('loginErr') || {}).textContent, pwErr: (g('pwErr') || {}).textContent }; });

  // ── владелец сбрасывает пароль z14t, у которого открыта админка ──
  const tp = await z.openAdmin(T);
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'staff');
  await boss.waitForSelector('#staffTable tr', { timeout: 15000 }).catch(() => {});
  const openCard = async nick => { await boss.evaluate(n => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    const b = tr && [...tr.querySelectorAll('button')].find(x => x.textContent.trim() === 'Карточка'); if (b) b.click(); }, nick); await z.sleep(1200); };
  await openCard(T);
  const c0 = norm(await z.text(boss, '#scPwd'));
  z.check('карточка z14t: «Сбросить пароль — забыл пароль: выдаст временный, все его входы закроются»',
    c0 === 'Сбросить пароль — забыл пароль: выдаст временный, все его входы закроются', c0);
  await boss.click('#scPwd button:has-text("Сбросить пароль")');
  await z.sleep(300);
  const armed = norm(await z.text(boss, '#scPwd button'));
  const toastB = norm(await z.toastAfter(boss, () => boss.click('#scPwd button:has-text("Точно сбросить?")'), 'Временный пароль')); // З19-и13: тост этого действия, а не #toast через паузу
  const tb = await boss.evaluate(() => ({ tp: ((document.querySelector('#scPwd .tp') || {}).textContent || '').trim(),
    box: ((document.getElementById('scPwd') || {}).textContent || '').replace(/\s+/g, ' ').trim() }));
  z.check('«Сбросить пароль» → «Точно сбросить?» → на месте кнопки временный пароль xxxx-xxxx-xxxx, «Скопировать», «Показывается один раз…»; тост; '
    + 'в базе — обязательная смена', armed === 'Точно сбросить?' && TEMP_RE.test(tb.tp) && tb.box.includes('Скопировать')
    && tb.box.includes('Показывается один раз. Все его входы уже закрыты; при входе админка попросит задать свой пароль.')
    && toastB === `🔑 Временный пароль ${T} выдан — передай ему` && flag(ut) === 't', JSON.stringify({ armed, tb, toastB }));
  await z.shot(boss, 'z14_password_reset');
  await boss.evaluate(() => closeDrawer());
  const temp = tb.tp;

  await tp.evaluate(() => api('/admin/work/me').catch(() => {})); // любой запрос открытой админки z14t
  await z.idle(tp); // выход после «пароль сменили» — запись
  const s1 = await screen(tp);
  z.check('открытая админка z14t на первом же запросе — экран входа «Пароль менялся — войди новым. Не знаешь его — спроси владельца.»',
    !s1.app && s1.login && s1.err === 'Пароль менялся — войди новым. Не знаешь его — спроси владельца.', JSON.stringify(s1));

  // ── вход временным: сначала свой пароль ──
  await enterAs(tp, T, temp);
  const s2 = await screen(tp);
  z.check('вход временным — не админка, а «z14t, задай свой пароль»; временный второй раз не спрашивают',
    !s2.app && s2.pw && !s2.login && !s2.cur && s2.head === `${T}, задай свой пароль`, JSON.stringify(s2));
  await z.shot(tp, 'z14_password_forced');
  await tp.fill('#pw_new', NEWT); await tp.fill('#pw_new2', NEWT + 'x');
  await tp.click('#pwCard button:has-text("Задать и войти")');
  await z.sleep(400);
  const s3 = await screen(tp);
  await tp.fill('#pw_new2', NEWT);
  const toastT = norm(await z.toastAfter(tp, async () => { await tp.click('#pwCard button:has-text("Задать и войти")');
    await tp.waitForSelector('#rail', { state: 'visible', timeout: 10000 }).catch(() => {}); }, 'Пароль задан'));
  const s4 = await screen(tp);
  await skipTour(tp);
  z.check('новые не совпали — «Новые пароли не совпадают»; совпали — админка открыта, тост «Пароль задан — дальше входи им», флага в базе нет',
    s3.pwErr === 'Новые пароли не совпадают' && s4.app && !s4.pw && toastT === 'Пароль задан — дальше входи им' && flag(ut) === 'f',
    JSON.stringify({ s3: s3.pwErr, s4, toastT }));

  // ── перезагрузка на экране смены: временный спрашивают ──
  const r = await z.api('POST', `/admin/staff/${uu}/password/reset`, {}, 'boss');
  const up = await blank(U);
  await enterAs(up, U, r.data.temp_password);
  await up.reload({ waitUntil: 'load' });
  await z.sleep(2500);
  const s5 = await screen(up);
  await up.fill('#pw_cur', r.data.temp_password); await up.fill('#pw_new', NEWU); await up.fill('#pw_new2', NEWU);
  await up.press('#pw_new2', 'Enter');
  await z.idle(up);
  const s6 = await screen(up);
  await skipTour(up);
  z.check('перезагрузка на экране смены — он же, временный спрашивают; Enter — админка', s5.pw && s5.cur && !s5.app && s6.app && flag(uu) === 'f',
    JSON.stringify({ s5, s6 }));

  // ── «Сменить пароль» по нику в шапке ──
  const who = await up.evaluate(() => document.getElementById('who').title);
  await up.click('#who');
  await z.sleep(800);
  const title = norm(await z.text(up, '#drawerTitle'));
  await up.fill('#mp_cur', NEWU); await up.fill('#mp_new', NEWU2); await up.fill('#mp_new2', NEWU2);
  const toastU = norm(await z.toastAfter(up, () => up.click('#drawer button:has-text("Сменить")'), 'Пароль сменён'));
  const still = await up.evaluate(() => api('/admin/work/me').then(() => true, () => false));
  await z.sleep(800);
  const s7 = await screen(up);
  const login2 = await z.login(U, NEWU2);
  z.check('ник в шапке — «Сменить пароль»: шторка «Сменить пароль · z14u», тост «Пароль сменён — на других устройствах войдёшь новым», '
    + 'админка работает дальше; новый пароль входит', who === 'Сменить пароль' && title === `Сменить пароль · ${U}`
    && toastU === 'Пароль сменён — на других устройствах войдёшь новым' && still && s7.app && login2.status === 200,
    JSON.stringify({ who, title, toastU, still, app: s7.app, login: login2.status }));
  await z.shot(up, 'z14_password_mine');

  // ── наём и журнал ──
  await boss.evaluate(() => hireDrawer());
  await z.sleep(600);
  const hire = norm(await z.text(boss, '#drawer'));
  await boss.evaluate(() => closeDrawer());
  z.check('наём: «Этот пароль временный: при первом входе админка попросит сотрудника задать свой»',
    hire.includes('Этот пароль временный: при первом входе админка попросит сотрудника задать свой.'), hire.slice(0, 300));
  await z.nav(boss, 'grants');
  await boss.click('#s-grants .segs button[data-k="staff"]');
  await z.sleep(1800);
  const au = await boss.evaluate(n => [...document.querySelectorAll('#auditTable tr')].filter(x => x.cells[2] && x.cells[2].innerText.trim() === n)
    .map(x => x.cells[1].innerText.trim() + ' | ' + x.cells[3].innerText.trim()), T);
  z.check('журнал «Персонал»: «персонал: пароль сменён — z14t · сменил пароль (временный — на свой)» и «персонал: пароль сброшен — '
    + 'z14t · выдан временный пароль, все входы закрыты»', au[0] === `персонал: пароль сменён | ${T} · сменил пароль (временный — на свой)`
    && au[1] === `персонал: пароль сброшен | ${T} · выдан временный пароль, все входы закрыты`, JSON.stringify(au.slice(0, 3)));

  // ── телефон: экран смены пароля ──
  const r2 = await z.api('POST', `/admin/staff/${uu}/password/reset`, {}, 'boss');
  const ph = await blank(U + '-phone', { width: 360, height: 780 });
  await enterAs(ph, U, r2.data.temp_password);
  const fit = await ph.evaluate(() => { const c = document.getElementById('pwCard').getBoundingClientRect();
    return { sw: document.documentElement.scrollWidth, box: c.width > 0 && c.left >= 0 && c.right <= innerWidth }; });
  const s8 = await screen(ph);
  await z.shot(ph, 'z14_password_phone');
  z.check('телефон 360: «задай свой пароль» — без прокрутки вбок', s8.pw && fit.sw <= 360 && fit.box, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
