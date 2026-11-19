// К11-и8 «PWA: «≈ XP» с талантом на опыт» — проверка в браузере (Chrome без окна): телефон 390×844, гость Guest46.
// Карточка идущей сессии в PWA показывает «≈ +N XP за сессию». Сервер при завершении начисляет опыт функцией
// sessionRewards: минуты × ставка × (1 + талант «Разгон опыта», +10% за уровень) × множитель ранга, кратно 5.
// PWA до К11-и8 талант не учитывала: гость с талантом 3-го уровня и рангом ×1.10 после трёх часов видел на телефоне
// «≈ +1 980 XP», а получал 2 575. Таблица ROWS — вывод серверной формулы (её три функции дословно, программа на Go,
// снята 19.11.2026); проверка заодно смотрит, что формула в серверных файлах та же, с которой таблица снята.
// Числа строит сама PWA: проверка подставляет ставку, ранг, талант и начало сессии в данные страницы и читает
// текст с экрана. Анимация счётчика выключена настройкой «меньше движения» — так страница пишет число сразу.
// В базе ничего не меняет: только вход гостя в PWA, все подстановки — в памяти страницы.
const fs = require('fs');
const path = require('path');
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

// [минуты игры, XP за минуту, уровень таланта, множитель ранга, сколько начислит сервер]
const ROWS = {
  plain: [[0, 10, 0, 1.00, 0], [1, 10, 0, 1.00, 10], [60, 10, 0, 1.00, 600], [90, 25, 0, 1.28, 2880]],
  talent: [[60, 10, 1, 1.00, 660], [60, 10, 3, 1.10, 860], [180, 10, 3, 1.10, 2575], [60, 10, 5, 1.55, 1395],
    [180, 10, 5, 1.55, 4185], [7, 10, 2, 1.18, 100], [1440, 10, 5, 1.55, 33480]],
  rate: [[45, 13, 2, 1.05, 735], [33, 7, 5, 1.40, 485]],
  // здесь формула «всё перемножить и округлить один раз» ошибается на 5 XP: сервер округляет ещё и посередине
  order: [[275, 10, 4, 1.05, 4045], [525, 10, 4, 1.05, 7720], [15, 1, 1, 1.05, 20], [24, 1, 1, 1.05, 25]],
};
const SHOW = [180, 10, 3, 1.10, 2575]; // строка для экрана и картинки
const FORMULA = [
  ['cmd/server/sessions.go', 'return int64(math.Round(float64(baseXP) * (1 + effect)))'],
  ['cmd/server/sessions.go', 'xp = models.RoundToStep(int64(math.Round(float64(boostedXP(int64(minutes)*r.XPPerMin, xpBoost))*rank.XPMult)), 5)'],
  ['internal/models/user.go', 'return (v + step/2) / step * step'],
];
const VER_MIN = '2026-11-19.2'; // версия PWA этой итерации; дальше она только растёт
const num = n => String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
const digits = s => (s == null ? null : Number(String(s).replace(/[^0-9]/g, '')));
const norm = s => (s == null ? s : String(s).replace(/[  ]/g, ' ').replace(/\s+/g, ' ').trim());
const off = (rows, got) => rows.map((r, i) => (got[i] === r[4] ? null
  : `${r[0]} мин, ставка ${r[1]}, талант ${r[2]}, ранг ×${r[3]}: ${got[i]} ≠ ${r[4]}`)).filter(Boolean);

