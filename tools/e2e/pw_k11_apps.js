// К11-и2 «Chrome отдельной плиткой» — проверка в браузере (Chrome без окна): Guest46 за PC-07, сессия идёт.
// Агент ПК подставной: отвечает на /ping, /apps-status, /launch и запоминает, что его просили запустить.
// 1. Главный экран: в ряду «Приложения» папки «Другие» нет; «Войсчат» и «Системные» — папки; Google Chrome —
//    плитка в том же ряду, того же размера и в одну линию с папками; ряд платформ не тронут («Другие игры» — папка).
// 2. Одно нажатие по плитке: агент получил одну команду запуска Chrome, папка не открылась; плитка «запускается…» →
//    «работает»; второе нажатие второй раз не запускает; если Chrome на ПК нет — плитка погашена и говорит почему.
// 3. Папки: «Войсчат» (внутри Discord) — выход по Esc и боковой кнопкой мыши; «Системные» (4 приложения) — «← Назад».
// 4. Вкладка «Приложения» и поиск — как были: раздел «Другие» с Chrome на месте, поиск находит Chrome.
// 5. EN и PL; экраны 1920, 1440, 1366 и 1280; правило: без группы ни одного приложения и сразу четыре.
// В базе ничего не меняет: гостя сажает и сессию закрывает сама, кошелёк возвращает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;
const same = (a, b) => J(a) === J(b);
const CH = 'Google Chrome';
const TILE = '#catBody > .folders > .app[data-app="browser"]';
const NO = 'плитки Chrome на главном экране нет';

