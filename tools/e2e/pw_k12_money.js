// К12-и6 «Деньги по-английски — с точкой» — проверка в браузере (Chrome без окна).
// Киоск и PWA пишут суммы в злотых: по-английски дробь через точку («12.50 zł»), по-русски и по-польски — через
// запятую («12,50 zł»); «zł» после суммы, в деньгах тысячи без пробела («1250,00 zł»), целые — как были («25»).
// Киоск открывается без гостя (/shell): помощники fmtZl и fmtNum и строка тарифов заставки (зона VIP, 27,5 zł/ч)
// на трёх языках — подстановкой языка в памяти страницы, как в pw_k12_dict.
// PWA — вход Guest46 на телефоне 390×844 на трёх языках (язык гостя в базе и в телефоне): помощники zl2, fmtPln,
// ktZl; текст уведомления о компенсации; тарифы клуба (clubRates — по зонам и без зон); история пополнений —
// подстановкой пополнений 12,5, 50 и 1000 zł в памяти страницы. В базе меняет только язык гостя и в конце возвращает его.
// На 4645bd0 (до К12-и6) падают: всё английское; тарифы клуба в PWA на всех языках («27,5» вместо «27,50»);
// история пополнений на всех языках («12,5», «1 000»); версия PWA.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;
const VER_MIN = '2026-11-21.3'; // версия PWA этой итерации; дальше она только растёт
const LS = ['ru', 'en', 'pl'];
const D = { ru: ',', en: '.', pl: ',' };
const HOUR = { ru: 'zł/час', en: 'zł/hour', pl: 'zł/godz.' };
const ATT = { ru: 'VIP <b>{r} zł</b>/ч', en: 'VIP <b>{r} zł</b>/h', pl: 'VIP <b>{r} zł</b>/h' };
const DEP = { ru: 'Пополнение', en: 'Top-up', pl: 'Wpłata' };

