// З15-и6 «Мелочи персонала — экран», браузер (№30; З-Р20). «Выйти» у админа на смене больше не выходит молча:
// открывается «Моя смена» с «⚠ Ты на смене с … — перед выходом сдай кассу» и тремя путями — «Сдать смену и выйти»,
// «Уйти без пересчёта и выйти», «Выйти, не закрывая смену». Esc — остаёшься в админке, «Моя смена» с чипа — как обычно.
// Выход без сдачи оставляет смену открытой (вход в те же сутки её продолжает), «Сдать смену и выйти» закрывает запись
// табеля и сдаёт кассу (пересчитано = должно быть — «сошлась»). Не на смене и у владельца — выход сразу. Телефон 360:
// шторка без прокрутки вбок, «Уйти без пересчёта и выйти». Нажимаются только видимые кнопки: на версии без шторки
// проверка не виснет, а проваливается. z15q — временный сотрудник: его открытые записи табеля удаляются в начале; денег
// не трогает (сдача — ровно «должно быть»).
const z = require('./_z');
(async () => {
  const Q = 'z15q';
  const qid = await z.hireTemp(Q);
  z.sql(`DELETE FROM work_entries WHERE user_id = '${qid}' AND ended_at IS NULL`);
  const since = z.sql('SELECT now()::text');
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const openCnt = () => z.sql(`SELECT count(*) FROM work_entries WHERE user_id = '${qid}' AND ended_at IS NULL`);
  const view = p => p.evaluate(() => ({
    app: !document.getElementById('appView').classList.contains('hidden'),
    login: !document.getElementById('loginView').classList.contains('hidden'),
    drawer: document.getElementById('drawer').classList.contains('open'),
    title: document.getElementById('drawerTitle').textContent }));
  const vis = (p, sel) => p.isVisible(sel).catch(() => false);
  const tap = async (p, sel, wait = 300) => { if (!await vis(p, sel)) return false; await p.click(sel); await z.sleep(wait); return true; };
  const out = p => tap(p, '#rail button[aria-label="Выйти"]', 1500);
  const btnTexts = p => p.evaluate(() => ['coSubmit', 'coSkip', 'msLeaveOpen'].map(i => {
    const b = document.getElementById(i); return b ? b.textContent.replace(/\s+/g, ' ').trim() : null; }));
  const relogin = async p => {
    if (!await vis(p, '#nick')) return;
    await p.fill('#nick', Q); await p.fill('#pass', z.STAFF_PASS); await p.keyboard.press('Enter');
    await p.waitForSelector('#rail', { state: 'visible', timeout: 10000 });
    await z.sleep(1500);
    await p.evaluate(() => { const t = document.getElementById('tour'); if (t && !t.classList.contains('hidden')) tourSkip(); });
  };

  const p = await z.openAdmin(Q); // вход в админку открывает смену
  const chip = norm(await z.text(p, '#shiftTxt'));
  z.check('вход z15q в админку открыл смену — в шапке «на смене с …»', /^на смене с \d\d:\d\d$/.test(chip) && openCnt() === '1', chip);
  await out(p);
  let v = await view(p);
  const warn = norm(await z.text(p, '#msLeave'));
  const btns = await btnTexts(p);
  const asked = v.app && !v.login && v.drawer && v.title === 'Моя смена' && /^⚠ Ты на смене с \d\d:\d\d — перед выходом сдай кассу\./.test(warn)
    && JSON.stringify(btns) === JSON.stringify(['Сдать смену и выйти', 'Уйти без пересчёта и выйти', 'Выйти, не закрывая смену']);
  z.check('«Выйти» на смене — не выход: «Моя смена» с «⚠ Ты на смене с … — перед выходом сдай кассу» и тремя путями — '
    + '«Сдать смену и выйти», «Уйти без пересчёта и выйти», «Выйти, не закрывая смену»', asked, JSON.stringify({ v, warn, btns }));
  await z.shot(p, 'z15_logout_ask');
  await p.evaluate(() => { const b = document.getElementById('drawerBody'); if (b) b.scrollTop = b.scrollHeight; });
  await z.shot(p, 'z15_logout_ask_bottom');
  await p.keyboard.press('Escape');
  await z.sleep(500);
  const vEsc = await view(p);
  await tap(p, '#shiftChip', 1500);
  v = await view(p);
  const plain = await btnTexts(p);
  const plainWarn = await p.evaluate(() => !!document.getElementById('msLeave'));
  z.check('Esc — остаёшься в админке, смена открыта; «Моя смена» с чипа — как обычно: «Сдать смену», без предупреждения о выходе',
    vEsc.app && !vEsc.drawer && v.app && v.drawer && !plainWarn && plain[0] === 'Сдать смену' && plain[1] === 'Уйти без пересчёта'
    && plain[2] === null && openCnt() === '1', JSON.stringify({ vEsc, plain, plainWarn }));
  await p.keyboard.press('Escape');
  await z.sleep(500);

  await out(p);
  await tap(p, '#msLeaveOpen');
  const armed = norm(await z.text(p, '#msLeaveOpen'));
  await tap(p, '#msLeaveOpen', 1500);
  v = await view(p);
  const leftOpen = armed === 'Ещё раз — выйти, смена останется открытой' && v.login && !v.app && openCnt() === '1';
  z.check('«Выйти, не закрывая смену» — со второго нажатия («Ещё раз — выйти, смена останется открытой»): экран входа, '
    + 'смена z15q открыта', leftOpen, JSON.stringify({ armed, v }));
  await relogin(p);
  const cnt = z.sql(`SELECT count(*) || '|' || count(*) FILTER (WHERE ended_at IS NULL) FROM work_entries WHERE user_id = '${qid}' AND started_at >= '${since}'`);
  z.check('вышел, не закрывая, и вошёл снова в те же сутки — та же смена продолжается: запись табеля с начала проверки одна, '
    + 'открытая', leftOpen && cnt === '1|1', cnt);

  await out(p);
  const expected = await p.evaluate(() => typeof closeoutPrev !== 'undefined' && closeoutPrev && !closeoutPrev.error
    ? (closeoutPrev.baseline ? 500 : closeoutPrev.expected_pln) : null);
  if (expected != null && await vis(p, '#coCounted')) await p.fill('#coCounted', String(expected));
  await z.sleep(200);
  await tap(p, '#coSubmit');
  const armed2 = norm(await z.text(p, '#coSubmit'));
  const t = norm(await z.toastAfter(p, () => tap(p, '#coSubmit', 0), 'Смена закрыта')); // З19-и13: тост этого действия, а не #toast через паузу
  v = await view(p);
  const row = z.sql(`SELECT (ended_at IS NOT NULL)::text || '|' || auto_closed::text FROM work_entries WHERE user_id = '${qid}' AND started_at >= '${since}'`);
  const co = z.sql(`SELECT skipped::text || '|' || COALESCE(diff_grosz::text, '') FROM shift_closeouts WHERE user_id = '${qid}' AND created_at >= '${since}' ORDER BY created_at DESC LIMIT 1`);
  z.check('«Сдать смену и выйти» (пересчитано = должно быть) — со второго нажатия: тост «Смена закрыта · … · касса сошлась», '
    + 'экран входа, запись табеля закрыта сама, сдача без расхождения',
    expected != null && armed2 === 'Ещё раз — сдать смену и выйти' && v.login && !v.app
    && /^Смена закрыта · \d+ ч \d\d м · (касса сошлась|начальный остаток ящика)/.test(t) && row === 'true|false' && co === 'false|0',
    JSON.stringify({ expected, armed2, v, t, row, co }));

  await relogin(p); // вход открывает новую смену — снимаем её «из табеля», пока он в админке
  z.sql(`DELETE FROM work_entries WHERE user_id = '${qid}' AND ended_at IS NULL`);
  await out(p);
  v = await view(p);
  z.check('не на смене (смену сняли, пока он был в админке) — «Выйти» выходит сразу, без шторки (на смене — шторка)',
    asked && v.login && !v.app && !v.drawer, JSON.stringify(v));

  await p.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const boss = await z.openAdmin('boss');
  await out(boss);
  v = await view(boss);
  z.check('владелец (своей смены нет) — «Выйти» выходит сразу (у админа на смене — шторка)', asked && v.login && !v.app && !v.drawer,
    JSON.stringify(v));

  await boss.goto('about:blank');
  await z.sleep(2500);
  const ph = await z.openAdmin(Q, { viewport: { width: 360, height: 780 } }); // снова на смене
  await out(ph);
  const fit = await ph.evaluate(() => { const b = document.getElementById('msLeave'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(ph, 'z15_logout_ask_phone');
  await tap(ph, '#coSkip');
  await tap(ph, '#coSkip', 2000);
  v = await view(ph);
  const skip = z.sql(`SELECT skipped::text FROM shift_closeouts WHERE user_id = '${qid}' AND created_at >= '${since}' ORDER BY created_at DESC LIMIT 1`);
  z.check('телефон 360: шторка выхода без прокрутки вбок; «Уйти без пересчёта и выйти» — смена закрыта без пересчёта, экран входа',
    fit.sw <= 360 && fit.box && v.login && !v.app && skip === 'true' && openCnt() === '0', JSON.stringify({ fit, v, skip }));
  await z.done();
})().catch(z.crash);
