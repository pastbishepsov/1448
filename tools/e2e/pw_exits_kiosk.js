// Выходы киоска гостя (web/shell.html + калибратор web/aim.html), docs/ВЫХОДЫ-НАЗАД.md: Esc, боковая кнопка
// «назад» мыши (CDP button:'back' — как у гостя в Chromium/WebView2), клавиша BrowserBack. Проверяет каждый
// выход: лок-экран и его окна, витрина (папка, вкладка, поиск), окна шапки, чат, профиль с правками
// («Выйти без сохранения?»), кейс (во время ленты — ничего, после приза — «Забрать»), калибратор в iframe,
// глухие экраны, итоги сессии, заставку; «назад» не уводит со страницы и не растит историю.
// Гость ExitKiosk (регистрируется сам), ПК PC-06 через ?pc=; в конце сессия гостя закрыта.
const L = require('./_exits');
const G = process.env.G || 'ExitKiosk', PC = process.env.PC || '00000000-0000-0000-0000-000000000006';
const vis = (p, id) => p.evaluate(id => { const e = document.getElementById(id); return !!e && getComputedStyle(e).display !== 'none'; }, id);
(async () => {
  await L.ensureMoney(G, 150); await L.endSessions(G);
  const uid = await L.ensureGuest(G);
  const b = await L.browser(); const ctx = await b.newContext({viewport: {width: 1440, height: 900}});
  const p = await ctx.newPage();
  const errs = []; p.on('pageerror', e => errs.push(e.message));
  p.on('console', m => { if (m.type() === 'error' && !L.quietConsole(m.text())) errs.push('console: ' + m.text()); });
  await p.goto(L.BASE + '/shell?pc=' + PC); await p.waitForTimeout(700);
  const url0 = p.url();
  await p.fill('#nick', G); await p.fill('#pass', 'Guest12345'); await p.click('#authBtn');
  await p.waitForFunction(() => document.getElementById('lockOverlay').classList.contains('show'), null, {timeout: 8000});
  const hist0 = await p.evaluate(() => history.length);
  console.log('— лок-экран');
  await L.esc(p); L.check('лок: Esc — лок на месте', await vis(p, 'lockOverlay'));
  await L.mouseBack(p); L.check('лок: «назад» — лок на месте', await vis(p, 'lockOverlay'));
  L.check('лок: «назад» не увёл со страницы', p.url() === url0, p.url());
  await p.click('#lockOverlay .lock-info a'); await p.waitForTimeout(250);
  L.check('лок → правила открыты', await vis(p, 'infoOverlay'));
  await L.esc(p); L.check('правила: Esc закрыл', !await vis(p, 'infoOverlay'));
  await p.click('#lockOverlay .lock-info a'); await p.waitForTimeout(250);
  await L.mouseBack(p); L.check('правила: «назад» закрыл', !await vis(p, 'infoOverlay'));
  L.check('после правил лок на месте', await vis(p, 'lockOverlay'));
  await p.click('#lockOverlay button[onclick="openPackShop()"]'); await p.waitForTimeout(600);
  L.check('витрина пакетов открыта', await vis(p, 'packOverlay'));
  await L.esc(p); L.check('пакеты: Esc закрыл', !await vis(p, 'packOverlay'));
  await p.click('#lockOverlay button[onclick="openPackShop()"]'); await p.waitForTimeout(600);
  await L.mouseBack(p); L.check('пакеты: «назад» закрыл', !await vis(p, 'packOverlay'));
  await p.click('#lockOverlay button[onclick="openPackShop()"]'); await p.waitForFunction(() => !!document.querySelector('#packOverlayBody button[onclick^="buyPack"]'), null, {timeout: 6000});
  await p.click('#packOverlayBody button[onclick^="buyPack"]'); await p.waitForTimeout(300);
  L.check('покупка → подтверждение поверх витрины', await vis(p, 'confirmOverlay') && await vis(p, 'packOverlay'));
  await L.esc(p); L.check('подтверждение: Esc = «Отмена», витрина осталась', !await vis(p, 'confirmOverlay') && await vis(p, 'packOverlay'));
  await p.click('#packOverlayBody button[onclick^="buyPack"]'); await p.waitForTimeout(300);
  await L.mouseBack(p); L.check('подтверждение: «назад» = «Отмена»', !await vis(p, 'confirmOverlay') && await vis(p, 'packOverlay'));
  await L.mouseBack(p); L.check('второй «назад» закрыл витрину', !await vis(p, 'packOverlay'));

  console.log('— сессия и витрина игр');
  await p.click('#lockStart');
  await p.waitForFunction(() => !document.getElementById('lockOverlay').classList.contains('show'), null, {timeout: 8000});
  if (await vis(p, 'readyOverlay')){ await p.click('#readyBtn'); await p.waitForTimeout(800); }
  await p.waitForTimeout(600);
  await p.click('.folder:has-text("Steam")'); await p.waitForTimeout(400);
  L.check('папка Steam открыта', await p.evaluate(() => !!document.querySelector('.folder-top')));
  await L.mouseBack(p); L.check('папка: «назад» мыши → витрина', await p.evaluate(() => !document.querySelector('.folder-top') && !catFolder));
  L.check('«назад» не увёл со страницы', p.url() === url0, p.url());
  await p.click('.folder:has-text("Steam")'); await p.waitForTimeout(400);
  await L.esc(p); L.check('папка: Esc → витрина', await p.evaluate(() => !document.querySelector('.folder-top')));
  await p.click('.cat-tabs button:has-text("Steam")'); await p.waitForTimeout(400);
  L.check('вкладка Steam выбрана', await p.evaluate(() => !!catFolder && catFolder.t === 'plat'));
  await L.mouseBack(p); L.check('вкладка: «назад» → «Все»', await p.evaluate(() => !catFolder && document.querySelector('.cat-tabs button').classList.contains('on')));
  await p.click('.folder:has-text("Steam")'); await p.waitForTimeout(400);
  await p.click('#catSearch'); await p.keyboard.type('counter'); await p.waitForTimeout(300);
  L.check('поиск внутри папки', await p.evaluate(() => catQuery === 'counter'));
  await L.esc(p);
  L.check('поиск: Esc чистит поиск и снимает фокус, папка на месте', await p.evaluate(() => catQuery === '' && !!catFolder && document.activeElement.id !== 'catSearch'));
  await L.esc(p); L.check('следующий Esc закрывает папку', await p.evaluate(() => !catFolder));
  await p.click('#catSearch'); await p.keyboard.type('dota'); await p.waitForTimeout(300);
  await L.mouseBack(p); L.check('поиск на витрине: «назад» чистит', await p.evaluate(() => catQuery === ''));

  console.log('— окна шапки');
  await p.click('#rankPill'); await p.waitForTimeout(300);
  L.check('ранги открыты', await vis(p, 'rankOverlay'));
  await L.esc(p); L.check('ранги: Esc закрыл', !await vis(p, 'rankOverlay'));
  await p.click('#rankPill'); await p.waitForTimeout(300);
  await L.mouseBack(p); L.check('ранги: «назад» закрыл', !await vis(p, 'rankOverlay'));
  await p.click('.topbar button[onclick="openInfo()"]'); await p.waitForTimeout(250);
  await L.mouseBack(p); L.check('ⓘ правила: «назад» закрыл', !await vis(p, 'infoOverlay'));
  await p.evaluate(() => openOdds(null, '', '')); await p.waitForTimeout(500);
  L.check('шансы открыты', await vis(p, 'oddsOverlay'));
  await L.esc(p); L.check('шансы: Esc закрыл', !await vis(p, 'oddsOverlay'));
  // BrowserBack (мультимедийная клавиша / драйвер мыши шлёт её вместо кнопки)
  await p.click('#rankPill'); await p.waitForTimeout(300);
  await L.browserBackKey(p);
  L.check('BrowserBack закрыл ранги', !await vis(p, 'rankOverlay'));
  L.check('BrowserBack не увёл со страницы', p.url() === url0, p.url());

  console.log('— чат со стойкой');
  await p.click('.topbar button[onclick="toggleChat()"]'); await p.waitForTimeout(300);
  await p.fill('#chatText', 'черновик');
  await p.focus('#chatText'); await L.esc(p);
  L.check('чат: Esc из поля ввода закрыл панель', !await vis(p, 'chatPanel'));
  await p.click('.topbar button[onclick="toggleChat()"]'); await p.waitForTimeout(300);
  L.check('чат: черновик на месте', await p.inputValue('#chatText') === 'черновик');
  await L.mouseBack(p); L.check('чат: «назад» закрыл', !await vis(p, 'chatPanel'));
  await p.evaluate(() => { document.getElementById('chatText').value = ''; });

  console.log('— профиль (правки → вопрос)');
  await p.click('#ava'); await p.waitForTimeout(300);
  L.check('профиль открыт', await vis(p, 'profOverlay'));
  await L.esc(p); L.check('профиль без правок: Esc закрыл сразу', !await vis(p, 'profOverlay') && !await vis(p, 'confirmOverlay'));
  await p.click('#ava'); await p.waitForTimeout(300);
  const nick0 = await p.inputValue('#pNick');
  await p.fill('#pNick', nick0 + 'x');
  await L.esc(p);
  L.check('правки: Esc → вопрос поверх профиля', await vis(p, 'confirmOverlay') && await vis(p, 'profOverlay'));
  const q = await p.evaluate(() => ({t: document.getElementById('cfTitle').textContent, yes: document.getElementById('cfYes').textContent,
    no: document.querySelector('#confirmOverlay .btn.ghost').textContent}));
  L.check('вопрос: «Выйти без сохранения?» / Выйти / Остаться', q.t === 'Выйти без сохранения?' && q.yes === 'Выйти' && q.no === 'Остаться', q);
  await L.esc(p); L.check('Esc на вопросе = «Остаться»: профиль открыт, правка цела', !await vis(p, 'confirmOverlay') && await vis(p, 'profOverlay') && await p.inputValue('#pNick') === nick0 + 'x');
  await L.mouseBack(p); L.check('«назад» с правками → снова вопрос', await vis(p, 'confirmOverlay'));
  await p.click('#confirmOverlay .btn.ghost'); await p.waitForTimeout(200);
  L.check('«Остаться» кнопкой — профиль открыт', await vis(p, 'profOverlay') && !await vis(p, 'confirmOverlay'));
  await p.mouse.click(30, 450); await p.waitForTimeout(250); // клик мимо окна
  L.check('клик мимо с правками → вопрос', await vis(p, 'confirmOverlay'));
  await p.click('#cfYes'); await p.waitForTimeout(250);
  L.check('«Выйти» — профиль закрыт', !await vis(p, 'profOverlay'));
  await p.click('#ava'); await p.waitForTimeout(300);
  L.check('правки не сохранились (ник прежний)', await p.inputValue('#pNick') === nick0);
  L.check('после вопроса кнопка отказа снова «Отмена» в общем окне', await p.evaluate(() => { askConfirm('t', 's'); const t = document.querySelector('#confirmOverlay .btn.ghost').textContent; confirmAnswer(false); return t; }) === 'Отмена');
  await p.evaluate(() => { const g = (CATALOG.games || [])[0]; if (g) pickFav(g.id); });
  await L.mouseBack(p); L.check('любимая игра — тоже правка: «назад» спрашивает', await vis(p, 'confirmOverlay'));
  await L.esc(p);
  await p.click('#profOverlay .pbtns .btn.ghost'); await p.waitForTimeout(200);
  L.check('«Отмена» профиля закрывает без вопроса (как раньше)', !await vis(p, 'profOverlay') && !await vis(p, 'confirmOverlay'));

  console.log('— новый уровень');
  await p.evaluate(() => showLevelUp(5)); await p.waitForTimeout(200);
  await L.mouseBack(p); L.check('новый уровень: «назад» закрыл', !await vis(p, 'lvlOverlay'));

  console.log('— кейс: во время ленты — ничего, после приза — «Забрать»');
  const gr = await L.api('POST', '/admin/users/' + uid + '/grant', {type: 'case', case_tier: 'light', reason: 'проверка выходов'}, await L.tok('boss'));
  L.check('кейс выдан', gr.status < 300, gr);
  await p.evaluate(() => loadAll()); await p.evaluate(() => sideTab('cases')); await p.waitForTimeout(900);
  await p.click('#sideBody .case-i:not(.opened)'); await p.waitForTimeout(700);
  L.check('лента крутится', await vis(p, 'caseOverlay') && await p.evaluate(() => document.getElementById('coResult').classList.contains('hidden')));
  await L.esc(p); L.check('Esc во время ленты — окно на месте', await vis(p, 'caseOverlay'));
  await L.mouseBack(p); L.check('«назад» во время ленты — окно на месте', await vis(p, 'caseOverlay'));
  await p.waitForFunction(() => !document.getElementById('coResult').classList.contains('hidden'), null, {timeout: 12000});
  await p.screenshot({path: L.OUT + '/k_case_prize.png'});
  await L.mouseBack(p); L.check('после приза «назад» = «Забрать»', !await vis(p, 'caseOverlay'));
  await p.evaluate(() => loadAll()); await p.waitForTimeout(600);

  console.log('— калибратор (iframe)');
  await p.evaluate(() => sideTab('sens')); await p.waitForTimeout(400);
  const openAim = async () => { await p.click('#sideBody button[onclick="openAimCal()"]'); await p.waitForFunction(() => { const f = document.getElementById('aimFrame'); return f && f.dataset.ready; }, null, {timeout: 8000}); await p.waitForTimeout(300); };
  await openAim(); L.check('калибратор открыт', await vis(p, 'aimOverlay'));
  await L.esc(p); L.check('калибратор: Esc (фокус в шелле) закрыл', !await vis(p, 'aimOverlay'));
  await openAim();
  const fr = () => p.frame({url: /aim\.html/});
  await fr().click('#setup .card h1, #setup .card .lede, #setup .card', {position: {x: 10, y: 10}}).catch(() => {});
  L.check('фокус внутри калибратора', await p.evaluate(() => document.activeElement && document.activeElement.id === 'aimFrame'));
  await L.esc(p); L.check('калибратор: Esc (фокус внутри) закрыл', !await vis(p, 'aimOverlay'));
  await openAim();
  await L.mouseBack(p, 700, 450); L.check('калибратор: «назад» мыши над iframe закрыл', !await vis(p, 'aimOverlay'));
  L.check('калибратор: страница не ушла', p.url() === url0, p.url());
  await openAim();
  await fr().click('#setup details summary'); await fr().click('#calBtn'); await p.waitForTimeout(300);
  L.check('калибровка свайпом открыта', await fr().evaluate(() => document.getElementById('calib').classList.contains('on')));
  await L.mouseBack(p, 700, 450);
  L.check('«назад»: калибровка → настройки, калибратор открыт', await vis(p, 'aimOverlay') && await fr().evaluate(() => !document.getElementById('calib').classList.contains('on')));
  await fr().click('#startBtn'); await p.waitForTimeout(500);
  L.check('арена калибратора', await fr().evaluate(() => document.getElementById('session').classList.contains('on')));
  await L.esc(p);
  L.check('Esc на арене (без захвата) → настройки', await vis(p, 'aimOverlay') && await fr().evaluate(() => document.getElementById('setup').classList.contains('on')));
  await L.mouseBack(p, 700, 450); L.check('«назад» с настроек закрыл калибратор', !await vis(p, 'aimOverlay'));
  await p.evaluate(() => aimMeasureDPI()); await p.waitForFunction(() => { const f = document.getElementById('aimFrame'); return f && f.dataset.ready; }, null, {timeout: 8000});
  await L.mouseBack(p, 700, 450); L.check('замер DPI: «назад» закрыл', !await vis(p, 'aimOverlay'));
  L.check('история не выросла и не ушла', p.url() === url0 && await p.evaluate(() => history.length) === hist0, [p.url(), hist0, await p.evaluate(() => history.length)]);

  console.log('— глухие экраны (симуляция слоя)');
  // ноль кошелька — как у живой сессии: zero_since в состоянии, оверлей рисует сам шелл (тик раз в секунду)
  for (const id of ['zeroOverlay', 'readyOverlay', 'pwdOverlay']){
    await p.click('#rankPill'); await p.waitForTimeout(250);          // окно под глухим экраном
    if (id === 'zeroOverlay'){ await p.evaluate(() => { activeSession.zero_since = new Date().toISOString(); }); await p.waitForTimeout(1200); }
    else await p.evaluate(id => document.getElementById(id).classList.remove('hidden'), id);
    await L.esc(p); await L.mouseBack(p);
    L.check(id + ': Esc и «назад» не закрывают ни его, ни окно под ним', await vis(p, id) && await vis(p, 'rankOverlay'));
    if (id === 'zeroOverlay'){ await p.evaluate(() => { delete activeSession.zero_since; }); await p.waitForTimeout(1200); }
    else await p.evaluate(id => document.getElementById(id).classList.add('hidden'), id);
    await L.esc(p);
    L.check(id + ' ушёл — Esc снова закрывает окна', !await vis(p, id) && !await vis(p, 'rankOverlay'));
  }

  console.log('— конец сессии: «назад» = «Ок»');
  await p.click('.topbar button[onclick^="logoutClick"]'); await p.waitForTimeout(200);
  await p.click('.topbar button[onclick^="logoutClick"]');
  await p.waitForFunction(() => document.getElementById('endOverlay').classList.contains('show'), null, {timeout: 8000});
  await p.waitForTimeout(600);
  await L.esc(p); if (await vis(p, 'lvlOverlay')) await L.esc(p);
  await p.waitForTimeout(300);
  L.check('итоги: Esc = «Ок» → выход из аккаунта (своё завершение)', !await vis(p, 'endOverlay') && await p.evaluate(() => !document.getElementById('authView').classList.contains('hidden')));

  console.log('— заставка');
  await p.fill('#nick', G); await p.fill('#pass', 'Guest12345'); await p.click('#authBtn');
  await p.waitForFunction(() => document.getElementById('lockOverlay').classList.contains('show'), null, {timeout: 8000});
  await p.evaluate(() => attractShow()); await p.waitForTimeout(300);
  L.check('заставка на экране', await vis(p, 'attract'));
  await L.mouseBack(p);
  L.check('«назад» только гасит заставку', !await vis(p, 'attract') && p.url() === url0);
  await p.evaluate(() => attractShow()); await p.waitForTimeout(200);
  await L.esc(p); L.check('Esc гасит заставку', !await vis(p, 'attract'));
  L.check('ошибок JS нет', errs.length === 0, errs);
  await b.close(); await L.endSessions(G); L.summary();
})().catch(e => { console.error(e); process.exit(1); });
