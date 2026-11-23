// К11-и1 «Тексты киоска» — проверка в браузере (Chrome без окна): Guest46 за PC-07, сессия идёт.
// 1. Словари RU/EN/PL: ключи совпадают; в русских строках нет «койнов» и «монет» (остаётся имя таланта «Монетный
//    двор»), в польских — одно слово «monetki»; число рядом со словом нигде не зашито; ни в одной
//    статичной подписи не виден незаполненный «{…}».
// 2. Формы числа: «1 монетка, 2 монетки, 5 монеток, 21 монетка», «1 очко, 2 очка, 5 очков»; по-польски форма «1» —
//    только само 1 («21 monetek», «21 gier»); по-английски coin/coins; дробное — «0,5 монетки».
// 3. Шапка: подписи у чисел согласованы с числом на трёх языках; ряд шапки — один ряд на 1920×1080 и 1440×900
//    (подписи видны), на 1366×768 и 1280×800 (подписи скрыты, как задумано в З19-и5; порог был «уже 1366», и на
//    самих 1366×768 шапка с подписями шла в два ряда — К11-и1 сдвинул порог на 1366). Всё — на трёх языках.
// 4. Уведомления: депозит, сгоревшие монетки, «монетки начнут таять», очки за ачивку.
// 5. Вкладка «Монетки»: имя вкладки, подпись калькулятора согласована с числом, «Как образуются монетки», ставка.
// 6. Шансы кейса («монетки a–b», «джекпот N монеток») и подпись приза под числом («монетка · кейс …»).
// 7. «Инфо»: вместо «{ic}» — значок; «ОТКРЫТЬ ▸» одной строкой и внутри строки кейса; «резет через 16 ч 50 мин»;
//    у «Кухни» значок из набора; папка с Discord — «Войсчат» / Voice chat / Czat głosowy.
// В базе ничего не меняет: гостя сажает и сессию закрывает сама, кошелёк возвращает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

const LANGS = ['ru', 'en', 'pl'];
const NS = [0, 1, 2, 4, 5, 11, 12, 14, 21, 22, 25, 101, 111, 0.5];
// какая форма у какого числа из NS: 0 — «1 монетка», 1 — «2 монетки», 2 — «5 монеток»; таблицы выписаны руками
const TAB = { ru: [2, 0, 1, 1, 2, 2, 2, 2, 0, 1, 2, 0, 2, 1], en: [2, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2],
  pl: [2, 0, 1, 1, 2, 2, 2, 2, 2, 1, 2, 2, 2, 1] };
const W = { ru: { coin: ['монетка', 'монетки', 'монеток'], sp: ['очко', 'очка', 'очков'] },
  en: { coin: ['coin', 'coins', 'coins'], sp: ['point', 'points', 'points'] },
  pl: { coin: ['monetka', 'monetki', 'monetek'], sp: ['punkt', 'punkty', 'punktów'] } };
