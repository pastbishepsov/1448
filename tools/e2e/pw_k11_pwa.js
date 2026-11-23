// К11-и3 «PWA: порог уровня и «монетки»» — проверка в браузере (Chrome без окна): телефон 390×844, гость Guest46.
// 1) Порог следующего уровня. Сервер считает его функцией models.XPForNextLevel и округляет до сотни, киоск считает
//    так же. PWA до К11-и3 округляла вниз до единицы: гость 4-го уровня видел на телефоне «4 645 / 6 964 XP», а на
//    киоске — «4 645 / 7 000 XP». Таблица SERVER — вывод настоящей серверной функции для уровней 1–30 (снята
//    19.11.2026); проверка заодно смотрит, что формула в серверном файле та же, с которой таблица снята.
// 2) Имя валюты. В гостевых строках PWA валюта называлась тремя словами: «монет», «койнов», «койны» (по-польски
//    «monet», «monety»). Теперь одно — «монетки» / «monetki», форма по числу, как на киоске; по-английски слово
//    «coins» прежнее, но «1 coin». Строки строит сама PWA: проверка подставляет числа в данные страницы (уровень,
//    баланс, курс, открытый кейс, приз, уведомление) и читает текст с экрана.
// В базе ничего не меняет: только вход гостя в PWA, все подстановки — в памяти страницы.
const fs = require('fs');
const path = require('path');
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

const SERVER = [1000, 2600, 4700, 7000, 9500, 12300, 15200, 18400, 21700, 25100, 28700, 32400, 36300, 40200, 44300,
  48500, 52800, 57200, 61700, 66300, 71000, 75800, 80600, 85600, 90600, 95700, 100900, 106200, 111500, 116900];
const FORMULA = ['raw := 1000 * math.Pow(float64(level), 1.4)', 'return int64(math.Round(raw/100)) * 100'];
const VER_MIN = '2026-11-19.1'; // версия PWA этой итерации; дальше она только растёт
// слово по числу: ожидания выписаны руками, а не той же формулой, что в странице
const NUMS = [1, 2, 5, 11, 21, 22, 104, 1500];
const WORD = {
  ru: ['монетка', 'монетки', 'монеток', 'монеток', 'монетка', 'монетки', 'монетки', 'монеток'],
  en: ['coin', 'coins', 'coins', 'coins', 'coins', 'coins', 'coins', 'coins'],
  pl: ['monetka', 'monetki', 'monetek', 'monetek', 'monetek', 'monetki', 'monetki', 'monetek'],
};
const NTF = { ru: (n, w) => `💰 +${n} ${w} — депозит 12,50 zł`, en: (n, w) => `💰 +${n} ${w} — top-up 12.50 zł`,
  pl: (n, w) => `💰 +${n} ${w} — wpłata 12,50 zł` };
// кошелёк на вкладке «Прогресс»: [баланс, монеток за 1 zł, монеток за минуту игры] → подпись баланса и строка курса
const WAL = [[1, 100, 1], [3, 22, 5], [1250, 1, 0.5]];
const WAL_WANT = {
  ru: [['монетка на балансе', '1 zł → 100 монеток · игра: +1 монетка/мин'], ['монетки на балансе', '1 zł → 22 монетки · игра: +5 монеток/мин'],
    ['монеток на балансе', '1 zł → 1 монетка · игра: +0.5 монетки/мин']],
  en: [['coin in balance', '1 zł → 100 coins · play: +1 coin/min'], ['coins in balance', '1 zł → 22 coins · play: +5 coins/min'],
    ['coins in balance', '1 zł → 1 coin · play: +0.5 coins/min']],
  pl: [['monetka na koncie', '1 zł → 100 monetek · gra: +1 monetka/min'], ['monetki na koncie', '1 zł → 22 monetki · gra: +5 monetek/min'],
    ['monetek na koncie', '1 zł → 1 monetka · gra: +0.5 monetki/min']],
};
const CASE_WANT = { ru: ['открыт: +1 монетка', 'открыт: +150 монеток'], en: ['opened: +1 coin', 'opened: +150 coins'],
  pl: ['otwarta: +1 monetka', 'otwarta: +150 monetek'] };