(async () => {
  const G = 'Guest46', g = z.uid(G);
  const lang0 = z.sql(`SELECT language FROM users WHERE id='${g}'`);
  z.atEnd(() => {
    z.sql(`UPDATE users SET language='${lang0}' WHERE id='${g}'`);
    z.check(`язык гостя в базе возвращён (${lang0})`, z.sql(`SELECT language FROM users WHERE id='${g}'`) === lang0, lang0);
  });
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());

  console.log('— Киоск: fmtZl, fmtNum и тарифы заставки на трёх языках');
  const k = await (await br.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell', { waitUntil: 'load' });
  const K = await k.evaluate(ls => {
    const keep = { STR, LANG, clubs }, out = {};
    clubs = [{ zones: [{ name: 'VIP', rate_pln: 27.5 }] }];
    for (const l of ls) {
      LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]);
      out[l] = { zl: [fmtZl(1250), fmtZl(34), fmtZl(125000)], num: [fmtNum(27.5), fmtNum(25)], att: attractRatesText() };
    }
    LANG = keep.LANG; STR = keep.STR; clubs = keep.clubs;
    return out;
  }, LS);
  for (const l of LS) {
    const o = K[l], d = D[l], L = l.toUpperCase();
    const zl = [`12${d}50 zł`, `0${d}34 zł`, `1250${d}00 zł`], num = [`27${d}50`, '25'], att = ATT[l].replace('{r}', `27${d}50`);
    z.check(`${L} киоск: fmtZl — ${zl.join(', ')}`, J(o.zl) === J(zl), J(o.zl));
    z.check(`${L} киоск: fmtNum — ${num.join(', ')}`, J(o.num) === J(num), J(o.num));
    z.check(`${L} киоск: тарифы заставки — «${att}»`, o.att === att, J(o.att));
  }

  console.log('— PWA: помощники, компенсация, тарифы клуба и история пополнений на трёх языках');
  let ver = null;
  for (const l of LS) {
    const L = l.toUpperCase(), d = D[l];
    z.sql(`UPDATE users SET language='${l}' WHERE id='${g}'`);
    await z.sleep(2500); // лимитер стенда: вход PWA — залп запросов; без паузы второй вход подряд может не пройти
    const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    await ctx.addInitScript(v => { try { localStorage.setItem('1448_lang', v); } catch (e) {} }, l);
    const a = await ctx.newPage();
    a.on('pageerror', e => z.jsErrors.push(`PWA ${L}: ` + e.message));
    await a.goto(z.BASE + '/app', { waitUntil: 'load' });
    await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
    const inOk = await a.waitForFunction(() => typeof me !== 'undefined' && me && me.id && typeof economy !== 'undefined'
      && economy && economy.rank, null, { timeout: 20000 }).then(() => true, () => false);
    const lang = inOk ? await a.evaluate(() => LANG) : null;
    z.check(`${L}: гость вошёл в PWA, язык страницы — ${l}`, inOk && lang === l, J(lang));
    if (!inOk) { await ctx.close(); continue; }
    await a.click('button[data-tab="progress"]'); // история пополнений живёт на вкладке «Прогресс»
    await a.waitForFunction(() => deposits !== null, null, { timeout: 8000 }).catch(() => {});
    const P = await a.evaluate(async () => {
      const out = { help: [zl2(1250), fmtPln(27.5), fmtPln(25), ktZl(7.5)],
        comp: ntfText({ type: 'incident_compensation', payload: { wallet_pln: 12.5 } }),
        zones: clubRates({ zones: [{ name: 'VIP', rate_pln: 27.5 }], base_rate_pln: 23 }),
        base: clubRates({ zones: [], base_rate_pln: 23.5 }) };
      const keep = { deposits, sessionsArr }, t = Date.now();
      deposits = [{ created_at: new Date(t - 6e4).toISOString(), amount_pln: 12.5, coins_granted: 0 },
        { created_at: new Date(t - 36e5).toISOString(), amount_pln: 50, coins_granted: 0 },
        { created_at: new Date(t - 72e5).toISOString(), amount_pln: 1000, coins_granted: 0 }];
      sessionsArr = [];
      renderHist();
      out.hist = document.getElementById('histCard').textContent.replace(/\s+/g, ' ');
      deposits = keep.deposits; sessionsArr = keep.sessionsArr; renderHist();
      out.ver = { app: APP_VER, mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version };
      return out;
    });
    ver = ver || P.ver;
    const help = [`12${d}50`, `27${d}50`, '25', `7${d}50 zł`], comp = `+12${d}50 zł`;
    const zones = `VIP 27${d}50 ${HOUR[l]}`, base = `23${d}50 ${HOUR[l]}`;
    const hist = [`${DEP[l]} 12${d}50 zł`, `${DEP[l]} 50 zł`, `${DEP[l]} 1000 zł`];
    z.check(`${L} PWA: zl2, fmtPln, ktZl — ${help.join(', ')}`, J(P.help) === J(help), J(P.help));
    z.check(`${L} PWA: компенсация — «${comp}»`, P.comp.includes(comp) && !P.comp.includes(`12${d === '.' ? ',' : '.'}5`), J(P.comp));
    z.check(`${L} PWA: тарифы клуба по зонам — «${zones}»`, P.zones === zones, J(P.zones));
    z.check(`${L} PWA: тариф клуба без зон — «${base}»`, P.base === base, J(P.base));
    z.check(`${L} PWA: история пополнений — ${hist.map(x => '«' + x + '»').join(', ')}`,
      hist.every(x => P.hist.includes(x)), J(P.hist.slice(0, 400)));
    await ctx.close();
  }
  z.check(`версия PWA поднята и одна в app.html и в манифесте (${ver && ver.app}) — телефоны гостей подтянут новую страницу`,
    !!ver && ver.app === ver.mf && ver.app >= VER_MIN, J(ver));
  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
