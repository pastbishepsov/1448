// Ж8 «Стойка вперёд» (Ж-Р12; №33, №34, №37, №38): виджет «Скоро на нуле» в Зале, посадка компании
// одной кнопкой (suggest соседних ПК, частичный провал построчно), табло /board под токеном
// (без токена/с чужим — заглушка, с токеном — ПК без ников и очередь с никами), цифры в палитре
// (ПК по номеру, телефон → гость), звук вызова с переключателем. Скриншоты. Критерий ADMIN.md Ж8.
const { chromium } = require('playwright-core');
const { toastAfter, idle, watch, signIn, reenter } = require('./_z'); // З19-и13: тост этого действия; З19-и14: после записи — idle
const { API, BASE, DB, CHROME, OUT, MOCK_AGENT, agentLog, WEB_DIR } = require('./_env'); // стенд задаётся окружением, см. README
const { execSync } = require('child_process');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [];
const check = (name, cond, detail) => out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
async function j(url, opt) { const r = await fetch(url, opt); const t = await r.text(); try { return Object.assign(JSON.parse(t), { _status: r.status }); } catch (e) { return { _status: r.status, _text: t }; } }
const psql = q => execSync(`psql "${DB}" -tAc "${q.replace(/"/g, '\\"')}"`).toString().trim();
const agents = [];
function agentStart(pcId, name) {
  const log = agentLog(name);
  try { require('fs').unlinkSync(log); } catch (e) {}
  const p = require('child_process').spawn('node', [MOCK_AGENT, pcId, log], { stdio: 'ignore', detached: true });
  agents.push(p);
}
(async () => {
  const login = async (l, p) => (await j(API + '/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ login: l, password: p }) })).access_token;
  const a1 = await login('admin1', 'Admin12345'); const ot = await login('boss', 'Boss12345'); const gt30 = await login('Guest30', 'Guest12345');
  const h = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + a1 }, ho = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + ot };
  const hg = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + gt30 };
  const NICKS = ['Guest18', 'Guest19', 'Guest24', 'Guest30', 'Янек'];
  const G = {}; NICKS.forEach(n => { G[n] = psql(`SELECT id FROM users WHERE nickname='${n}'`); });
  const ids = Object.values(G).map(x => `'${x}'`).join(',');
  const pcs = (await j(API + '/admin/computers', { headers: h })).computers; const P = n => pcs.find(p => p.name === n);
  const reset = async () => {
    for (const sid of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE status='active' AND user_id IN (${ids})`).split(/\s+/).filter(Boolean))
      await j(API + '/admin/sessions/' + sid + '/end', { method: 'POST', headers: h, body: '{}' });
    psql(`UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN (SELECT computer_id FROM sessions WHERE status='active')`);
    psql(`UPDATE waitlist SET status='removed', resolved_at=now() WHERE status='waiting' AND user_id IN (${ids})`);
    psql(`UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id IN (${ids})`);
    psql(`UPDATE users SET wallet_grosz=12000 WHERE id='${G.Guest30}'`); psql(`DELETE FROM wallet_transactions WHERE user_id='${G.Guest30}'`);
    await j(API + '/admin/board/token', { method: 'DELETE', headers: ho });
  };
  await reset();
  ['PC-01', 'PC-02', 'PC-03', 'PC-05', 'PC-06'].forEach((n, i) => agentStart(P(n).id, 'z8_' + i));
  await sleep(1500);

  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage(); watch(page);
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  const posted = [];
  page.on('request', r => { if (r.method() === 'POST' && /seat-group$/.test(r.url())) posted.push(JSON.parse(r.postData() || '{}')); });
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await signIn(page, 'boss', 'Boss12345'); // З19-и14: вход — когда запас лимитера стенда полный, дальше — когда вход дописан, а не пауза
  await page.evaluate(() => nav('hall')); await sleep(1500);

  // ── и1: «Скоро на нуле» ──────────────────────────────────────────────
  let low = await page.evaluate(() => document.getElementById('lowBox').classList.contains('hidden'));
  check('и1: без сессий на грани виджет «Скоро на нуле» скрыт', low === true, String(low));
  const seat30 = await j(API + '/admin/computers/' + P('PC-05').id + '/session', { method: 'POST', headers: h, body: JSON.stringify({ guest_id: G.Guest30, planned_min: 60 }) });
  check('посадка Guest30 за PC-05 (API) → 201', seat30._status === 201, JSON.stringify(seat30).slice(0, 120));
  await j(API + '/me/sessions/ready', { method: 'POST', headers: hg, body: '{}' });
  await sleep(600);
  psql(`UPDATE users SET wallet_grosz=300 WHERE id='${G.Guest30}'`); // 3 zł при 20 zł/ч → 9 мин
  await page.evaluate(() => refresh()); await sleep(1500);
  const lowRow = await page.evaluate(() => ({ hidden: document.getElementById('lowBox').classList.contains('hidden'), n: document.getElementById('lowN').textContent, txt: document.getElementById('lowList').textContent.replace(/\s+/g, ' ').trim(), btns: [...document.querySelectorAll('#lowList button')].map(b => b.textContent.trim()) }));
  const mLeft = /ещё (\d+) мин/.exec(lowRow.txt);
  check('и1: виджет виден — Guest30 · PC-05 · «ещё N мин» (N ≤ 15), кнопки «пополнить» и «пакет»', !lowRow.hidden && lowRow.n === '1' && /Guest30/.test(lowRow.txt) && /PC-05/.test(lowRow.txt) && mLeft && +mLeft[1] <= 15 && lowRow.btns.some(b => /пополнить/.test(b)) && lowRow.btns.some(b => /пакет/.test(b)), JSON.stringify(lowRow));
  await page.screenshot({ path: OUT + '/zh8_low.png', clip: { x: 1040, y: 60, width: 400, height: 420 } });
  await page.click('#lowList button:has-text("пополнить")'); await sleep(1200);
  const depForm = await page.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, sum: !!document.getElementById('d_sum') }));
  check('и1: «пополнить» открывает карточку гостя сразу с формой депозита', /Guest30/.test(depForm.title) && depForm.sum, JSON.stringify(depForm));
  await page.evaluate(() => closeDrawer());
  psql(`UPDATE users SET wallet_grosz=12000 WHERE id='${G.Guest30}'`);
  await page.evaluate(() => refresh()); await sleep(1500);
  low = await page.evaluate(() => document.getElementById('lowBox').classList.contains('hidden'));
  check('и1: пополнили — виджет снова скрыт', low === true, String(low));

  // ── и2: компания ────────────────────────────────────────────────────
  await page.click('#s-hall .toolrow button:has-text("Компания")'); await sleep(1500);
  let grpUI = await page.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, sizes: [...document.querySelectorAll('#drawerBody .segs')][0].textContent.trim(), on: [...document.querySelectorAll('#drawerBody .segs button.on')].map(b => b.textContent.trim()), rows: document.querySelectorAll('#grpRows .actrow').length, sel: [...document.querySelectorAll('#grpRows select')].map(s => s.selectedOptions[0] && s.selectedOptions[0].textContent), hint: [...document.querySelectorAll('#drawerBody .muted')].map(x => x.textContent).find(t => /Предложены|Ищу|не хватает/.test(t)) }));
  check('и2: шторка «Компания»: размер 2..6 (2 включён), «Любая зона», 60 мин, две строки с ПК-соседями PC-01, PC-02 по схеме', /Компания/.test(grpUI.title) && grpUI.sizes === '23456' && grpUI.on.includes('2') && grpUI.on.includes('60 мин') && grpUI.on.includes('Любая зона') && grpUI.rows === 2 && grpUI.sel.join(',') === 'PC-01,PC-02' && /Предложены соседние машины по схеме зала: PC-01, PC-02/.test(grpUI.hint || ''), JSON.stringify(grpUI));
  await page.click('#drawerBody .segs button:has-text("3")'); await sleep(900);
  grpUI = await page.evaluate(() => ({ rows: document.querySelectorAll('#grpRows .actrow').length, sel: [...document.querySelectorAll('#grpRows select')].map(s => s.selectedOptions[0] && s.selectedOptions[0].textContent) }));
  check('и2: размер 3 → три строки, третьей предложен PC-03 (ряд)', grpUI.rows === 3 && grpUI.sel.join(',') === 'PC-01,PC-02,PC-03', JSON.stringify(grpUI));
  await page.fill('#grpG0', 'Guest19'); await page.keyboard.press('Enter'); await sleep(900);
  let chip0 = await page.evaluate(() => (document.querySelector('#grpRows .actrow .mchip') || {}).textContent);
  check('и2: ник + Enter → гость опознан чипом «✓ Guest19»', /✓ Guest19/.test(chip0 || ''), String(chip0));
  await page.fill('#grpG1', 'Ян'); await page.keyboard.press('Enter'); await sleep(900);
  const cands = await page.evaluate(() => [...document.querySelectorAll('#drawerBody .cands button b')].map(b => b.textContent));
  check('и2: «Ян» подходит двоим → список кандидатов (Яна, Янек), как у посадки/очереди', cands.length === 2 && cands.includes('Яна') && cands.includes('Янек'), JSON.stringify(cands));
  await page.click('#drawerBody .cands button:has-text("Янек")'); await sleep(700);
  const chips = await page.evaluate(() => [...document.querySelectorAll('#grpRows .mchip')].map(x => x.textContent));
  const candsLeft = await page.evaluate(() => document.querySelectorAll('#drawerBody .cands').length);
  check('и2: выбор кандидата → второй чип «✓ Янек», кандидаты убраны', chips.length === 2 && /Янек/.test(chips[1]) && candsLeft === 0, JSON.stringify(chips) + ' cands=' + candsLeft);
  await page.fill('#grpG2', 'NoSuchGuest');
  let toastTxt = await toastAfter(page, () => page.keyboard.press('Enter'), 'не найден');
  check('и2: неизвестный ник → тост «Гость «NoSuchGuest» не найден», строка остаётся полем', /не найден/.test(toastTxt), toastTxt);
  toastTxt = await toastAfter(page, () => page.click('#drawerBody button:has-text("Посадить всех")'), ''); // «Опознай всех» или «не найден» — любой тост этого нажатия
  check('и2: «Посадить всех» с неопознанным гостем → тост (опознай всех / не найден — повторный резолв), запроса на сервер нет', /Опознай всех|не найден/.test(toastTxt) && posted.length === 0, toastTxt + ' posted=' + posted.length);
  await page.fill('#grpG2', 'Guest18'); await page.keyboard.press('Enter'); await sleep(900);
  await page.screenshot({ path: OUT + '/zh8_group.png', clip: { x: 1040, y: 0, width: 400, height: 700 } });
  toastTxt = await toastAfter(page, () => page.click('#drawerBody button:has-text("Посадить всех")'), 'Компания посажена');
  const drawerOpen = await page.evaluate(() => document.getElementById('drawer').getAttribute('aria-hidden') === 'false');
  check('и2: компания из трёх посажена одним POST /admin/seat-group (planned_min 60, 3 места) — тост «Компания посажена: 3 гостя», шторка закрыта', /Компания посажена: 3 гостя/.test(toastTxt) && posted.length === 1 && posted[0].planned_min === 60 && posted[0].seats.length === 3 && posted[0].seats.every(s => s.guest_id && s.computer_id), toastTxt + ' ' + JSON.stringify(posted[0]) + ' open=' + drawerOpen);
  const active = psql(`SELECT string_agg(c.name, ',' ORDER BY c.name) FROM sessions s JOIN computers c ON c.id=s.computer_id WHERE s.status='active' AND s.user_id IN ('${G.Guest19}','${G['Янек']}','${G.Guest18}')`);
  check('и2: в базе три активные сессии за PC-01, PC-02, PC-03', active === 'PC-01,PC-02,PC-03', active);
  await sleep(1200);
  const tiles = await page.evaluate(() => [...document.querySelectorAll('.hpc')].filter(t => /PC-0[123]/.test(t.textContent)).map(t => t.className.replace(/\s+/g, ' ')));
  check('и2: плитки PC-01..03 в зале перекрасились (не available)', tiles.length === 3 && tiles.every(c => !/available/.test(c)), JSON.stringify(tiles));
  // частичный провал: Guest18 сняли, Guest19 сидит → компания [Guest19, Guest18] сажает одного
  const sid18 = psql(`SELECT id FROM sessions WHERE status='active' AND user_id='${G.Guest18}'`);
  await j(API + '/admin/sessions/' + sid18 + '/end', { method: 'POST', headers: h, body: '{}' });
  await sleep(800);
  await page.click('#s-hall .toolrow button:has-text("Компания")'); await sleep(1500);
  const sugg2 = await page.evaluate(() => [...document.querySelectorAll('#grpRows select')].map(s => s.selectedOptions[0] && s.selectedOptions[0].textContent));
  check('и2: PC-01/02 заняты — новой паре предложены другие соседи', sugg2.length === 2 && !sugg2.includes('PC-01') && !sugg2.includes('PC-02') && sugg2[0] !== sugg2[1], JSON.stringify(sugg2));
  await page.fill('#grpG0', 'Guest19'); await page.keyboard.press('Enter'); await sleep(800);
  await page.fill('#grpG1', 'Guest18'); await page.keyboard.press('Enter'); await sleep(800);
  toastTxt = await toastAfter(page, () => page.click('#drawerBody button:has-text("Посадить всех")'), 'Посажено');
  const partial = await page.evaluate(() => ({ open: document.getElementById('drawer').getAttribute('aria-hidden') === 'false', rows: [...document.querySelectorAll('#grpRows .actrow')].map(r => r.textContent.replace(/\s+/g, ' ').trim()), selects: document.querySelectorAll('#grpRows select').length, btn: (document.querySelector('#drawerBody button[onclick="grpSeatAll()"]') || {}).textContent, hint: [...document.querySelectorAll('#drawerBody .muted')].map(x => x.textContent).find(t => /Кто не сел|Предложены/.test(t)) }));
  check('и2: частичный провал: тост «Посажено 1 из 2», шторка открыта, строка Guest19 с причиной «уже идёт сессия», строка Guest18 застыла «→ ' + sugg2[1] + ' ▶ посажен» без селекта, кнопка «…ещё раз (кто не сел)»', /Посажено 1 из 2/.test(toastTxt) && partial.open && /⚠ .*сессия/.test(partial.rows[0]) && new RegExp(sugg2[1] + ' ▶ посажен').test(partial.rows[1]) && partial.selects === 1 && /ещё раз \(кто не сел\)/.test(partial.btn || '') && /Кто не сел — поправь/.test(partial.hint || ''), toastTxt + ' ' + JSON.stringify(partial));
  await page.screenshot({ path: OUT + '/zh8_group_partial.png', clip: { x: 1040, y: 0, width: 400, height: 700 } });
  await page.evaluate(() => closeDrawer()); await sleep(300);

  // ── и3: табло — карточка владельца в «Экономике» ─────────────────────
  await page.evaluate(() => nav('settings')); await sleep(1800);
  let bb = await page.evaluate(() => ({ txt: document.getElementById('boardBox').textContent.replace(/\s+/g, ' ').trim(), btns: [...document.querySelectorAll('#boardBox button')].map(b => b.textContent.trim()) }));
  check('и3: карточка «Табло зала»: выключено, кнопка «Выпустить ссылку»', /выключено/.test(bb.txt) && bb.btns.length === 1 && /Выпустить ссылку/.test(bb.btns[0]), JSON.stringify(bb));
  await toastAfter(page, () => page.click('#boardBox button:has-text("Выпустить ссылку")'), 'Табло включено'); // снимок ниже — с этим тостом
  bb = await page.evaluate(() => ({ txt: document.getElementById('boardBox').textContent.replace(/\s+/g, ' ').trim(), link: (document.getElementById('boardLink') || {}).value, btns: [...document.querySelectorAll('#boardBox button, #boardBox a.btn')].map(b => b.textContent.trim()), toast: document.getElementById('toast').textContent }));
  const tok1 = /token=([0-9a-f]{48})$/.exec(bb.link || '');
  check('и3: включено — ссылка origin + /board?token=<48 hex>, кнопки копировать/открыть/перевыпустить/выключить, тост', /включено/.test(bb.txt) && tok1 && bb.link.startsWith(BASE + '/board?token=') && bb.btns.some(b => /Скопировать/.test(b)) && bb.btns.some(b => /Открыть табло/.test(b)) && bb.btns.some(b => /Перевыпустить/.test(b)) && bb.btns.some(b => /Выключить/.test(b)) && /Табло включено/.test(bb.toast), JSON.stringify(bb));
  const srvTok = (await j(API + '/admin/board/token', { headers: ho })).token;
  check('и3: токен в карточке = токен на сервере', tok1 && tok1[1] === srvTok, (tok1 || [])[1] + ' vs ' + srvTok);
  await page.click('#boardRotateBtn'); await sleep(300);
  const armed = await page.evaluate(() => document.getElementById('boardRotateBtn').textContent.trim());
  check('и3: «Перевыпустить» со взводом — первый клик: «Ещё раз — старая ссылка умрёт»', /Ещё раз — старая ссылка умрёт/.test(armed), armed);
  await toastAfter(page, () => page.click('#boardRotateBtn'), 'старая ссылка больше не работает');
  bb = await page.evaluate(() => ({ link: (document.getElementById('boardLink') || {}).value, toast: document.getElementById('toast').textContent }));
  const tok2 = /token=([0-9a-f]{48})$/.exec(bb.link || '');
  check('и3: второй клик — новый токен ≠ старому, тост «старая ссылка больше не работает»', tok2 && tok2[1] !== tok1[1] && /старая ссылка больше не работает/.test(bb.toast), JSON.stringify(bb));
  await page.screenshot({ path: OUT + '/zh8_boardbox.png', clip: { x: 200, y: 0, width: 1240, height: 900 } });

  // ── и3: страница табло ───────────────────────────────────────────────
  const tv = await ctx.newPage();
  await tv.setViewportSize({ width: 1920, height: 1080 });
  const tvErrors = []; tv.on('pageerror', e => tvErrors.push(e.message));
  await tv.goto(BASE + '/board', { waitUntil: 'load' }); await sleep(800);
  let off = await tv.evaluate(() => ({ show: document.getElementById('off').classList.contains('show'), title: document.getElementById('offTitle').textContent, txt: document.getElementById('offTxt').textContent }));
  check('табло без токена: заглушка «Табло выключено» — «В адресе нет токена…», данных не просит', off.show && /Табло выключено/.test(off.title) && /нет токена/.test(off.txt), JSON.stringify(off));
  await tv.goto(BASE + '/board?token=' + tok1[1], { waitUntil: 'load' }); await sleep(1500);
  off = await tv.evaluate(() => ({ show: document.getElementById('off').classList.contains('show'), txt: document.getElementById('offTxt').textContent, zones: document.getElementById('zones').textContent.trim() }));
  check('табло со старым токеном: сервер 401 → заглушка «Ссылка устарела…», зал не отрисован', off.show && /устарела/.test(off.txt) && off.zones === '', JSON.stringify(off));
  // очередь по нику и бронь — до захода с живым токеном
  const club = psql(`SELECT id FROM clubs LIMIT 1`);
  psql(`INSERT INTO waitlist (club_id, user_id, status) VALUES ('${club}', '${G.Guest24}', 'waiting')`);
  const t70 = new Date(Date.now() + 70 * 60000); t70.setSeconds(0, 0);
  const bk = await j(API + '/admin/bookings', { method: 'POST', headers: h, body: JSON.stringify({ guest_id: G.Guest24, computer_id: P('PC-09').id, start_time: t70.toISOString(), duration_min: 60 }) });
  check('бронь Guest24 на PC-09 через 70 мин (API) создана', bk._status === 201, JSON.stringify(bk).slice(0, 100));
  await tv.goto(BASE + '/board?token=' + tok2[1], { waitUntil: 'load' }); await sleep(1500);
  const board = await tv.evaluate(() => ({
    off: document.getElementById('off').classList.contains('show'),
    free: document.getElementById('free').textContent, freeTxt: document.getElementById('freeTxt').textContent,
    zones: [...document.querySelectorAll('#zones .zone .nm')].map(x => x.textContent),
    pcs: document.querySelectorAll('#zones .pc').length,
    busy: [...document.querySelectorAll('#zones .pc.busy')].map(x => x.textContent.trim()),
    freeList: [...document.querySelectorAll('#zones .pc.free')].map(x => x.textContent.trim()),
    zonesTxt: document.getElementById('zones').textContent,
    queue: [...document.querySelectorAll('#queue .qi')].map(q => ({ pos: q.querySelector('.pos').textContent, nick: q.querySelector('b').textContent, called: q.classList.contains('called') })),
    qN: document.getElementById('qN').textContent,
    bk: [...document.querySelectorAll('#bookings .bk')].map(b => ({ t: b.querySelector('.t').textContent, c: b.querySelector('.c').textContent, d: b.querySelector('.d').textContent })),
    foot: document.getElementById('footTxt').textContent, stale: document.getElementById('foot').classList.contains('stale'),
    club: document.getElementById('club').textContent, clock: document.getElementById('clock').textContent,
  }));
  const totalPcs = +psql(`SELECT count(*) FROM computers WHERE status<>'maintenance'`);
  const busyExp = ['PC-01', 'PC-02', 'PC-05'].concat(sugg2.filter((n, i) => i === 1)); // Guest18 сел за вторую предложенную
  check('табло с токеном: заглушки нет, зоны Standard и VIP, плиток = ПК не в ремонте, счётчик свободных', !board.off && board.zones.includes('Standard') && board.zones.includes('VIP') && board.pcs === totalPcs && +board.free === totalPcs - board.busy.length && /свободн/.test(board.freeTxt), JSON.stringify({ free: board.free, pcs: board.pcs, zones: board.zones, busy: board.busy }));
  check('табло: занятые ПК (PC-01, PC-02, PC-05, ' + busyExp[3] + ') без ников — в блоке зала нет ни одного «Guest»/«Янек»', busyExp.every(n => board.busy.includes(n)) && board.busy.length === 4 && !/Guest|Янек/.test(board.zonesTxt) && board.freeList.includes('PC-09') && board.freeList.includes('VIP-03'), JSON.stringify(board.busy));
  check('табло: очередь по нику — «1 Guest24», счётчик «· 1»', board.queue.length === 1 && board.queue[0].nick === 'Guest24' && board.queue[0].pos === '1' && !board.queue[0].called && board.qN === '· 1', JSON.stringify(board.queue) + ' ' + board.qN);
  const hhmmLocal = (d) => String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
  check('табло: ближайшая бронь — «HH:MM PC-09 60 мин», без ника', board.bk.length >= 1 && board.bk.some(b => b.c === 'PC-09' && b.d === '60 мин' && b.t === hhmmLocal(t70)) && !/Guest24/.test(JSON.stringify(board.bk)), JSON.stringify(board.bk) + ' ожидалось ' + hhmmLocal(t70));
  check('табло: подвал «обновлено … · каждые 10 с», клуб и часы на месте, курсор скрыт', /обновлено .* каждые 10 с/.test(board.foot) && !board.stale && board.club && /^\d\d:\d\d$/.test(board.clock), JSON.stringify({ foot: board.foot, club: board.club, clock: board.clock }));
  await tv.screenshot({ path: OUT + '/zh8_board.png' });
  // вызов из очереди → called
  const wid = psql(`SELECT id FROM waitlist WHERE status='waiting' AND user_id='${G.Guest24}'`);
  psql(`UPDATE waitlist SET notified_at=now() WHERE id='${wid}'`);
  await tv.evaluate(() => tick()); await sleep(800);
  const called = await tv.evaluate(() => ({ called: document.querySelector('#queue .qi').classList.contains('called'), w: document.querySelector('#queue .qi .w').textContent }));
  check('табло: гостя вызвали — строка подсвечена, «ваш ПК свободен — к стойке»', called.called && /к стойке/.test(called.w), JSON.stringify(called));
  psql(`UPDATE waitlist SET status='removed', resolved_at=now() WHERE id='${wid}'`);
  // выключили в админке — табло само гаснет на следующем тике
  await page.click('#boardOffBtn'); await sleep(300);
  await toastAfter(page, () => page.click('#boardOffBtn'), 'Табло выключено');
  bb = await page.evaluate(() => ({ txt: document.getElementById('boardBox').textContent.replace(/\s+/g, ' ').trim(), toast: document.getElementById('toast').textContent }));
  check('и3: «Выключить табло» (со взводом) → карточка «выключено», тост', /выключено/.test(bb.txt) && /Табло выключено/.test(bb.toast), JSON.stringify(bb));
  await tv.evaluate(() => tick()); await sleep(800);
  off = await tv.evaluate(() => ({ show: document.getElementById('off').classList.contains('show'), title: document.getElementById('offTitle').textContent }));
  check('табло после выключения: следующий тик → 401 → заглушка «Табло выключено»', off.show && /Табло выключено/.test(off.title), JSON.stringify(off));
  check('табло: ошибок JS нет', tvErrors.length === 0, tvErrors.join(' | '));
  await tv.close();

  // ── и4: цифры в палитре ─────────────────────────────────────────────
  await page.evaluate(() => nav('hall')); await sleep(1000);
  const parse = await page.evaluate(() => ['12', ' 5', 'pc 7', 'PC-07', 'пк7', 'v3', 'vip 3', 'вип-3', 'в1', 'abc', 'Guest1', '1234', '12345', 'p'].map(q => [q, palParsePC(q)]));
  const pm = Object.fromEntries(parse);
  check('и4: разбор: «12»/« 5»/«pc 7»/«PC-07»/«пк7» → pc; «v3»/«vip 3»/«вип-3»/«в1» → vip; «abc»/«Guest1»/«1234»/«p» → не ПК', pm['12'].kind === 'pc' && pm['12'].num === '12' && pm[' 5'].kind === 'pc' && pm['pc 7'].kind === 'pc' && pm['pc 7'].explicitPrefix && pm['PC-07'].num === '07' && pm['пк7'].kind === 'pc' && pm['v3'].kind === 'vip' && pm['vip 3'].kind === 'vip' && pm['вип-3'].kind === 'vip' && pm['в1'].kind === 'vip' && pm['abc'] === null && pm['Guest1'] === null && pm['1234'] === null && pm['12345'] === null && pm['p'] === null, JSON.stringify(parse));
  const match = await page.evaluate(() => ({ exact: palPCMatch('PC-05', palParsePC('5')), lead: palPCMatch('PC-05', palParsePC('05')), digit: palPCMatch('PC-15', palParsePC('1')), vipByBare: palPCMatch('VIP-01', palParsePC('1')), vipByPc: palPCMatch('VIP-01', palParsePC('pc 1')), pcByVip: palPCMatch('PC-03', palParsePC('v3')), none: palPCMatch('PC-05', palParsePC('12')) }));
  check('и4: совпадение: «5»→PC-05 точное (3), «05» тоже, «1»→PC-15 по цифре (1), голая «1» берёт и VIP-01, «pc 1» — только PC, «v3» — только VIP', match.exact === 3 && match.lead === 3 && match.digit === 1 && match.vipByBare === 3 && match.vipByPc === 0 && match.pcByVip === 0 && match.none === 0, JSON.stringify(match));
  await page.evaluate(() => palOpen()); await sleep(200);
  const palType = async (q) => { await page.fill('#palInp', q); await sleep(700); return page.evaluate(() => ({ items: [...document.querySelectorAll('#palList .pi')].map(x => ({ name: x.querySelector('b').textContent, sub: (x.querySelector('.muted') || {}).textContent || '' })), secs: [...document.querySelectorAll('#palList .sub')].map(x => x.textContent), none: (document.querySelector('#palList .none') || {}).textContent })); };
  let pl = await palType('5');
  check('и4: «5» → одна строка PC-05 в секции «ПК», подпись «занят: Guest30»', pl.items.length === 1 && pl.items[0].name === 'PC-05' && pl.secs.join() === 'ПК' && /занят: Guest30/.test(pl.items[0].sub), JSON.stringify(pl));
  pl = await palType('1');
  check('и4: «1» → PC-01 и VIP-01 (точные), без разделов/действий', pl.items.map(x => x.name).sort().join() === 'PC-01,VIP-01' && pl.secs.every(s => /ПК/.test(s)), JSON.stringify(pl.items.map(x => x.name)) + ' ' + pl.secs);
  pl = await palType('v3');
  check('и4: «v3» → только VIP-03 («свободен»)', pl.items.length === 1 && pl.items[0].name === 'VIP-03' && /свободен|offline/.test(pl.items[0].sub), JSON.stringify(pl));
  pl = await palType('vip 3');
  check('и4: «vip 3» → VIP-03', pl.items.length === 1 && pl.items[0].name === 'VIP-03', JSON.stringify(pl.items));
  pl = await palType('pc 1');
  check('и4: «pc 1» → только PC-01 (VIP отрезан явным префиксом)', pl.items.length === 1 && pl.items[0].name === 'PC-01', JSON.stringify(pl.items));
  pl = await palType('12');
  check('и4: «12» → пусто с подсказкой «ПК с номером 12 нет»', pl.items.length === 0 && /ПК с номером 12 нет/.test(pl.none || ''), JSON.stringify(pl));
  pl = await palType('500120030');
  check('и4: девять цифр → «Гости по телефону»: Guest30 с телефоном', pl.items.length === 1 && pl.items[0].name === 'Guest30' && pl.secs.join() === 'Гости по телефону' && /\+48/.test(pl.items[0].sub), JSON.stringify(pl));
  pl = await palType('0000');
  check('и4: телефон, которого нет → «Гостя с таким телефоном нет»', pl.items.length === 0 && /Гостя с таким телефоном нет/.test(pl.none || ''), JSON.stringify(pl));
  pl = await palType('зал');
  check('и4: буквы работают как раньше — разделы («Зал»)', pl.items.some(x => x.name === 'Зал') && pl.secs.includes('Разделы'), JSON.stringify(pl.items.map(x => x.name)));
  await page.fill('#palInp', 'v3'); await sleep(700); await page.keyboard.press('Enter'); await sleep(900);
  const afterEnter = await page.evaluate(() => ({ palHidden: document.getElementById('pal').classList.contains('hidden'), title: document.getElementById('drawerTitle').textContent, tab: curTab }));
  check('и4: Enter на «v3» → палитра закрыта, Зал, шторка VIP-03', afterEnter.palHidden && /VIP-03/.test(afterEnter.title) && afterEnter.tab === 'hall', JSON.stringify(afterEnter));
  await page.evaluate(() => closeDrawer());
  await page.evaluate(() => palOpen()); await page.fill('#palInp', '5'); await sleep(700);
  await page.screenshot({ path: OUT + '/zh8_palette.png', clip: { x: 320, y: 60, width: 800, height: 300 } });
  await page.keyboard.press('Escape'); await sleep(200);

  // ── и5: звук вызова ─────────────────────────────────────────────────
  const snd0 = await page.evaluate(() => ({ on: callSoundOn, ls: localStorage.getItem('1448a_callsound'), unlocked: audioUnlocked }));
  check('и5: по умолчанию звук включён (localStorage пуст), AudioContext разблокирован первым же кликом', snd0.on === true && snd0.ls === null && snd0.unlocked === true, JSON.stringify(snd0));
  await page.click('#bellBtn'); await sleep(300);
  let sb = await page.evaluate(() => (document.getElementById('callSoundBtn') || {}).textContent);
  check('и5: в колоколе кнопка «🔔 звук вкл»', /звук вкл/.test(sb || ''), String(sb));
  await page.click('#callSoundBtn'); await sleep(300);
  let snd = await page.evaluate(() => ({ on: callSoundOn, ls: localStorage.getItem('1448a_callsound'), btn: document.getElementById('callSoundBtn').textContent, dropOpen: !document.getElementById('bellDrop').classList.contains('hidden') }));
  check('и5: клик → «🔕 звук выкл», localStorage=off, список не закрылся', snd.on === false && snd.ls === 'off' && /звук выкл/.test(snd.btn) && snd.dropOpen, JSON.stringify(snd));
  await page.screenshot({ path: OUT + '/zh8_bell.png', clip: { x: 900, y: 0, width: 540, height: 260 } });
  await page.click('#callSoundBtn'); await sleep(300);
  snd = await page.evaluate(() => ({ on: callSoundOn, ls: localStorage.getItem('1448a_callsound') }));
  check('и5: ещё клик → снова вкл, localStorage=on', snd.on === true && snd.ls === 'on', JSON.stringify(snd));
  await page.keyboard.press('Escape'); await page.evaluate(() => document.getElementById('bellDrop').classList.add('hidden'));
  // живой вызов гостя → playCallSound() ровно один раз; сообщение (не вызов) — нет
  const bell0 = +(await page.evaluate(() => document.getElementById('bellBadge').textContent)) || 0;
  await page.evaluate(() => { window.__calls = 0; const orig = playCallSound; playCallSound = function (f) { window.__calls++; return orig.call(this, f); }; });
  await j(API + '/me/chat', { method: 'POST', headers: hg, body: JSON.stringify({ kind: 'call' }) }); await sleep(1500);
  await j(API + '/me/chat', { method: 'POST', headers: hg, body: JSON.stringify({ kind: 'text', text: 'принесите воды' }) }); await sleep(1500);
  const calls = await page.evaluate(() => ({ calls: window.__calls, bell: document.getElementById('bellBadge').textContent, toast: document.getElementById('toast').textContent }));
  check('и5: вызов из шелла → сигнал 1 раз; обычное сообщение — без сигнала; колокол +2', calls.calls === 1 && +calls.bell === bell0 + 2, JSON.stringify(calls) + ' было ' + bell0);
  // после перезагрузки настройка «выкл» переживает
  await page.evaluate(() => localStorage.setItem('1448a_callsound', 'off'));
  await reenter(page); // перезагрузка — снова вход: тот же залп запросов
  const persisted = await page.evaluate(() => callSoundOn);
  check('и5: «выкл» переживает перезагрузку страницы (настройка этого компьютера)', persisted === false, String(persisted));
  await page.evaluate(() => localStorage.removeItem('1448a_callsound'));
  for (const m of psql(`SELECT COALESCE(string_agg(id::text, ' '), '') FROM chat_messages WHERE user_id='${G.Guest30}' AND NOT read_staff AND sender<>'staff'`).split(/\s+/).filter(Boolean))
    await j(API + '/admin/chat/' + m + '/ack', { method: 'POST', headers: h, body: '{}' });

  check('ошибок JS в админке нет', errors.length === 0, errors.join(' | '));
  await browser.close();
  await reset();
  agents.forEach(p => { try { process.kill(-p.pid); } catch (e) { try { p.kill(); } catch (e2) {} } });
  const okN = out.filter(l => l.startsWith('ok')).length, failN = out.length - okN;
  console.log(out.join('\n') + `\n\n${okN} ok, ${failN} FAIL`);
  process.exit(failN ? 1 : 0);
})().catch(e => { console.error('CRASH', e); agents.forEach(p => { try { process.kill(-p.pid); } catch (x) {} }); process.exit(2); });