// то же правило для чисел, которых нет в таблице (остаток гостя, джекпот со стенда); ниже сверяется с таблицей
const form = (l, n) => {
  if (l === 'en') return n === 1 ? 0 : 2;
  if (!Number.isInteger(n)) return 1;
  const a = n % 10, b = n % 100;
  if (l === 'pl' ? n === 1 : (a === 1 && b !== 11)) return 0;
  return a >= 2 && a <= 4 && (b < 12 || b > 14) ? 1 : 2;
};
const coin = (l, n) => W[l].coin[form(l, n)], sp = (l, n) => W[l].sp[form(l, n)];
const num = s => +String(s).replace(/\s/g, '').replace(',', '.');
const same = (a, b) => J(a) === J(b);
const ru = n => n.toLocaleString('ru-RU').replace(/\s/g, ' ');

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
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k11_texts: проверка вернула кошелёк', now() FROM u WHERE u.w <> ${v}`);
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
  z.check('правило форм числа в самой проверке сходится с таблицей, выписанной руками',
    LANGS.every(l => same(NS.map(n => form(l, n)), TAB[l])), J(LANGS.map(l => NS.map(n => form(l, n)))));

  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  z.check('гость посажен за PC-07, сессия идёт', !!(seat.data && seat.data.session_id), J(seat.data));

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const AGENT_OK = { 'Access-Control-Allow-Origin': '*' };
  const ctx = await br.newContext({ viewport: { width: 1920, height: 1080 } });
  await ctx.route('http://127.0.0.1:1448/**', rt => {
    const path = new URL(rt.request().url()).pathname;
    const body = path === '/ping' ? { computer_id: pcId } : path === '/apps-status' ? { apps: {} } : null;
    return rt.fulfill(body ? { status: 200, contentType: 'application/json', headers: AGENT_OK, body: J(body) }
      : { status: 404, headers: AGENT_OK, body: '' });
  });
  const p = await ctx.newPage();
  p.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  const until = (fn, arg, ms) => p.waitForFunction(fn, arg, { timeout: ms }).then(() => true).catch(() => false);
  const shot = async (sel, name) => {
    try { await p.locator(sel).first().screenshot({ path: `${z.OUT}/k11_${name}.png`, timeout: 5000 }); }
    catch (e) { console.log('кадр', name, e.message.split('\n')[0]); }
  };
  const size = async (wd, ht) => { await p.setViewportSize({ width: wd, height: ht }); await p.waitForTimeout(250); };

  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
  await until(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'), null, 15000);
  await p.waitForSelector('#scLeft', { timeout: 8000 }).catch(() => {});
  await p.waitForTimeout(900); // числа в шапке досчитали
  const has = await p.evaluate(() => {
    // выполнить fn на другом языке и тут же вернуть язык (настоящая смена языка перезагружает страницу)
    window.__k11 = (lang, fn) => {
      const keep = { STR, LANG };
      try { LANG = lang; STR = Object.assign({}, SH_RU, SH_LANGS[lang]); applyStrings(); return fn(); }
      finally { LANG = keep.LANG; STR = keep.STR; applyStrings(); }
    };
    // сменить язык надолго (на время замера и кадра) и перерисовать всё, как после загрузки; __k11back — вернуть
    window.__k11all = () => { applyStrings(); renderTop(); renderWallet(); renderSession(); renderSide(); renderLock(); };
    window.__k11set = l => { window.__k11keep = window.__k11keep || { STR, LANG }; LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]); window.__k11all(); };
    window.__k11back = () => { const k = window.__k11keep; if (k) { LANG = k.LANG; STR = k.STR; window.__k11keep = null; window.__k11all(); } };
    window.__k11t = e => (e ? e.textContent.replace(/\s+/g, ' ').trim() : null);
    return { fn: typeof plCoin === 'function' && typeof plSp === 'function' && typeof topLabels === 'function', lang: LANG };
  });
  z.atEnd(() => p.evaluate(() => window.__k11back()).catch(() => {}));
  z.check('в киоске есть формы числа для монеток и очков и подписи шапки по числу; язык гостя — русский', has.fn && has.lang === 'ru', J(has));

  console.log('— 1. словари');
  const d = await p.evaluate(() => {
    const D = [SH_RU, SH_EN, SH_PL], all = [...new Set(D.flatMap(o => Object.keys(o)))];
    const hit = (o, rx) => Object.keys(o).filter(k => rx.test(String(o[k]))).sort();
    return { n: D.map(o => Object.keys(o).length), diff: all.filter(k => !D.every(o => k in o)),
      ruOld: hit(SH_RU, /койн|монет(?!к|ок|н)/i), ruMint: hit(SH_RU, /монетн/i), plOld: hit(SH_PL, /coin|monet(?!k|ek)/i),
      numRu: hit(SH_RU, /\{\w+\} (монет|очк)/i), numEn: hit(SH_EN, /\{\w+\} (coins?|points?)\b/i), numPl: hit(SH_PL, /\{\w+\} (monet|punkt)/i),
      gone: ['tb_coins', 'tb_sp', 'calc_coins_lbl'].filter(k => D.some(o => k in o)), ic: D.map(o => hit(o, /\{ic:\w+\}/).join()),
      // статичные подписи (data-str и родня): переменные в них никто не подставляет — «{…}» был бы виден гостю как текст
      raw: ['ru', 'en', 'pl'].map(l => window.__k11(l, () => {
        const rx = /\{[\w:]+\}/, ds = e => e.dataset.str || e.dataset.strHtml || e.dataset.strPh || e.dataset.strTitle;
        const q = (sel, f) => [...document.querySelectorAll(sel)].filter(e => rx.test(f(e) || '')).map(ds);
        return [...q('[data-str],[data-str-html]', e => e.textContent), ...q('[data-str-ph]', e => e.placeholder), ...q('[data-str-title]', e => e.title)];
      })),
      words: D.map(o => ['pl_coin_1', 'pl_coin_2', 'pl_coin_5', 'pl_sp_1', 'pl_sp_2', 'pl_sp_5'].map(k => o[k] || null)) };
  });
  z.check(`ключи трёх словарей совпадают (${d.n.join(' / ')})`, d.diff.length === 0 && d.n[0] === d.n[1] && d.n[1] === d.n[2], J(d.diff).slice(0, 400));
  z.check('в русских строках нет «койнов» и «монет» — везде «монетки» (остаётся только имя таланта «Монетный двор»)',
    d.ruOld.length === 0 && same(d.ruMint, ['how_mint']), J([d.ruOld, d.ruMint]));
  z.check('в польских строках одно слово — «monetki»: ни «coiny», ни «monety»', d.plOld.length === 0, J(d.plOld));
  z.check('число рядом со словом «монетки» или «очки» нигде не зашито в строку — слово подставляется по числу; старых подписей шапки и калькулятора нет',
    d.numRu.length + d.numEn.length + d.numPl.length + d.gone.length === 0, J([d.numRu, d.numEn, d.numPl, d.gone]));
  z.check('слова для форм числа есть на трёх языках', LANGS.every((l, i) => same(d.words[i], [...W[l].coin, ...W[l].sp])), J(d.words));
  z.check('ни в одной статичной подписи киоска не виден незаполненный «{…}» (три языка); значок в тексте «Инфо» задан именем — «{ic:percent}»',
    d.raw.every(a => a.length === 0) && same(d.ic, ['info_li3', 'info_li3', 'info_li3']), J([d.raw, d.ic]));

  console.log('— 2. формы числа');
  const pf = !has.fn ? null : await p.evaluate(ns => Object.fromEntries(['ru', 'en', 'pl'].map(l => [l, window.__k11(l, () => ({
    coin: ns.map(n => plCoin(n)), sp: ns.map(n => plSp(n)), game: ns.map(n => plGame(n)), g: [STR.pl_game_1, STR.pl_game_2, STR.pl_game_5] }))])), NS);
  for (const l of LANGS) {
    z.check(`${l}: ${[1, 2, 5, 21].map(n => `${n} ${coin(l, n)}`).join(', ')}; ${[1, 2, 5].map(n => `${n} ${sp(l, n)}`).join(', ')} — и остальные числа таблицы, дробное тоже`,
      !!pf && same(pf[l].coin, TAB[l].map(i => W[l].coin[i])) && same(pf[l].sp, TAB[l].map(i => W[l].sp[i]))
      && same(pf[l].game, TAB[l].map(i => pf[l].g[i])), J(pf && pf[l]).slice(0, 500));
  }

  console.log('— 3. шапка');
  const top = !has.fn ? null : await p.evaluate(nums => {
    const keep = [me.coins_balance, me.skillpoints_available];
    const cap = () => [document.getElementById('coinsLbl').textContent, document.getElementById('spLbl').textContent];
    const out = { c: keep[0], s: keep[1], real: cap(), sub: {} };
    try {
      for (const l of ['ru', 'en', 'pl']) {
        out.sub[l] = window.__k11(l, () => nums.map(n => { me.coins_balance = n; me.skillpoints_available = n; topLabels(); return cap(); }));
      }
      me.coins_balance = 1; me.skillpoints_available = 2; renderTop(); out.wired = cap(); // обычная перерисовка шапки
    } finally { me.coins_balance = keep[0]; me.skillpoints_available = keep[1]; renderTop(); }
    out.back = cap();
    return out;
  }, [1, 2, 5, 21]);
  z.check(`шапка: подписи согласованы с настоящими числами гостя (${top ? `${ru(top.c)} ${top.real[0]}, ${top.s} ${top.real[1]}` : '—'})`,
    !!top && same(top.real, [coin('ru', top.c), sp('ru', top.s)]) && same(top.back, top.real), J(top && [top.real, top.back]));
  for (const l of LANGS) {
    z.check(`шапка, ${l}: ${[1, 2, 5, 21].map(n => `${n} ${coin(l, n)} / ${n} ${sp(l, n)}`).join(', ')}`,
      !!top && same(top.sub[l], [1, 2, 5, 21].map(n => [coin(l, n), sp(l, n)])), J(top && top.sub[l]));
  }
  z.check('шапка: подпись меняется вместе с числом при обычной перерисовке (1 монетка, 2 очка)', !!top && same(top.wired, ['монетка', 'очка']), J(top && top.wired));
  await p.waitForTimeout(1200);

  // замер ряда шапки — как в pw_z19_kiosk_head; очков подставляем 55: самая длинная подпись («очков», «punktów»)
  const measure = () => p.evaluate(() => {
    const tb = document.querySelector('.topbar'), dot = document.getElementById('agentDot'), spEl = document.getElementById('sp');
    const keep = [me.skillpoints_available, spEl.textContent];
    me.skillpoints_available = 55; spEl.textContent = '55';
    if (typeof topLabels === 'function') topLabels();
    try {
      const vis = e => getComputedStyle(e).display !== 'none' && e.getBoundingClientRect().width > 0;
      const R = e => { const q = e.getBoundingClientRect(); return { t: Math.round(q.top), r: Math.round(q.right), b: Math.round(q.bottom) }; };
      const row = [...tb.children].filter(e => e !== dot && vis(e)).map(R), rl = [...tb.querySelectorAll('.res .rl')];
      return { W: innerWidth, sw: tb.scrollWidth, cw: tb.clientWidth, h: Math.round(tb.getBoundingClientRect().height),
        oneRow: Math.max(...row.map(q => q.t)) < Math.min(...row.map(q => q.b)), maxR: Math.max(...row.map(q => q.r)),
        labels: rl.filter(vis).length, cap: rl.map(e => e.textContent).join(' / ') };
    } finally { me.skillpoints_available = keep[0]; spEl.textContent = keep[1]; if (typeof topLabels === 'function') topLabels(); }
  });
  const inLang = async (l, fn) => {
    if (l === 'ru') return fn();
    await p.evaluate(l => window.__k11set(l), l);
    try { await p.waitForTimeout(900); return await fn(); } finally { await p.evaluate(() => window.__k11back()); await p.waitForTimeout(300); }
  };
  const rowOk = m => m.oneRow && m.sw <= m.cw && m.maxR <= m.W;
  for (const [wd, ht] of [[1920, 1080], [1440, 900], [1366, 768], [1280, 800]]) {
    await size(wd, ht);
    const ms = [];
    for (const l of LANGS) {
      ms.push(await inLang(l, async () => {
        const m = await measure();
        if ((wd === 1920 && l === 'ru') || (wd === 1440 && l === 'pl') || (wd === 1366 && l === 'pl')) await shot('.topbar', `top_${l}_${wd}`);
        return m;
      }));
    }
    if (wd > 1366) {
      z.check(`${wd}×${ht}: шапка — один ряд, на экране целиком, подписи у чисел видны; на трёх языках («${ms.map(m => m.cap).join('», «')}»)`,
        ms.every(m => rowOk(m) && m.labels === 2), J(ms));
    } else {
      z.check(`${wd}×${ht} (1366 и уже): подписи у чисел скрыты, шапка — один ряд на экране; на трёх языках`, ms.every(m => rowOk(m) && m.labels === 0), J(ms));
    }
  }
  await size(1920, 1080);

  console.log('— 4. уведомления');
  const nt = await p.evaluate(() => {
    const tx = (type, payload) => String(notifText({ type, payload })).replace(/\s/g, ' ');
    return Object.fromEntries(['ru', 'en', 'pl'].map(l => [l, window.__k11(l, () => ({
      dep: [1, 2, 5, 1500].map(c => tx('deposit', { coins: c, amount_pln: 12.5 })),
      burned: [1, 2, 5, 21].map(c => tx('coins_burned', { coins: c, idle_days: 30, left: 40 })),
      soon: tx('coins_burn_soon', { days: 3, pct: 5, coins: 200 }),
      ach: [1, 2, 5].map(n => tx('achievement_earned', { title: 'Проба', reward_skillpoints: n })) }))]));
  });
  const DEP = { ru: (c, w) => `💰 +${c} ${w} — депозит 12,50 zł`, en: (c, w) => `💰 +${c} ${w} — top-up 12.50 zł`, pl: (c, w) => `💰 +${c} ${w} — wpłata 12,50 zł` };
  const BRN = { ru: c => `🔥 Сгорело монеток: ${c} за 30 дн простоя — осталось 40`, en: c => `🔥 ${c} ${coin('en', c)} burned after 30 idle days — 40 left`,
    pl: c => `🔥 Spłonęło monetek: ${c} po 30 dniach przerwy — zostało 40` };
  const SOON = { ru: 'монетки начнут таять', en: 'your coins start melting', pl: 'monetki zaczną topnieć' };
  const ACH = { ru: n => ` · +${n} ${sp('ru', n)}`, en: n => ` · +${n} ${sp('en', n)}`, pl: n => ` · +${n} pkt` };
  for (const l of LANGS) {
    const a = nt[l];
    z.check(`уведомления, ${l}: «${DEP[l](1, coin(l, 1))}», «+1 500 ${coin(l, 1500)}»; «${BRN[l](21)}»; «…${SOON[l]}…»; «…${ACH[l](2).slice(3)}»`,
      same(a.dep, [1, 2, 5, 1500].map(c => DEP[l](ru(c), coin(l, c)))) && same(a.burned, [1, 2, 5, 21].map(BRN[l]))
      && a.soon.includes(SOON[l]) && [1, 2, 5].every((n, i) => a.ach[i].endsWith(ACH[l](n))), J(a));
  }

  console.log('— 5. вкладка «Монетки»');
  await p.click('#st-coins');
  await until(() => curTab === 'coins' && !!document.getElementById('calcCoins'), null, 8000);
  const ct = await p.evaluate(() => {
    const t = window.__k11t, g = id => t(document.getElementById(id)), body = () => t(document.getElementById('sideBody'));
    const one = () => ({ tab: g('st-coins'), body: body(), n: g('calcCoins'), lbl: g('calcCoinsLbl') });
    const out = { ru: one(), slide: [] }, keep = calcMinutes;
    for (const m of [15, 300, 600]) { calcSlide(m); out.slide.push([g('calcCoins'), g('calcCoinsLbl')]); }
    calcSlide(keep);
    for (const l of ['en', 'pl']) out[l] = window.__k11(l, () => { renderSide(); return one(); });
    renderSide();
    return out;
  });
  await shot('#sideBody', 'coins');
  z.check('вкладка называется «Монетки» (Coins, Monetki); в ней нет «койнов», есть «Как образуются монетки»; по-польски — только «monetki»',
    ct.ru.tab === 'Монетки' && ct.en.tab === 'Coins' && ct.pl.tab === 'Monetki' && !/койн/i.test(ct.ru.body) && ct.ru.body.includes('Как образуются монетки')
    && !/coin|monet(?!k|ek)/i.test(ct.pl.body), J([ct.ru.tab, ct.en.tab, ct.pl.tab, (ct.pl.body.match(/.{0,25}(coin|monet(?!k|ek)).{0,25}/i) || [''])[0]]));
  z.check(`калькулятор: слово под числом согласовано с ним (${ct.slide.map(s => s.join(' ')).join('; ')}; EN ${ct.en.n} ${ct.en.lbl}, PL ${ct.pl.n} ${ct.pl.lbl})`,
    ct.slide.every(([n, w]) => !!w && w === coin('ru', num(n))) && LANGS.every(l => !!ct[l].lbl && ct[l].lbl === coin(l, num(ct[l].n))), J(ct.slide));
  const RATE = { ru: /Базовая ставка — ([\d.,]+) (\S+?)\/мин/, en: /Base rate — ([\d.,]+) (\S+?)\/min/, pl: /Stawka bazowa — ([\d.,]+) (\S+?)\/min/ };
  const rate = LANGS.map(l => { const m = ct[l].body.match(RATE[l]); return m ? [m[1], m[2], coin(l, num(m[1]))] : null; });
  z.check(`ставка в «Как образуются монетки» согласована с числом: ${rate.map(r => (r ? `${r[0]} ${r[1]}` : '—')).join(', ')} в минуту`,
    rate.every(r => !!r && r[1] === r[2]), J(rate));

  console.log('— 6. шансы и приз');
  const od = await p.evaluate(async () => {
    const t = window.__k11t, ev = { stopPropagation() {}, preventDefault() {} }, out = { rows: {}, prize: {}, jp: {} };
    await openOdds(ev, 'light', ''); // первый раз — с сервера; дальше из кэша, без ожиданий
    const rows = () => [...document.querySelectorAll('#oddsBody .odds-row .l')].map(e => t(e));
    for (const l of ['ru', 'en', 'pl']) out.rows[l] = window.__k11(l, () => { openOdds(ev, 'light', ''); return rows(); });
    closeOverlay('oddsOverlay');
    const els = ['coStage', 'coTitle', 'coBox', 'coAmount', 'coType', 'coResult'].map(id => document.getElementById(id));
    const snap = els.map(e => [e.className, e.getAttribute('style')]), inner = els.slice(1, 5).map(e => e.innerHTML);
    const fx = [FX.confetti, FX.countUp];
    FX.confetti = () => {}; FX.countUp = (el, v) => { el.textContent = String(v); }; // без конфетти и счёта на экране
    try {
      for (const l of ['ru', 'en', 'pl']) {
        out.prize[l] = window.__k11(l, () => { out.jp[l] = STR.co_jackpot; return [1, 2, 5, 21, 100000].map(n => {
          showPrize({ drop_type: 'coins', drop_amount: n, tier: 'light' }, [10, 50, 100000]); return t(document.getElementById('coType')); }); });
      }
    } finally {
      FX.confetti = fx[0]; FX.countUp = fx[1];
      els.forEach((e, i) => { e.className = snap[i][0]; if (snap[i][1] == null) e.removeAttribute('style'); else e.setAttribute('style', snap[i][1]); });
      els.slice(1, 5).forEach((e, i) => { e.innerHTML = inner[i]; });
    }
    return out;
  });
  const JP = { ru: /^джекпот ([\d ]+) (\S+)$/, en: /^jackpot ([\d ]+) (\S+)$/, pl: /^jackpot ([\d ]+) (\S+)$/ };
  const CN = { ru: /^монетки [\d ]+–[\d ]+$/, en: /^coins [\d ]+–[\d ]+$/, pl: /^monetki [\d ]+–[\d ]+$/ };
  for (const l of LANGS) {
    const r = od.rows[l] || [], jp = r.map(s => s.match(JP[l])).filter(Boolean), cn = r.filter(s => CN[l].test(s));
    z.check(`шансы кейса, ${l}: «${cn[0] || '—'}», «${jp[0] ? jp[0][0] : '—'}» — слово согласовано с числом`,
      cn.length > 0 && jp.length > 0 && jp.every(m => m[2] === coin(l, num(m[1]))), J(r).slice(0, 400));
  }
  const PZ = { ru: (w, t) => `${w} · кейс ${t}`, en: (w, t) => `${w} · ${t} case`, pl: (w, t) => `${w} · skrzynka ${t}` };
  for (const l of LANGS) {
    const want = [1, 2, 5, 21].map(n => PZ[l](coin(l, n), 'Light')).concat(`${od.jp[l]} · ${PZ[l](coin(l, 100000), 'Light')}`);
    z.check(`подпись приза под числом, ${l}: ${want.slice(0, 3).map(s => `«${s}»`).join(', ')}; джекпот — «${want[4]}»`, same(od.prize[l], want), J(od.prize[l]));
  }

  console.log('— 7. «Инфо», «ОТКРЫТЬ», резет, «Кухня», «Войсчат»');
  const info = await p.evaluate(() => {
    const li = () => document.querySelector('li[data-str-html="info_li3"]');
    const tmp = document.createElement('span'); tmp.innerHTML = UI.percent; // тот же значок, что на кнопке шансов у кейса
    const one = () => ({ svg: li().querySelectorAll('svg.isvg').length, raw: /\{[\w:]+\}/.test(li().textContent),
      btn: !!li().querySelector('svg') && li().querySelector('svg').outerHTML === tmp.firstChild.outerHTML,
      end: window.__k11t(li()).slice(-44), h: window.__k11t(document.querySelector('[data-str="info_h_coins"]')) });
    const out = Object.fromEntries(['ru', 'en', 'pl'].map(l => [l, window.__k11(l, one)]));
    out.now = one();
    return out;
  });
  z.check('«Инфо»: в строке про таблицу шансов вместо «{ic}» — значок кнопки шансов (на трёх языках); заголовок — «Монетки и кейсы — правила»',
    [...LANGS, 'now'].every(l => info[l].svg === 1 && !info[l].raw && info[l].btn) && info.ru.h === 'Монетки и кейсы — правила' && info.pl.h === 'Monetki i skrzynki — zasady', J(info));
  await p.evaluate(() => openInfo());
  await shot('li[data-str-html="info_li3"]', 'info');
  await p.evaluate(() => closeOverlay('infoOverlay'));

  await p.click('#st-cases');
  await until(() => curTab === 'cases' && !!document.querySelector('.case-i .open-hint'), null, 8000);
  const hint = () => p.evaluate(() => {
    const m = () => {
      const hs = [...document.querySelectorAll('.case-i .open-hint')], o = { n: hs.length, lines: 0, out: 0 };
      for (const e of hs) {
        const g = document.createRange(); g.selectNodeContents(e);
        if (new Set([...g.getClientRects()].filter(q => q.width > 0).map(q => Math.round(q.top))).size !== 1) o.lines++;
        const row = e.closest('.case-i'), a = e.getBoundingClientRect(), b = row.getBoundingClientRect();
        if (a.right > b.right + 0.5 || a.left < b.left - 0.5) o.out++;
      }
      if (hs.length) { o.ws = getComputedStyle(hs[0]).whiteSpace; o.t = hs[0].textContent; o.w = Math.round(hs[0].closest('.case-i').getBoundingClientRect().width); }
      return o;
    };
    const out = { ru: m() };
    for (const l of ['en', 'pl']) out[l] = window.__k11(l, () => { renderSide(); return m(); });
    renderSide();
    return out;
  });
  const H = {};
  for (const [wd, ht] of [[1920, 1080], [1366, 768]]) { await size(wd, ht); H[wd] = await hint(); }
  await size(1920, 1080);
  z.check(`«ОТКРЫТЬ ▸» (OPEN, OTWÓRZ) не переносится и не вылезает из строки кейса: 1920×1080 и 1366×768, три языка (кейсов с кнопкой — ${H[1920].ru.n})`,
    [1920, 1366].every(w => LANGS.every(l => { const m = H[w][l]; return m.n > 0 && m.ws === 'nowrap' && m.lines + m.out === 0; })), J(H));

  await p.click('#st-ach');
  await until(() => curTab === 'ach' && !!achievements && !!document.querySelector('#sideBody .muted'), null, 8000);
  const rs = await p.evaluate(() => {
    const keep = achievements.resets_at, out = { had: keep || null };
    const txt = () => { const e = [...document.querySelectorAll('#sideBody .muted')].find(x => x.textContent.includes('⏳')), s = window.__k11t(e) || ''; return e ? s.slice(s.indexOf('⏳')) : null; };
    const at = min => new Date(Date.now() + min * 60000 - 5000).toISOString();
    // кусок от «⏳» до конца строки стоит на одной строке (не рвётся посреди длительности)
    const one = () => {
      const e = [...document.querySelectorAll('#sideBody .muted')].find(x => x.textContent.includes('⏳')); if (!e) return null;
      const w = document.createTreeWalker(e, NodeFilter.SHOW_TEXT); let n;
      while ((n = w.nextNode())) {
        const i = n.data.indexOf('⏳'); if (i < 0) continue;
        const g = document.createRange(); g.setStart(n, i); g.setEnd(n, n.data.length);
        const tops = [...g.getClientRects()].filter(q => q.width > 0).map(q => q.top).sort((a, b) => a - b);
        return tops.length > 0 && tops[tops.length - 1] - tops[0] < 6;
      }
      return null;
    };
    out.one = {};
    try {
      for (const l of ['ru', 'en', 'pl']) out[l] = window.__k11(l, () => [16 * 60 + 50, 23, 60, 0.5].map(min => { achievements.resets_at = at(min); renderSide(); (out.one[l] = out.one[l] || []).push(one()); return txt(); }));
      achievements.resets_at = at(16 * 60 + 50); renderSide(); // для кадра
    } catch (e) { out.err = String(e); achievements.resets_at = keep; renderSide(); }
    window.__k11r = keep;
    return out;
  });
  await shot('#sideBody .muted:has-text("⏳")', 'reset');
  await p.evaluate(() => { achievements.resets_at = window.__k11r; renderSide(); });
  const RS = { ru: ['⏳ резет через 16 ч 50 мин', '⏳ резет через 23 мин', '⏳ резет через 1 ч 00 мин', '⏳ резет через 1 мин'],
    en: ['⏳ resets in 16 h 50 min', '⏳ resets in 23 min', '⏳ resets in 1 h 00 min', '⏳ resets in 1 min'],
    pl: ['⏳ reset za 16 h 50 min', '⏳ reset za 23 min', '⏳ reset za 1 h 00 min', '⏳ reset za 1 min'] };
  z.check('«Ачивки»: до резета — длительностью, а не как время на часах: «резет через 16 ч 50 мин», «резет через 23 мин» (EN и PL так же); надпись не рвётся посреди',
    LANGS.every(l => same(rs[l], RS[l]) && (rs.one[l] || []).length === 4 && rs.one[l].every(x => x === true)), J(rs));

  const tabs = await p.evaluate(() => ['now', 'cases', 'talents', 'ach', 'top', 'hist', 'food', 'book', 'pack', 'coins'].map(x => {
    const b = document.getElementById('st-' + x), s = b.querySelector('svg.isvg'), q = s ? s.getBoundingClientRect() : { width: 0, height: 0 };
    return { x, svg: b.querySelectorAll('svg.isvg').length, w: Math.round(q.width), h: Math.round(q.height), i: b.querySelectorAll('i').length,
      e: /\p{Extended_Pictographic}/u.test(b.textContent) };
  }));
  const food = tabs.find(t => t.x === 'food'), hist = tabs.find(t => t.x === 'hist');
  z.check('у вкладки «Кухня» значок из набора, того же размера, что у соседних; эмодзи во вкладках нет; значок есть у всех десяти',
    tabs.every(t => t.svg >= 1 && t.i === 0 && !t.e) && food.w === hist.w && food.h === hist.h && food.w > 0, J(tabs));
  await shot('.sidetabs', 'tabs');

  const fl = await p.evaluate(() => ({ names: [...document.querySelectorAll('.f-name')].map(e => window.__k11t(e)),
    grp: ['ru', 'en', 'pl'].map(l => window.__k11(l, () => grpName('Коммуникация'))) }));
  z.check('папка с Discord называется «Войсчат» (Voice chat, Czat głosowy); «Коммуникации» на экране нет',
    fl.names.includes('Войсчат') && !fl.names.includes('Коммуникация') && same(fl.grp, ['Войсчат', 'Voice chat', 'Czat głosowy']), J(fl));
  await shot('.folder:has-text("Войсчат")', 'folder');

  await p.waitForTimeout(1200);
  const fin = await p.evaluate(() => ({ lang: LANG, title: document.title === STR.page_title, coins: document.getElementById('coins').textContent.replace(/\D/g, ''), real: String(me.coins_balance) }));
  z.check('после проверки киоск снова по-русски, число монеток в шапке настоящее', fin.lang === 'ru' && fin.title && fin.coins === fin.real, J(fin));
  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
