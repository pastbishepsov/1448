// К11-и6 «Подписи запуска без рода» — проверка в браузере (Chrome без окна): Guest46 за PC-07, сессия идёт.
// Агент ПК подставной, как в pw_k11_apps: отвечает на /ping, /apps-status, /launch и запоминает команды запуска.
// На главном экране берём плитку игры (первая в ряду «Игры») и плитку приложения (Google Chrome) и на трёх языках
// смотрим всё, что киоск пишет про запуск: подпись на плитке и всплывающую строку после нажатия.
// Подписи безличные и одни на игру и на приложение: «работает», «недоступно», «… уже работает на этом ПК»,
// «…: нет на этом ПК», «…: запуск на этом ПК не разрешён», «…: запуск не подтвердился». По-польски — так же
// («działa», «niedostępne», «brak na tym PC»). По-английски рода нет: строки прежние, проверяем, что они на месте.
// Срок «процесс так и не появился» в киоске 45 секунд; на одно нажатие проверка сжимает его до 0,3 секунды.
// В базе ничего не меняет: гостя сажает и сессию закрывает сама, кошелёк возвращает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;
const CH = 'Google Chrome';
const WANT = {
  ru: { run: 'работает', off: 'недоступно',
    again: n => `✅ ${n} уже работает на этом ПК`, missing: n => `🚫 ${n}: нет на этом ПК`,
    denied: n => `🚫 ${n}: запуск на этом ПК не разрешён`, other: n => `🚫 ${n}: сейчас недоступно на этом ПК`,
    started: n => `✅ ${n} работает`,
    noconf: n => `⚠ ${n}: запуск не подтвердился. Если экран не сменился — позови админа` },
  en: { run: 'running', off: 'unavailable',
    again: n => `✅ ${n} is already running on this PC`, missing: n => `🚫 ${n} is not installed on this PC`,
    denied: n => `🚫 ${n} is not allowed on this PC`, other: n => `🚫 ${n} is unavailable on this PC right now`,
    started: n => `✅ ${n} is running`,
    noconf: n => `⚠ ${n} did not confirm the start. If the screen did not change, call the admin` },
  pl: { run: 'działa', off: 'niedostępne',
    again: n => `✅ ${n} już działa na tym PC`, missing: n => `🚫 ${n}: brak na tym PC`,
    denied: n => `🚫 ${n}: uruchamianie na tym PC jest niedozwolone`, other: n => `🚫 ${n}: teraz niedostępne na tym PC`,
    started: n => `✅ ${n} działa`,
    noconf: n => `⚠ ${n}: brak potwierdzenia startu. Jeśli ekran się nie zmienił — zawołaj admina` },
};
// все строки киоска про запуск и слова, которыми раньше подпись подстраивалась под «игра» (женский род)
const KEYS = ['launching', 'launch_agent_err', 'launch_agent_err_def', 'launch_on_pc', 'launch_failed', 'st_unavail', 'st_launching',
  'st_running', 'launch_wait', 'launch_started', 'launch_running', 'launch_noconfirm', 'na_path_unreachable', 'na_target_missing',
  'na_not_allowed', 'na_default'];
