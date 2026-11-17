// З19-и15 «Вход в админку без двойной загрузки «Зала»; секундный таймер не стоит при «нет связи»», браузер (из «Найдено
// по дороге» З19-и14). Вход в админку формой: «Зал» (обзор, ПК, брони, очередь, кухня, вызовы) и лента грузятся один раз,
// всего запросов меньше запаса лимитера стенда (20 на адрес), 429 нет; выход и вход в той же вкладке — так же. Живое
// событие за время входа — своё обновление «Зала» (liveRefresh), его проверка считает отдельно: ленту живые события не
// догружают, поэтому она — чистый признак второй загрузки. Живой канал оборвался или поднялся позже 2 с после входа
// (тикет держит page.route) — «Зал» и лента догоняются второй раз, как раньше (Ж3-и4). «Нет связи» и «подключение…»:
// индикатор — как был, а секундный таймер идёт — счётчики смены и пульса «Отчётов» растут. На HEAD при входе «Зал» и
// лента грузятся дважды, а при «нет связи» таймер стоит. Гостей нет: входят boss и admin1.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const HALL = ['/admin/overview', '/admin/computers', '/admin/bookings', '/admin/waitlist', '/admin/kitchen/orders', '/admin/chat/pending'];
const FEED = '/admin/audit?limit=30'; // бэкфилл ленты
(async () => {
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  // вкладка админки на экране входа; log — запросы API страницы (и 429) между start и stop
  const open = async nick => {
    const p = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage(); z.watch(p);
    p.on('pageerror', e => z.jsErrors.push(`${nick}: ${e.message}`));
    const log = []; let on = false;
    p.on('request', r => { if (on && r.url().includes('/api/v1/')) log.push(r.method() + ' ' + r.url().replace(/^.*\/api\/v1/, '')); });
    p.on('response', r => { if (on && r.status() === 429) log.push('429 ' + r.url().replace(/^.*\/api\/v1/, '')); });
    p.on('websocket', w => w.on('framereceived', f => { if (!on) return; // живое событие — своё обновление «Зала»
      try { const m = JSON.parse(f.payload); if (m && m.type) log.push('WS ' + m.type); } catch (e) {} }));
    await p.goto(z.BASE + '/admin', { waitUntil: 'load' });
    return { p, start: () => { log.length = 0; on = true; }, stop: () => { on = false; return log.slice(); } };
  };
  const cnt = (log, h) => log.filter(x => x === 'GET ' + h).length;
  const hall = log => Object.fromEntries([...HALL, FEED].map(h => [h, cnt(log, h)]));
  const n429 = log => log.filter(x => x.startsWith('429 ')).length;
  const events = log => log.filter(x => x.startsWith('WS ')).length;
  const api = log => log.filter(x => !x.startsWith('429 ') && !x.startsWith('WS ')).length;
  // вход загрузил «Зал» один раз: лента — ровно раз, «Зал» — раз и по обновлению на живое событие; без событий — меньше 20
  const once = log => cnt(log, FEED) === 1 && HALL.every(h => cnt(log, h) >= 1 && cnt(log, h) <= 1 + events(log))
    && (events(log) > 0 || api(log) < 20) && !n429(log);

  // вход владельца
  const A = await open('boss');
  A.start();
  await z.signIn(A.p, 'boss', z.passwordOf('boss'));
  const l1 = A.stop(), c1 = hall(l1);
  const v1 = await A.p.evaluate(() => ({ ws: wsOpen, tiles: document.querySelectorAll('.hpc').length }));
  z.check(`вход владельца формой: «Зал» (обзор, ПК, брони, очередь, кухня, вызовы) и лента — по одному разу, всего ${api(l1)} `
    + `запросов — меньше запаса лимитера стенда (20), 429 нет; живой канал поднят, зал на экране`,
    once(l1) && v1.ws && v1.tiles > 0, JSON.stringify({ c1, ev: events(l1), v1, l1 }));

  // обрыв канала — догоняем, как раньше
  A.start();
  await A.p.evaluate(() => ws.close());
  await A.p.waitForFunction(() => !wsOpen, null, { timeout: 5000 }).catch(() => {});
  await A.p.waitForFunction(() => wsOpen, null, { timeout: 30000 }).catch(() => {}); // переподключение — шаг 5 с и новый тикет
  await z.idle(A.p);
  const l2 = A.stop(), c2 = hall(l2);
  z.check('живой канал оборвался и поднялся снова — «Зал» и лента догоняются, что было, пока сокет молчал: обзор, ПК и лента — '
    + 'ещё раз, как раньше', c2['/admin/overview'] >= 1 && c2['/admin/computers'] >= 1 && c2[FEED] === 1
    && await A.p.evaluate(() => wsOpen), JSON.stringify({ c2, l2 }));

  // «нет связи» и «подключение…» — секундный таймер идёт
  const t = await A.p.evaluate(async () => {
    const wait = ms => new Promise(r => setTimeout(r, ms));
    const st = () => ({ txt: document.getElementById('liveTxt').textContent, bad: document.getElementById('live').classList.contains('bad'), shift: shiftTick });
    lastSync = -1; shiftTick = 0; await wait(1200); const a = st();
    await wait(1500); const b = st();
    lastSync = 0; shiftTick = 0; await wait(2200); const c = st();
    await refresh(); await wait(1200); const d = st();
    return { a, b, c, d };
  });
  z.check('«нет связи» (обмен не удался — ставит и один 429): индикатор «нет связи» красным, как был, а секундный таймер идёт — '
    + 'счётчик смены за 2,7 с вырос; «подключение…» — так же; удачный обмен — снова «live»',
    t.a.txt === 'нет связи' && t.a.bad && t.b.shift >= 2 && t.c.txt === 'подключение…' && t.c.shift >= 1 && t.d.txt === 'live' && !t.d.bad,
    JSON.stringify(t));
  await z.nav(A.p, 'stats');
  const pl = await A.p.evaluate(async () => { lastSync = -1; pulseTick = 0; await new Promise(r => setTimeout(r, 2600));
    const n = pulseTick; await refresh(); return n; });
  z.check('«Отчёты» при «нет связи»: счётчик пульса (раз в 5 минут) идёт — за 2,6 с вырос', pl >= 2, String(pl));

  // выход и вход admin1 в той же вкладке
  await A.p.evaluate(() => logout());
  await z.idle(A.p);
  A.start();
  await z.signIn(A.p, 'admin1', z.passwordOf('admin1'));
  const l3 = A.stop(), c3 = hall(l3);
  z.check(`выход и вход admin1 в той же вкладке: снова «Зал» и лента — по одному разу, всего ${api(l3)} запросов, 429 нет`,
    once(l3) && await A.p.evaluate(() => wsOpen), JSON.stringify({ c3, ev: events(l3), l3 }));

  // живой канал поднялся позже 2 с после входа — догоняем
  const B = await open('boss');
  await B.p.route('**/api/v1/admin/ws-ticket', async route => { await z.sleep(2600); await route.continue(); });
  B.start();
  await z.signIn(B.p, 'boss', z.passwordOf('boss'));
  const l4 = B.stop(), c4 = hall(l4);
  z.check('живой канал поднялся позже 2 с после входа (тикет держится 2,6 с) — «Зал» и лента догоняются: обзор, ПК и лента — '
    + 'второй раз, как раньше', c4['/admin/overview'] >= 2 && c4['/admin/computers'] >= 2 && c4[FEED] === 2
    && await B.p.evaluate(() => wsOpen), JSON.stringify({ c4, l4 }));
  z.check('ошибок JS на страницах нет', z.jsErrors.length === 0, z.jsErrors.join(' | ')); // браузер свой — done() его не знает
  await z.done();
})().catch(z.crash);
