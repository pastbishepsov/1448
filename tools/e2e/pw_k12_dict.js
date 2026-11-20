// К12-и3 «Строки словаря» — проверка в браузере (Chrome без окна). Четыре строки, найденные в К12-и2:
// 1) киоск, всплывающая строка «сенса подставлена, а игра уже работает»: по-английски и по-польски она теперь,
//    как и по-русски, начинается со сказуемого — «already running: CS2, Apex Legends — …», «już działa: …»,
//    и не зависит от числа игр (было «CS2, Apex Legends is already running»);
// 2) киоск, эффект таланта «Градация везения»: по-английски «×1.1» с точкой, по-русски и по-польски «×1,1»;
// 3) PWA, окно пополнения по-русски — без «в спринте 6»;
// 4) PWA показывает имя ранга на языке гостя, как киоск (RANK_KEYS → rk_N): шапка, карточка ранга, «… до «…»».
//    Имя, которого нет среди семи стандартных, остаётся как прислал сервер.
// Киоск открывается без гостя (/shell): строки строит сам киоск из своих словарей, эталон имён рангов — тоже киоск.
// PWA — вход Guest46 на телефоне 390×844 на трёх языках (язык гостя в базе и в телефоне): сначала шапка с тем
// рангом, что прислал сервер, потом подстановка в памяти страницы «Ветеран, дальше Мастер», чтобы строка «до …»
// была всегда. В базе меняет только язык гостя и в конце возвращает его.
// На c373a19 (до К12-и3) падают строки EN и PL в киоске, «×1,1» по-английски, «спринт», ранги и версия PWA.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;
const VER_MIN = '2026-11-20.1'; // версия PWA этой итерации; дальше она только растёт
const GAMES = 'CS2, Apex Legends', CYR = /[А-Яа-яЁё]/;
const WANT = {
  ru: { sens: ` · уже работает: ${GAMES} — сенса подхватится при перезапуске (CS2: exec 1448 в консоли)`, boost: ['×1,1', '×1,25'] },
  en: { sens: ` · already running: ${GAMES} — sens applies after a restart (CS2: exec 1448 in the console)`, boost: ['×1.1', '×1.25'] },
  pl: { sens: ` · już działa: ${GAMES} — czułość wczyta się po restarcie (CS2: exec 1448 w konsoli)`, boost: ['×1,1', '×1,25'] },
};