const OLD = { ru: ['запущена', 'недоступна', 'установлена', 'разрешена', 'подтвердила'],
  pl: ['uruchomiona', 'niedostępna', 'zainstalowana', 'dozwolona', 'potwierdziła'] };

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
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k11_launch: проверка вернула кошелёк', now() FROM u WHERE u.w <> ${v}`);
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
  const away = async ms => { await p.mouse.move(4, 4); await p.waitForTimeout(ms); }; // мышь в стороне, плитка без наведения
  const poll = () => p.evaluate(async () => { // дождаться своего опроса агента, а не чужого
    for (let k = 0; k < 60 && appsInFlight; k++) await new Promise(r => setTimeout(r, 50));
    await pollAppsStatus();
  });
  const SEL = id => `#catBody > .games > [data-app="${id}"], #catBody > .folders > [data-app="${id}"]`;
  // плитка на главном экране: класс состояния и подпись на ней
  const tile = id => p.evaluate(s => {
    const e = document.querySelector(s);
    if (!e) return null;
    const b = e.querySelector(':scope > .gstate');
    return { cls: (e.className.match(/st-\w+/) || [''])[0], st: b ? b.textContent.replace(/\s+/g, ' ').trim() : '' };
  }, SEL(id));
  const toastNow = () => p.evaluate(() => { const t = document.getElementById('toast'); return t.classList.contains('show') ? t.textContent : ''; });
  const press = async id => { await p.click(SEL(id)); await p.waitForTimeout(450); return toastNow(); };
  const shot = async (name, sel) => { // кадр: один блок с полями
    try {
      const c = await p.evaluate(s => {
        const e = document.querySelector(s);
        if (!e) return null;
        const b = e.getBoundingClientRect(), x = Math.max(0, b.left - 14), y = Math.max(0, b.top - 14);
        return { x, y, width: Math.min(innerWidth - x, b.width + 28), height: Math.min(innerHeight - y, b.height + 28) };
      }, sel);
      if (c) await p.screenshot({ path: `${z.OUT}/k11l_${name}.png`, clip: c });
    } catch (e) { console.log('кадр', name, e.message.split('\n')[0]); }
  };

  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
  await until(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'), null, 15000);
  await until(() => agentOnline === true && document.querySelectorAll('#catBody > .folders').length === 2, null, 10000);
  await away(750);
  const cat = await p.evaluate(keys => {
    window.__k116 = { keep: null,
      set(l) { this.keep = this.keep || { STR, LANG }; LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]); applyStrings(); renderCatalog(); },
      back() { if (this.keep) { LANG = this.keep.LANG; STR = this.keep.STR; this.keep = null; applyStrings(); renderCatalog(); } } };
    const first = document.querySelector('#catBody > .games > .game[data-app]'), gid = first ? first.dataset.app : '';
    const game = CATALOG.games.find(x => x.id === gid), app = CATALOG.apps.find(x => x.id === 'browser');
    const dict = l => keys.map(k => [k, String(Object.assign({}, SH_RU, SH_LANGS[l])[k] || '')]);
    return { online: agentOnline, lang: LANG, gid, gname: game ? appName(game.name) : '', aname: app ? appName(app.name) : '',
      appTile: !!document.querySelector('#catBody > .folders > .app[data-app="browser"]'), wait: LAUNCH_TIMEOUT,
      dict: { ru: dict('ru'), pl: dict('pl') } };
  }, KEYS);
  const GID = cat.gid, GN = cat.gname, ready = cat.online && cat.lang === 'ru' && !!GID && !!GN && cat.appTile && cat.aname === CH;
  z.check('агент ПК на связи; на главном экране есть плитка игры и плитка Google Chrome', ready,
    J([cat.online, cat.lang, GID, GN, cat.appTile, cat.aname]));

  // один проход по плитке: что написано на ней и что всплывает после нажатия в каждом состоянии
  const walk = async (id, tag) => {
    const o = {}, n0 = ag.launches.length;
    ag.apps = {}; ag.running = { app_id: id, pid: 4242 }; await poll();
    o.run = await tile(id);
    o.again = await press(id);
    if (tag) { await away(250); await shot(`${tag}_run`, SEL(id)); await shot(`${tag}_run_toast`, '#toast'); }
    ag.running = null; ag.apps = { [id]: { available: false, reason: 'target_missing' } }; await poll();
    o.off = await tile(id);
    o.missing = await press(id);
    if (tag) { await away(250); await shot(`${tag}_off`, SEL(id)); await shot(`${tag}_off_toast`, '#toast'); }
    ag.apps = { [id]: { available: false, reason: 'not_allowed' } }; await poll();
    o.denied = await press(id);
    ag.apps = { [id]: { available: false, reason: 'k116_other' } }; await poll();
    o.other = await press(id);
    o.idle = ag.launches.length - n0; // пока плитка работала или была погашена, агента не трогали
    ag.apps = {}; await poll();
    const n1 = ag.launches.length;
    o.first = await press(id); // «🚀 Запускаю …»
    o.mid = await tile(id);
    ag.running = { app_id: id, pid: 4242 }; await poll(); // агент увидел процесс
    o.started = await toastNow();
    o.sent1 = ag.launches.slice(n1);
    ag.running = null; await poll();
    const n2 = ag.launches.length;
    await p.evaluate(() => { // срок ожидания процесса — 0,3 с вместо 45 с, только на это нажатие
      const st = window.setTimeout; window.__k116st = st;
      window.setTimeout = (fn, ms, ...a) => st(fn, ms === LAUNCH_TIMEOUT ? 300 : ms, ...a);
    });
    await p.click(SEL(id)); await p.waitForTimeout(900);
    await p.evaluate(() => { if (window.__k116st) { window.setTimeout = window.__k116st; delete window.__k116st; } });
    o.noconf = await toastNow();
    o.sent2 = ag.launches.slice(n2);
    o.end = await tile(id);
    await away(150);
    return o;
  };
  const NONE = { run: {}, off: {}, mid: {}, end: {} };
  for (const l of ['ru', 'en', 'pl']) {
    const L = l.toUpperCase(), W = WANT[l];
    console.log(`— ${L}: игра «${GN}» и приложение «${CH}»`);
    if (l !== 'ru') { await p.evaluate(x => window.__k116.set(x), l); await away(750); }
    const a = ready ? await walk(GID, l === 'en' ? '' : `${l}_game`) : NONE;
    const b = ready ? await walk('browser', l === 'en' ? '' : `${l}_app`) : NONE;
    const both = f => f(a, GN) && f(b, CH), see = k => J([a[k], b[k]]);
    z.check(`${L}: работающая плитка подписана «${W.run}» — и игра, и приложение`,
      both(o => o.run.cls === 'st-running' && o.run.st === W.run), see('run'));
    z.check(`${L}: нажатие по работающей — «${W.again('…')}», второй раз не запускает`,
      both((o, n) => o.again === W.again(n)) && a.idle === 0 && b.idle === 0, J([a.again, b.again, a.idle, b.idle]));
    z.check(`${L}: погашенная плитка подписана «${W.off}» — и игра, и приложение`,
      both(o => o.off.cls === 'st-off' && o.off.st === W.off), see('off'));
    z.check(`${L}: на ПК её нет — «${W.missing('…')}»`, both((o, n) => o.missing === W.missing(n)), see('missing'));
    z.check(`${L}: запуск закрыт настройкой — «${W.denied('…')}»`, both((o, n) => o.denied === W.denied(n)), see('denied'));
    z.check(`${L}: причина не названа — «${W.other('…')}»`, both((o, n) => o.other === W.other(n)), see('other'));
    z.check(`${L}: агент увидел процесс — «${W.started('…')}», команда запуска ушла одна`,
      both((o, n) => o.started === W.started(n) && o.mid.cls === 'st-launching') && J(a.sent1) === J([GID]) && J(b.sent1) === J(['browser']),
      J([a.started, b.started, a.mid, b.mid, a.sent1, b.sent1]));
    z.check(`${L}: процесс так и не появился — «${W.noconf('…').split('. ')[0]}…», плитка снова обычная`,
      both((o, n) => o.noconf === W.noconf(n) && o.end.cls === '' && o.end.st === '') && J(a.sent2) === J([GID]) && J(b.sent2) === J(['browser']),
      J([a.noconf, b.noconf, a.end, b.end, a.sent2, b.sent2]));
  }
  await p.evaluate(() => window.__k116.back()); await away(750);

  console.log('— Словари: слова женского рода в строках про запуск');
  for (const l of ['ru', 'pl']) {
    const hit = cat.dict[l].filter(([, v]) => OLD[l].some(w => v.includes(w))).map(([k, v]) => k + ': ' + v);
    z.check(`${l.toUpperCase()}: в строках про запуск нет слов «${OLD[l].join('», «')}»`, cat.dict[l].every(([, v]) => v) && hit.length === 0, J(hit.length ? hit : cat.dict[l].filter(([, v]) => !v)));
  }

  const fin = await p.evaluate(s => ({ lang: LANG, rows: document.querySelectorAll('#catBody > .folders').length, open: catFolder,
    st: [...document.querySelectorAll('#catBody .gstate')].length, real: !window.__k116st,
    tiles: s.map(q => !!document.querySelector(q)) }), [SEL(GID), SEL('browser')]);
  z.check('после проверки киоск как был: по-русски, главный экран, плитки без пометок',
    fin.lang === 'ru' && fin.rows === 2 && fin.open === null && fin.st === 0 && fin.real && fin.tiles.every(Boolean), J(fin));
  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