(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const w0 = z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  // кошелёк возвращаем одним запросом вместе со строкой журнала на ту же разницу (правило стенда трека З:
  // «кошелёк гостя = сумма его журнала», e2e_z0_stand); в конце сверяем, что расхождение осталось каким было
  const setWallet = v => z.sql(`WITH o AS (SELECT id, wallet_grosz AS w FROM users WHERE id='${g}' FOR UPDATE),
      u AS (UPDATE users SET wallet_grosz = ${v} FROM o WHERE users.id = o.id RETURNING o.w)
    INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, note, created_at)
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k11_apps: проверка вернула кошелёк', now() FROM u WHERE u.w <> ${v}`);
  const gap = () => z.sql('SELECT u.wallet_grosz - COALESCE((SELECT sum(amount_grosz) FROM wallet_transactions w WHERE w.user_id = u.id), 0) '
    + `FROM users u WHERE u.id='${g}'`);
  const gap0 = gap();
  z.atEnd(async () => {
    await endTails();
    setWallet(+w0);
    z.check('проверка убрала за собой: сессия закрыта, кошелёк гостя сходится со своим журналом так же, как до неё',
      gap() === gap0, `расхождение ${gap0} → ${gap()}`);
  });
  await endTails();

  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  z.check('гость посажен за PC-07, сессия идёт', !!(seat.data && seat.data.session_id), J(seat.data));

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const CORS = { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Headers': 'Content-Type',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS' };
  const ag = { apps: {}, running: null, launches: [] }; // что «знает» подставной агент и о чём его просили
  const ctx = await br.newContext({ viewport: { width: 1920, height: 1080 } });
  await ctx.route('http://127.0.0.1:1448/**', rt => {
    const rq = rt.request(), path = new URL(rq.url()).pathname;
    const json = body => rt.fulfill({ status: 200, contentType: 'application/json', headers: CORS, body: J(body) });
    if (rq.method() === 'OPTIONS') return rt.fulfill({ status: 204, headers: CORS, body: '' });
    if (path === '/ping') return json({ computer_id: pcId });
    if (path === '/apps-status') return json({ apps: ag.apps, running: ag.running });
    if (path === '/launch') {
      let b = {};
      try { b = rq.postDataJSON() || {}; } catch (e) { /* тело не JSON — запишем «?» */ }
      ag.launches.push(b.app_id || '?');
      return json({ ok: true, pid: 4242, state: 'launching' });
    }
    return rt.fulfill({ status: 404, headers: CORS, body: '' });
  });
  const p = await ctx.newPage();
  p.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  const until = (fn, arg, ms) => p.waitForFunction(fn, arg, { timeout: ms }).then(() => true).catch(() => false);
  const settle = async () => { await p.mouse.move(4, 4); await p.waitForTimeout(750); }; // появление плиток доиграло, мышь в стороне
  const size = async (wd, ht) => { await p.setViewportSize({ width: wd, height: ht }); await settle(); };
  const poll = () => p.evaluate(async () => { // дождаться своего опроса агента, а не чужого
    for (let k = 0; k < 60 && appsInFlight; k++) await new Promise(r => setTimeout(r, 50));
    await pollAppsStatus();
  });
  // кадр: заголовок «Приложения» вместе с рядом
  const shotRow = async name => {
    try {
      const c = await p.evaluate(() => {
        const rows = document.querySelectorAll('#catBody > .folders'), row = rows[rows.length - 1];
        if (!row) return null;
        row.scrollIntoView({ block: 'end' });
        const h = row.previousElementSibling, a = (h || row).getBoundingClientRect(), b = row.getBoundingClientRect();
        const y = Math.max(0, a.top - 10), x = Math.max(0, b.left - 10);
        return { x, y, width: Math.min(innerWidth - x, b.width + 20), height: Math.min(innerHeight - y, b.bottom - y + 10) };
      });
      if (c) { await p.waitForTimeout(200); await p.screenshot({ path: `${z.OUT}/k11a_${name}.png`, clip: c }); }
    } catch (e) { console.log('кадр', name, e.message.split('\n')[0]); }
  };
  // что сейчас в каталоге: ряды главного экрана, плитки ряда «Приложения», открытая папка, всплывшая подсказка
  const look = () => p.evaluate(() => {
    const txt = e => (e ? e.textContent.replace(/\s+/g, ' ').trim() : '');
    const rows = [...document.querySelectorAll('#catBody > .folders')], row = rows[rows.length - 1];
    const rb = row ? row.getBoundingClientRect() : { left: 0, right: 0 };
    const tst = document.getElementById('toast');
    return {
      rows: rows.length, open: catFolder ? catFolder.t + ':' + (catFolder.name || '') : null,
      top: txt(document.querySelector('#catBody .folder-top h2')),
      inside: [...document.querySelectorAll('#catBody .apps [data-app]')].map(e => e.dataset.app),
      plat: rows.length > 1 ? [...rows[0].querySelectorAll('.f-name')].map(txt) : [], platN: catGroups.length,
      kids: row ? [...row.children].map(e => {
        const nm = e.querySelector('.f-name, .n'), ic = e.querySelector('.aicon svg, .aicon .em, .f-icon svg'), b = e.getBoundingClientRect();
        return { kind: e.classList.contains('folder') ? 'folder' : e.classList.contains('app') ? 'app' : e.className,
          name: txt(nm), cnt: txt(e.querySelector('.f-cnt')), id: e.dataset.app || '', st: txt(e.querySelector('.gstate')),
          cls: (e.className.match(/st-\w+/) || [''])[0], w: e.offsetWidth, h: e.offsetHeight, t: e.offsetTop,
          icon: ic ? Math.round(ic.getBoundingClientRect().height) : 0, fit: !!nm && nm.scrollWidth <= nm.clientWidth + 1,
          inRow: b.left >= rb.left - 1 && b.right <= rb.right + 1 };
      }) : [],
      hscroll: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1 || (!!row && row.scrollWidth > row.clientWidth + 1),
      toast: tst.classList.contains('show') ? tst.textContent : '',
    };
  });
  const folders = s => s.kids.filter(k => k.kind === 'folder'), tiles = s => s.kids.filter(k => k.kind === 'app');
  const names = a => a.map(k => k.name);
  const onMain = s => s.rows === 2 && s.open === null && !s.top;
  const appsRow = () => p.locator('#catBody > .folders').last();
  const mouseBack = async () => { // боковая кнопка мыши «назад», как в _exits.js
    const c = await ctx.newCDPSession(p);
    await c.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: 700, y: 450, button: 'back', buttons: 8, clickCount: 1 });
    await c.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: 700, y: 450, button: 'back', buttons: 0, clickCount: 1 });
    await c.detach().catch(() => {});
  };

  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
  await until(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'), null, 15000);
  await until(() => agentOnline === true && document.querySelectorAll('#catBody > .folders').length === 2, null, 10000);
  await settle();
  const cat = await p.evaluate(() => {
    window.__k112 = { apps0: CATALOG.apps.slice(), keep: null,
      set(l) { this.keep = this.keep || { STR, LANG }; LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]); applyStrings(); renderCatalog(); },
      back() { if (this.keep) { LANG = this.keep.LANG; STR = this.keep.STR; this.keep = null; applyStrings(); renderCatalog(); } } };
    return { online: agentOnline, lang: LANG, apps: CATALOG.apps.map(a => [a.id, a.name, a.sub || '']), sys: CATALOG.system.map(a => a.id),
      t: { launching: T('launching', { n: appName('Google Chrome') }), again: T('launch_running', { n: appName('Google Chrome') }),
        missing: T(NA_REASON.target_missing, { n: appName('Google Chrome') }), stL: STR.st_launching, stR: STR.st_running, stOff: STR.st_unavail,
        back: STR.back } };
  });
  z.check('агент ПК на связи; в каталоге стенда Google Chrome — приложение без группы, Discord — в «Коммуникации», системных четыре',
    cat.online && cat.lang === 'ru' && cat.apps.some(a => a[0] === 'browser' && a[1] === CH && a[2] === '')
      && cat.apps.some(a => a[0] === 'discord' && a[2] === 'Коммуникация') && cat.sys.length === 4, J(cat.apps) + ' ' + J(cat.sys));

  console.log('— 1. Главный экран, ряд «Приложения»');
  const m = await look();
  const mf = folders(m), mt = tiles(m), ch = mt.find(k => k.id === 'browser'), hasTile = !!ch;
  z.check('в ряду «Приложения» папки «Другие» нет', onMain(m) && !names(mf).includes('Другие'), J(names(m.kids)));
  z.check('«Войсчат» и «Системные» остались папками: 1 приложение и 4 приложения',
    same(mf.filter(k => k.name !== 'Другие').map(k => [k.name, k.cnt]), [['Войсчат', '1 приложение'], ['Системные', '4 приложения']]), J(mf.map(k => [k.name, k.cnt])));
  z.check('Google Chrome лежит плиткой в том же ряду, после папок',
    hasTile && mt.length === 1 && ch.name === CH && same(m.kids.map(k => k.kind), ['folder', 'folder', 'app']), J(m.kids.map(k => [k.kind, k.name])));
  z.check('плитка Chrome того же размера, что папки, и стоит с ними в одну линию',
    hasTile && mf.every(k => k.w === ch.w && k.h === ch.h && k.t === ch.t), hasTile ? J(m.kids.map(k => [k.name, k.w, k.h, k.t])) : NO);
  z.check('у плитки значок Chrome не мельче значка папки, имя «Google Chrome» целиком', hasTile && ch.icon >= 36 && ch.icon >= Math.min(...mf.map(k => k.icon)) - 4 && ch.fit && !ch.cls,
    hasTile ? J([ch.icon, mf.map(k => k.icon), ch.fit, ch.cls]) : NO);
  z.check('ряд платформ не тронут: папок столько же, сколько платформ, «Другие игры» — папка',
    m.plat.length === m.platN && m.platN >= 2 && m.plat.includes('Другие игры') && m.plat.includes('Steam'), J(m.plat));
  await shotRow('row');

  console.log('— 2. Запуск одним нажатием');
  if (hasTile) { await p.click(TILE); await p.waitForTimeout(700); }
  const l1 = await look(), t1 = tiles(l1).find(k => k.id === 'browser') || {};
  z.check('одно нажатие по плитке — агент получил команду запустить Chrome, ровно одну', hasTile && same(ag.launches, ['browser']), hasTile ? J(ag.launches) : NO);
  z.check('папка при этом не открылась: экран остался главным', hasTile && onMain(l1), hasTile ? J([l1.rows, l1.open, l1.top]) : NO);
  z.check('плитка сразу пишет «запускается…», всплывает «🚀 Запускаю Google Chrome…»',
    hasTile && t1.cls === 'st-launching' && t1.st === cat.t.stL && l1.toast === cat.t.launching, hasTile ? J([t1.cls, t1.st, l1.toast]) : NO);
  await shotRow('row_launching');
  ag.running = { app_id: 'browser', pid: 4242 };
  await poll();
  const l2 = await look(), t2 = tiles(l2).find(k => k.id === 'browser') || {};
  z.check('агент сообщил, что Chrome работает, — плитка пишет «работает»', hasTile && t2.cls === 'st-running' && t2.st === cat.t.stR, hasTile ? J([t2.cls, t2.st]) : NO);
  if (hasTile) { await p.click(TILE); await p.waitForTimeout(500); }
  const l3 = await look();
  z.check('второе нажатие второй раз не запускает: «уже работает», команда агенту по-прежнему одна',
    hasTile && same(ag.launches, ['browser']) && l3.toast === cat.t.again, hasTile ? J([ag.launches, l3.toast]) : NO);
  ag.running = null; ag.apps = { browser: { available: false, reason: 'target_missing' } };
  await poll();
  const l4 = await look(), t4 = tiles(l4).find(k => k.id === 'browser') || {};
  if (hasTile) { await p.click(TILE); await p.waitForTimeout(500); }
  const l5 = await look();
  z.check('если Chrome на этом ПК не установлен — плитка погашена («недоступно»), нажатие объясняет почему и ничего не запускает',
    hasTile && t4.cls === 'st-off' && t4.st === cat.t.stOff && l5.toast === cat.t.missing && l5.toast.includes(CH) && same(ag.launches, ['browser']),
    hasTile ? J([t4.cls, t4.st, l5.toast, ag.launches]) : NO);
  ag.apps = {};
  await poll();
  await settle();
  const l6 = await look(), t6 = tiles(l6).find(k => k.id === 'browser') || {};
  z.check('агент снова видит Chrome — плитка обычная, без пометок', hasTile && t6.cls === '' && t6.st === '' && onMain(l6), hasTile ? J([t6.cls, t6.st]) : NO);

  console.log('— 3. Папки «Войсчат» и «Системные»: вход и выходы');
  const openFolder = async name => { await appsRow().locator('.folder', { hasText: name }).first().click(); await p.waitForTimeout(500); return look(); };
  const v1 = await openFolder('Войсчат');
  await p.keyboard.press('Escape'); await p.waitForTimeout(500);
  const v2 = await look();
  z.check('папка «Войсчат» открывается, внутри Discord; Esc возвращает на главный экран',
    v1.top === 'Войсчат' && same(v1.inside, ['discord']) && onMain(v2), J([v1.top, v1.inside, v2.rows, v2.open]));
  const s1 = await openFolder('Системные');
  await p.locator('#catBody .folder-top .plat-btn').click(); await p.waitForTimeout(500);
  const s2 = await look();
  z.check('папка «Системные»: внутри четыре приложения; «← Назад» возвращает на главный экран',
    s1.top === 'Системные' && same(s1.inside, cat.sys) && s1.inside.length === 4 && onMain(s2), J([s1.top, s1.inside, s2.rows, s2.open]));
  const b1 = await openFolder('Войсчат');
  await mouseBack(); await p.waitForTimeout(500);
  const b2 = await look();
  z.check('боковая кнопка мыши «назад» возвращает из папки на главный экран', b1.top === 'Войсчат' && onMain(b2), J([b1.top, b2.rows, b2.open]));
  await settle();
  const b3 = await look();
  z.check('после возвратов ряд тот же: две папки и плитка Chrome', same(names(b3.kids), ['Войсчат', 'Системные', CH]) && tiles(b3).length === 1, J(names(b3.kids)));

  console.log('— 4. Вкладка «Приложения» и поиск — как были');
  await p.locator('button[onclick="catTab(\'apps\')"]').click(); await p.waitForTimeout(500);
  const tab = await p.evaluate(() => ({ open: catFolder && catFolder.t,
    sec: [...document.querySelectorAll('#catBody .plat-h')].map(h => [h.querySelector('h3').textContent.trim(),
      [...h.nextElementSibling.querySelectorAll('[data-app]')].map(e => e.dataset.app).join(',')]) }));
  await p.locator('#catBody .folder-top .plat-btn').click(); await p.waitForTimeout(500);
  const tab2 = await look();
  z.check('вкладка «Приложения»: разделы «Войсчат», «Системные», «Другие»; Chrome — в «Других»; «← Назад» возвращает',
    tab.open === 'apps' && same(tab.sec, [['Войсчат', 'discord'], ['Системные', cat.sys.join(',')], ['Другие', 'browser']]) && onMain(tab2), J(tab));
  await p.fill('#catSearch', 'chrome'); await p.waitForTimeout(700);
  const q1 = await p.evaluate(() => [...document.querySelectorAll('#catBody [data-app]')].map(e => e.dataset.app));
  await p.locator('.cat-search .x').click(); await p.waitForTimeout(600);
  const q2 = await look();
  z.check('поиск «chrome» находит Google Chrome; крестик возвращает главный экран', q1.includes('browser') && onMain(q2), J([q1, q2.rows, q2.open]));

  console.log('— 5. EN и PL, экраны, правило «без группы — плитки»');
  const WANT = { en: ['Voice chat', 'System'], pl: ['Czat głosowy', 'Systemowe'] }, OTHER = { en: 'Other', pl: 'Inne' };
  for (const l of ['en', 'pl']) {
    await p.evaluate(x => window.__k112.set(x), l); await settle();
    const s = await look(), f = folders(s), t = tiles(s);
    z.check(`${l.toUpperCase()}: папки «${WANT[l].join('» и «')}», папки «${OTHER[l]}» нет, плитка Google Chrome на месте`,
      same(names(f), WANT[l]) && t.length === 1 && t[0].name === CH && t[0].fit && !s.hscroll, J(names(s.kids)));
  }
  await p.evaluate(() => window.__k112.back()); await settle();
  const wide = [];
  for (const [wd, ht] of [[1920, 1080], [1440, 900], [1366, 768], [1280, 800]]) {
    await size(wd, ht);
    const s = await look(), t = tiles(s).find(k => k.id === 'browser'), f = folders(s);
    wide.push({ wd, ok: !!t && t.fit && t.inRow && !s.hscroll && f.every(k => k.w === t.w && k.h === t.h && k.t === t.t), names: names(s.kids), t: t ? [t.w, t.h, t.fit, t.inRow] : null, hs: s.hscroll });
    if (wd === 1366) await shotRow('row_1366');
  }
  z.check('на экранах 1920×1080, 1440×900, 1366×768 и 1280×800 плитка в ряду, размером с папку, имя целиком, вбок ничего не уезжает',
    wide.every(x => x.ok), J(wide));
  await size(1280, 800);
  await p.evaluate(() => { CATALOG.apps = window.__k112.apps0.filter(a => a.sub); renderCatalog(); }); await settle();
  const r0 = await look();
  z.check('приложений без группы нет — в ряду одни папки, ни папки «Другие», ни пустой плитки',
    onMain(r0) && same(names(r0.kids), ['Войсчат', 'Системные']) && tiles(r0).length === 0, J(names(r0.kids)));
  await p.evaluate(() => {
    CATALOG.apps = window.__k112.apps0.concat(['Блокнот', 'Калькулятор', 'Проигрыватель с длинным именем'].map((n, k) => ({ id: 'k112_' + k, name: n, em: '🧪', sub: '' })));
    renderCatalog();
  }); await settle();
  const r4 = await look(), r4t = tiles(r4);
  z.check('приложений без группы четыре — четыре плитки одного размера с папками, папки «Другие» нет, на 1280×800 ряд переносится, а не уезжает вбок',
    onMain(r4) && same(names(folders(r4)), ['Войсчат', 'Системные']) && r4t.length === 4 && r4t.every(k => k.w === r4.kids[0].w && k.h === r4.kids[0].h && k.inRow) && !r4.hscroll,
    J(r4.kids.map(k => [k.name, k.w, k.h, k.t, k.inRow])) + ' ' + r4.hscroll);
  await shotRow('row_4');
  await p.evaluate(() => { CATALOG.apps = window.__k112.apps0; renderCatalog(); });
  await size(1920, 1080);

  const fin = await look(), finS = await p.evaluate(() => ({ lang: LANG, apps: CATALOG.apps.map(a => a.id) }));
  z.check('после проверки киоск как был: по-русски, каталог прежний, главный экран', finS.lang === 'ru' && same(finS.apps, cat.apps.map(a => a[0])) && onMain(fin), J(finS));
  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