(async () => {
  const G = 'Guest46';
  console.log('— сервер');
  const miss = FORMULA.filter(([f, line]) => {
    try { return !fs.readFileSync(path.join(__dirname, '..', '..', 'backend', f), 'utf8').includes(line); } catch (e) { return true; }
  });
  z.check('сервер: формула опыта за сессию та же, с которой снята таблица проверки (sessionRewards, boostedXP, RoundToStep)',
    miss.length === 0, miss.length ? miss.map(m => 'нет строки в backend/' + m[0] + ': ' + m[1]).join(' | ') + ' — пересними таблицу ROWS' : '');

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(async () => { await br.close().catch(() => {}); });
  const a = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2,
    reducedMotion: 'reduce' })).newPage();
  a.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await a.goto(z.BASE + '/app', { waitUntil: 'load' });
  await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
  const inOk = await a.waitForFunction(() => typeof me !== 'undefined' && me && me.id && typeof economy !== 'undefined' && economy && economy.rates
    && economy.rank && economy.talents && document.getElementById('liveCard'), null, { timeout: 20000 }).then(() => true).catch(() => false);
  const lite = inOk && await a.evaluate(() => document.body.classList.contains('perf-lite'));
  if (!inOk || !lite) {
    z.check('PWA: гость вошёл, экономика загружена, счётчики без анимации', false,
      inOk ? 'страница не включила режим без анимации — число на экране было бы промежуточным' : 'за 20 секунд страница не получила профиль и экономику');
    await z.done(); return;
  }

  // опросы сервера остановлены: иначе страница сама вернёт настоящие данные (сессии у гостя нет) поверх подстановок
  await a.evaluate(() => { stopPolls(); stopLive(); if (typeof stopComms === 'function') stopComms(); });
  const live = rows => a.evaluate(list => list.map(([m, rate, lvl, mult]) => {
    economy.rates = Object.assign({}, economy.rates, { xp_per_min: rate });
    economy.rank = Object.assign({}, economy.rank, { xp_mult: mult });
    economy.talents = Object.assign({}, economy.talents, { xp_boost: lvl * 0.1 }); // как talentEffect: уровень × 0.1
    activeSession = { id: 'k11-xp', status: 'active', paused_total_sec: 0,
      started_at: new Date(Date.now() - (m * 60 + 20) * 1000).toISOString() };
    renderLive();
    const el = document.getElementById('liveXP');
    return el ? [el.textContent, el.parentElement.textContent] : [null, null];
  }), rows);
  const got = {};
  for (const g of Object.keys(ROWS)) got[g] = (await live(ROWS[g])).map(x => digits(x[0]));

  console.log('— карточка идущей сессии в PWA');
  let d = off(ROWS.plain, got.plain);
  z.check(`без таланта число прежнее и равно серверному: час игры — ${num(got.plain[2])} XP, полтора часа при ставке 25 и ранге ×1.28 — ${num(got.plain[3])}`,
    d.length === 0, d.join('; '));
  d = off(ROWS.talent, got.talent);
  z.check(`с талантом на опыт телефон показывает столько, сколько начислит сервер: 3 ч, талант 3, ранг ×1.10 — ${num(got.talent[2])} XP (7 сочетаний, уровни таланта 1–5)`,
    d.length === 0, d.join('; '));
  d = off(ROWS.rate, got.rate);
  z.check(`талант и ставка, которую поменял владелец: 45 мин при 13 XP/мин, талант 2, ранг ×1.05 — ${num(got.rate[0])} XP`, d.length === 0, d.join('; '));
  d = off(ROWS.order, got.order);
  z.check(`округления идут в том же порядке, что на сервере: 275 мин, талант 4, ранг ×1.05 — ${num(got.order[0])} XP (один раз округлённое дало бы 4 040)`,
    d.length === 0, d.join('; '));

  const scr = (await live([SHOW]))[0];
  const want = `≈ +${num(SHOW[4])} XP за сессию`;
  await a.evaluate(() => document.getElementById('liveCard').scrollIntoView({ block: 'center' }));
  await a.locator('#liveCard').screenshot({ path: `${z.OUT}/k11_xp_live.png` }).catch(() => {});
  z.check(`на экране телефона строка целиком — «${norm(scr[1])}»`, norm(scr[1]) === want, norm(scr[1]) === want ? '' : `${J(norm(scr[1]))} ≠ ${J(want)}`);

  console.log('— киоск');
  const same = [...ROWS.plain, ...ROWS.talent, ...ROWS.rate];
  const k = await (await br.newContext({ viewport: { width: 1920, height: 1080 } })).newPage();
  await k.goto(z.BASE + '/shell', { waitUntil: 'load' });
  const kiosk = await k.evaluate(list => list.map(([m, rate, lvl, mult]) => {
    economy = { rates: { xp_per_min: rate, coins_per_min: 1 }, rank: { xp_mult: mult, coin_mult: 1 }, talents: { xp_boost: lvl * 0.1 } };
    return calcCompute(m).xp;
  }), same).catch(e => [String(e.message).split('\n')[0]]);
  await k.close();
  d = kiosk.length === same.length ? off(same, kiosk) : [J(kiosk).slice(0, 200)];
  z.check(`киоск для тех же ${same.length} сочетаний считает то же — на телефоне и на ПК клуба у гостя одна цифра`, d.length === 0, d.join('; '));

  console.log('— версия и ошибки');
  const ver = await a.evaluate(async () => ({ app: APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  z.check(`версия PWA поднята и одна в app.html и в манифесте (${ver.app}) — телефоны гостей подтянут новую страницу`,
    ver.app === ver.mf && ver.app >= VER_MIN, ver.app === ver.mf && ver.app >= VER_MIN ? '' : J(ver));
  z.check('ошибок JS в PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