(async () => {
  const G = 'Guest46', g = z.uid(G);
  const lang0 = z.sql(`SELECT language FROM users WHERE id='${g}'`);
  z.atEnd(() => {
    z.sql(`UPDATE users SET language='${lang0}' WHERE id='${g}'`);
    z.check(`язык гостя в базе возвращён (${lang0})`, z.sql(`SELECT language FROM users WHERE id='${g}'`) === lang0, lang0);
  });
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());

  console.log('— Киоск: строки из его словарей на трёх языках');
  const k = await (await br.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell', { waitUntil: 'load' });
  const K = await k.evaluate(games => {
    const keep = { STR, LANG }, out = {};
    for (const l of ['ru', 'en', 'pl']) {
      LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]);
      out[l] = { sens: T('toast_sens_running', { g: games }),
        boost: [TAL_FMT.case_tier_boost(0.1), TAL_FMT.case_tier_boost(0.25)],
        rk: [1, 2, 3, 4, 5, 6, 7].map(n => STR['rk_' + n]),
        byRu: Object.fromEntries(Object.entries(RANK_KEYS).map(([ru, key]) => [ru, STR[key]])) };
    }
    LANG = keep.LANG; STR = keep.STR;
    return out;
  }, GAMES);
  for (const l of ['ru', 'en', 'pl']) {
    const o = K[l], W = WANT[l], L = l.toUpperCase();
    z.check(`${L}: «игра уже работает» — «${W.sens.slice(3)}»`, o.sens === W.sens, J(o.sens));
    z.check(`${L}: «Градация везения» — ${W.boost.join(' и ')}`, J(o.boost) === J(W.boost), J(o.boost));
  }

  console.log('— PWA: ранги на языке гостя, пополнение без «спринта»');
  let ver = null;
  for (const l of ['ru', 'en', 'pl']) {
    const L = l.toUpperCase();
    z.sql(`UPDATE users SET language='${l}' WHERE id='${g}'`);
    const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    await ctx.addInitScript(v => { try { localStorage.setItem('1448_lang', v); } catch (e) {} }, l);
    const a = await ctx.newPage();
    a.on('pageerror', e => z.jsErrors.push(`PWA ${L}: ` + e.message));
    await a.goto(z.BASE + '/app', { waitUntil: 'load' });
    await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
    const inOk = await a.waitForFunction(() => typeof me !== 'undefined' && me && me.id && typeof economy !== 'undefined'
      && economy && economy.rank && document.getElementById('rankName').textContent !== '—', null, { timeout: 20000 })
      .then(() => true, () => false);
    z.check(`${L}: гость вошёл в PWA, ранг пришёл с сервера`, inOk, '');
    if (!inOk) { await ctx.close(); continue; }
    await a.click('button[data-tab="progress"]');
    await a.waitForFunction(() => document.querySelector('#rankCard div[style*="font-weight:900"]'), null, { timeout: 8000 }).catch(() => {});
    const P = await a.evaluate(async () => {
      const head = () => document.getElementById('rankName').textContent;
      const out = { lang: LANG, server: economy.rank.name, head: head(), topup: STR.topup_body,
        rk: [1, 2, 3, 4, 5, 6, 7].map(n => STR['rk_' + n]),
        other: typeof rankLabel === 'function' ? rankLabel('Чемпион клуба') : null };
      const keep = { rank: economy.rank, next: economy.next_rank };
      economy.rank = Object.assign({}, keep.rank, { name: 'Ветеран', level: 3, min_hours: 50 });
      economy.next_rank = { name: 'Мастер', level: 4, min_hours: 120, hours_left: 12 };
      renderTop(); renderRank();
      const card = document.getElementById('rankCard'), nm = card.querySelector('div[style*="font-weight:900"]');
      const pt = card.querySelector('.hbar + .muted'); // строка под полосой: «Ещё 12 ч до «Мастер»»
      Object.assign(out, { head2: head(), name: nm ? nm.textContent.trim() : null, next: pt ? pt.textContent.trim() : null,
        card: card.innerText.replace(/\s+/g, ' ').trim() });
      economy.rank = keep.rank; economy.next_rank = keep.next; renderTop(); renderRank();
      out.back = head() === out.head;
      out.ver = { app: APP_VER, mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version };
      return out;
    });
    ver = ver || P.ver;
    const kr = K[l].byRu, want = n => kr[n] || n;
    z.check(`${L}: имена рангов в словаре PWA те же, что в киоске — ${K[l].rk.join(', ')}`, J(P.rk) === J(K[l].rk), J(P.rk));
    z.check(`${L}: шапка — ранг гостя «${want(P.server)}» (сервер прислал «${P.server}»)`, P.lang === l && P.head === want(P.server),
      J([P.lang, P.head]));
    z.check(`${L}: подстановка «Ветеран → Мастер»: шапка «${kr['Ветеран']}», карточка «${kr['Ветеран']}», строка «… до ${kr['Мастер']}»`,
      P.head2 === kr['Ветеран'] && P.name === kr['Ветеран'] && !!P.next && P.next.includes(kr['Мастер']), J([P.head2, P.name, P.next]));
    if (l !== 'ru') z.check(`${L}: в карточке ранга нет кириллицы`, !CYR.test(P.card), P.card);
    z.check(`${L}: имя не из семи стандартных остаётся как есть`, P.other === 'Чемпион клуба', J(P.other));
    z.check(`${L}: после подстановки шапка снова как с сервера`, P.back, '');
    if (l === 'ru') z.check('RU: окно пополнения без «в спринте 6» — «…вместе со Stripe.»',
      !P.topup.includes('спринт') && P.topup.endsWith('вместе со Stripe.'), J(P.topup.slice(-60)));
    await ctx.close();
  }
  z.check(`версия PWA поднята и одна в app.html и в манифесте (${ver && ver.app}) — телефоны гостей подтянут новую страницу`,
    !!ver && ver.app === ver.mf && ver.app >= VER_MIN, J(ver));
  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})();