const PRIZE_WANT = { ru: ['монетка · кейс Medium', 'монеток · кейс Medium'], en: ['coin · Medium case', 'coins · Medium case'],
  pl: ['monetka · skrzynka Medium', 'monetek · skrzynka Medium'] };
const LS = ['ru', 'en', 'pl'];
const num = n => String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
const norm = s => (s == null ? s : String(s).replace(/[  ]/g, ' '));
const deep = o => JSON.parse(J(o), (k, v) => (typeof v === 'string' ? norm(v) : v));
const diff = (got, want) => LS.flatMap(l => want[l].map((x, i) => (J(got[l] && got[l][i]) === J(x) ? null : `${l}: ${J(got[l] && got[l][i])} ≠ ${J(x)}`)))
  .filter(Boolean);

(async () => {
  const G = 'Guest46';
  console.log('— порог уровня');
  let src = '';
  try { src = fs.readFileSync(path.join(__dirname, '..', '..', 'backend', 'internal', 'models', 'user.go'), 'utf8'); } catch (e) { src = ''; }
  const fOk = FORMULA.every(f => src.includes(f));
  z.check('сервер: формула порога в models.XPForNextLevel та же, с которой снята таблица проверки (округление до сотни)', fOk,
    fOk ? '' : (src ? 'формула в backend/internal/models/user.go изменилась — пересними таблицу SERVER' : 'нет файла user.go'));

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(async () => { await br.close().catch(() => {}); });
  const a = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2 })).newPage();
  a.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await a.goto(z.BASE + '/app', { waitUntil: 'load' });
  await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
  const inOk = await a.waitForFunction(() => typeof me !== 'undefined' && me && me.id && typeof economy !== 'undefined' && economy && economy.rates,
    null, { timeout: 20000 }).then(() => true).catch(() => false);
  if (!inOk) { z.check('PWA: гость вошёл, профиль и курс загружены', false, 'за 20 секунд страница не получила профиль'); return z.done(); }

  const mine = await a.evaluate(() => Array.from({ length: 30 }, (_, i) => xpNext(i + 1)));
  const off = SERVER.map((s, i) => (mine[i] === s ? null : `ур. ${i + 1}: ${mine[i]} ≠ ${s}`)).filter(Boolean);
  z.check(`PWA: пороги уровней 1–30 равны серверным (4-й уровень — ${num(mine[3])})`, off.length === 0, off.slice(0, 6).join('; '));

  const k = await (await br.newContext({ viewport: { width: 1920, height: 1080 } })).newPage();
  await k.goto(z.BASE + '/shell', { waitUntil: 'load' });
  const kiosk = await k.evaluate(() => Array.from({ length: 30 }, (_, i) => xpNext(i + 1))).catch(e => [String(e.message).split('\n')[0]]);
  await k.close();
  z.check('киоск считает те же 30 порогов — на телефоне и на ПК клуба у гостя одна цифра', J(kiosk) === J(SERVER),
    J(kiosk) === J(SERVER) ? '' : J(kiosk).slice(0, 200));

  // шапка: гость 4-го уровня с 4 645 XP (числа — в памяти страницы, отрисовка — её собственная)
  const head = deep(await a.evaluate(() => {
    STR = Object.assign({}, STR_RU); LANG = 'ru';
    window.__k113 = { level: me.level, xp: me.xp_current };
    me.level = 4; me.xp_current = 4645; renderTop();
    const t = id => document.getElementById(id).textContent;
    return { xp: t('xpText'), left: t('xpLeft'), lvl: t('hLvl'), bar: document.getElementById('xpBar').style.width };
  }));
  await a.locator('header.topbar').screenshot({ path: `${z.OUT}/k113_head.png` }).catch(e => console.log('кадр шапки: ' + e.message.split('\n')[0]));
  z.check(`PWA, шапка: 4-й уровень, 4 645 XP — «${head.xp}», «${head.left}»`,
    head.xp === '4 645 / 7 000 XP' && head.left === 'до уровня: 2 355 XP' && head.lvl === '4' && /^66\.3/.test(head.bar), J(head));
  await a.evaluate(() => { me.level = window.__k113.level; me.xp_current = window.__k113.xp; renderTop(); });

  console.log('— имя валюты');
  const dict = await a.evaluate(() => {
    const bad = (d, re) => Object.entries(d).filter(([, v]) => typeof v === 'string' && re.test(v)).map(([key, v]) => key + ': ' + v.slice(0, 48));
    return { ru: bad(STR_RU, /койн|коин|монет(?!к|ок)/i), pl: bad(STR_PL, /monet(?!k|ek)/i),
      forms: [STR_RU, STR_EN, STR_PL].map(d => ['pl_coin_1', 'pl_coin_2', 'pl_coin_5'].map(key => d[key] || '')) };
  });
  z.check(`PWA: в гостевых строках одно имя валюты — по-русски только «монетки», по-польски только «monetki» `
    + `(другим словом: ${dict.ru.length} рус., ${dict.pl.length} польск.); формы слова есть в трёх языках`,
    dict.ru.length === 0 && dict.pl.length === 0
    && J(dict.forms) === J([['монетка', 'монетки', 'монеток'], ['coin', 'coins', 'coins'], ['monetka', 'monetki', 'monetek']]),
    J({ ru: dict.ru, pl: dict.pl, forms: dict.forms }).slice(0, 700));

  const ntf = deep(await a.evaluate(nums => {
    const save = [STR, LANG], out = {};
    for (const l of ['ru', 'en', 'pl']) { STR = Object.assign({}, STR_RU, LANGS[l]); LANG = l;
      out[l] = nums.map(n => ntfText({ type: 'deposit', payload: { coins: n, amount_pln: 12.5 } })); }
    [STR, LANG] = save;
    return out;
  }, NUMS));
  const ntfWant = Object.fromEntries(LS.map(l => [l, NUMS.map((n, i) => NTF[l](num(n), WORD[l][i]))]));
  const dN = diff(ntf, ntfWant);
  z.check(`PWA, уведомление о пополнении: слово по числу для ${NUMS.join(', ')} — «${ntf.ru[0]}», «${ntf.ru[1]}», «${ntf.ru[2]}»; `
    + `EN «${ntf.en[0]}», PL «${ntf.pl[5]}»`, dN.length === 0, dN.slice(0, 5).join(' | '));

  // вкладка «Прогресс»: кошелёк гостя — сначала кадр с его настоящими числами, потом подстановки
  await a.evaluate(async () => { setTab('progress'); await loadProgress(); STR = Object.assign({}, STR_RU); LANG = 'ru'; renderHist();
    document.getElementById('histBal').scrollIntoView({ block: 'center' }); });
  await a.waitForTimeout(900);
  const box = await a.evaluate(() => { const c = document.getElementById('histCard').getBoundingClientRect(),
    r = document.getElementById('histBal').parentElement.getBoundingClientRect(); return { x: c.x, w: c.width, y: r.y, h: r.height }; });
  await a.screenshot({ path: `${z.OUT}/k113_wallet.png`, clip: { x: Math.max(0, box.x), y: Math.max(0, box.y - 52), width: Math.min(390, box.w), height: box.h + 96 } })
    .catch(e => console.log('кадр кошелька: ' + e.message.split('\n')[0]));
  const wal = deep(await a.evaluate(cases => {
    const save = { STR, LANG, bal: me.coins_balance, rates: economy.rates }, out = {};
    for (const l of ['ru', 'en', 'pl']) { STR = Object.assign({}, STR_RU, LANGS[l]); LANG = l;
      out[l] = cases.map(([bal, c, m]) => {
        me.coins_balance = bal; economy.rates = Object.assign({}, save.rates, { deposit_coins_per_pln: c, coins_per_min: m });
        renderHist();
        const row = document.getElementById('histBal').parentElement, lab = document.getElementById('histBal').nextElementSibling;
        return [lab ? lab.textContent : null, row.nextElementSibling ? row.nextElementSibling.textContent : null];
      }); }
    STR = save.STR; LANG = save.LANG; me.coins_balance = save.bal; economy.rates = save.rates; renderHist();
    return out;
  }, WAL));
  const dL = diff({ ru: wal.ru.map(x => x[0]), en: wal.en.map(x => x[0]), pl: wal.pl.map(x => x[0]) },
    Object.fromEntries(LS.map(l => [l, WAL_WANT[l].map(x => x[0])])));
  z.check(`PWA, кошелёк: подпись баланса по числу (1, 3, 1 250) — «${wal.ru[0][0]}», «${wal.ru[1][0]}», «${wal.ru[2][0]}»; EN и PL так же`,
    dL.length === 0, dL.slice(0, 5).join(' | '));
  const dR = diff({ ru: wal.ru.map(x => x[1]), en: wal.en.map(x => x[1]), pl: wal.pl.map(x => x[1]) },
    Object.fromEntries(LS.map(l => [l, WAL_WANT[l].map(x => x[1])])));
  z.check(`PWA, кошелёк: строка курса — «${wal.ru[0][1]}»; дробное — «${wal.ru[2][1]}»; EN и PL так же`,
    dR.length === 0, dR.slice(0, 4).join(' | '));

  const cs = deep(await a.evaluate(() => {
    const save = { STR, LANG, cases }, out = {};
    for (const l of ['ru', 'en', 'pl']) { STR = Object.assign({}, STR_RU, LANGS[l]); LANG = l;
      out[l] = [1, 150].map(n => {
        cases = [{ id: 'k113', tier: 'medium', source: 'session', opened_at: new Date().toISOString(),
          expires_at: new Date(Date.now() + 5 * 864e5).toISOString(), drop_type: 'coins', drop_amount: n }];
        renderCases();
        const s = document.querySelector('#casesList .case-row .muted.cnum');
        return s ? s.textContent : null;
      }); }
    STR = save.STR; LANG = save.LANG; cases = save.cases; renderCases();
    return out;
  }));
  const dC = diff(cs, CASE_WANT);
  z.check(`PWA, список кейсов: открытый кейс — «${cs.ru[0]}», «${cs.ru[1]}»; EN «${cs.en[0]}», PL «${cs.pl[1]}»`,
    dC.length === 0, dC.slice(0, 5).join(' | '));

  const pz = deep(await a.evaluate(() => {
    const save = { STR, LANG }, out = {};
    for (const l of ['ru', 'en', 'pl']) { STR = Object.assign({}, STR_RU, LANGS[l]); LANG = l;
      out[l] = [1, 150].map(n => { showPrize({ tier: 'medium', drop_type: 'coins', drop_amount: n }); return document.getElementById('coWhat').textContent; }); }
    document.getElementById('coPrize').classList.add('hidden'); closeCase();
    STR = save.STR; LANG = save.LANG; renderTop();
    return out;
  }));
  const dP = diff(pz, PRIZE_WANT);
  z.check(`PWA, приз из кейса: подпись под числом — «${pz.ru[0]}», «${pz.ru[1]}»; EN «${pz.en[0]}», PL «${pz.pl[1]}»`,
    dP.length === 0, dP.slice(0, 5).join(' | '));

  console.log('— версия и ошибки');
  const ver = await a.evaluate(async () => ({ app: APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  z.check(`версия PWA поднята и одна в app.html и в манифесте (${ver.app}) — телефоны гостей подтянут новую страницу`,
    ver.app === ver.mf && ver.app >= VER_MIN, J(ver));
  z.check('ошибок JS в PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
