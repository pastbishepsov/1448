// К12-и4 «Ранги: шапка PWA не режет имя, последний ранг тайный» — проверка в браузере (Chrome без окна).
// Решения основателя 20.11 и 21.11: последний ранг скрыт «***», пока гость его не взял. Прячет СЕРВЕР: имя и его
// перевод (names: ru/en/pl) приходят только гостю, который ранг взял. Когда взял — имя открывается по буквам
// («***» → «Б***» → … → имя, около 2 с) в киоске и в PWA, один раз на устройство. Часы и бонусы ранга видны всегда.
// 1) в исходниках киоска и PWA трёх имён нет — ни в словарях, ни в RANK_KEYS;
// 2) гость Guest21 с 200 ч (ранг «Легенда», следующий — тайный): в ответе /me/economy трёх имён нет, следующий ранг и
//    седьмой в списке — «***» с secret, часы и бонусы на месте; киоск — «***» в строке «до …» и в «Все ранги и бонусы»;
//    PWA — «***» в строке «до …» карточки ранга, в странице и в её данных имён нет;
// 3) тот же гость с 400 ч: сервер прислал имя и перевод. Киоск — шапка открывает имя по буквам, кадр за кадром, флаг
//    устройства стоит, второй показ сразу; калькулятор и список — имя на трёх языках. PWA на ru/en/pl — то же в шапке
//    и в карточке ранга; перерисовка карточки и перезагрузка страницы — сразу имя, без «***»;
// 4) шапка PWA на 360 и 390 px: все 21 имя (7 рангов × 3 языка) видны целиком — и с монетами «999 999»; имя, ушедшее
//    на вторую строку, идёт без точки-разделителя в её начале.
// Часы — одна завершённая сессия гостя 30 дней назад (12 000 минут, потом 24 000): не «сегодня» — отчёты дня её не видят,
// и не старше 90 дней — иначе гость для сгорания монет «пропал», и работа сгорания (раз в 6 ч) срезала бы ему 10%.
// В конце сессия удаляется, язык гостя возвращается, монеты, опыт и ачивки гостя — те же, что до проверки.
// На 034ee68 (до К12-и4) падают: имена в исходниках, «***» в ответе сервера, открытие по буквам, шапка на 360 px.
const z = require('./_z');
const fs = require('fs'), path = require('path');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;
const G = 'Guest21', LANGS3 = ['ru', 'en', 'pl'];
const NAMES = { ru: 'Бессмертный', en: 'Immortal', pl: 'Nieśmiertelny' }, ALL3 = Object.values(NAMES);
const LEGEND = { ru: 'Легенда', en: 'Legend', pl: 'Legenda' };
const has3 = s => ALL3.filter(n => String(s).includes(n));
// кадры открытия, как их строит rankHead: «***», «Б***», «Бе***», …, «Бессмертны*», «Бессмертный»
const FRAMES = name => { const ch = Array.from(name), out = [];
  for (let i = 0; i <= ch.length; i++) out.push(ch.slice(0, i).join('') + '*'.repeat(Math.min(3, ch.length - i)));
  return out; };
// запись в странице: каждый новый текст шапки (#rankName) и карточки PWA (#rankCardName) — [мс, текст]
const REC = () => {
  window.__rk = { head: [], card: [] };
  const rec = () => { for (const [k, id] of [['head', 'rankName'], ['card', 'rankCardName']]) {
    const el = document.getElementById(id), a = window.__rk[k]; if (!el) continue;
    const t = el.textContent; if (!a.length || a[a.length - 1][1] !== t) a.push([Date.now(), t]); } };
  document.addEventListener('DOMContentLoaded', () => { rec();
    new MutationObserver(rec).observe(document.body, { subtree: true, childList: true, characterData: true }); });
};
// открытие по буквам: с первого «***» кадры ровно как у rankHead, на всё — около 2 с
const reveal = (a, name) => {
  const i = a.findIndex(x => x[1] === '***'), part = i < 0 ? [] : a.slice(i);
  return { ok: J(part.map(x => x[1])) === J(FRAMES(name)), ms: part.length ? part[part.length - 1][0] - part[0][0] : 0,
    got: part.map(x => x[1]) };
};
// шапка PWA: имя ранга видно целиком (не за правым краем, не обрезано внутри), на второй строке — без точки
const GEO = () => {
  const sub = document.querySelector('.tb-user .sub'), rk = sub.querySelector('.rk'), lv = sub.querySelector('.lv');
  const S = sub.getBoundingClientRect(), R = rk.getBoundingClientRect();
  const E = document.getElementById('rankEm').getBoundingClientRect(), N = document.getElementById('rankName').getBoundingClientRect();
  const wrapped = !!lv && R.top > lv.getBoundingClientRect().top + 2;
  return { ok: N.right <= S.right + 0.5 && E.left >= S.left - 0.5 && N.bottom <= S.bottom + 0.5 && rk.scrollWidth <= rk.clientWidth + 1,
    dot: !wrapped || R.left <= S.left - 9, wrapped };
};

(async () => {
  const g = z.uid(G);
  const snap = () => z.sql(`SELECT u.language || '|' || u.coins_balance || '|' || u.xp_current || '|' ||
    (SELECT count(*) FROM user_achievements a WHERE a.user_id = u.id) FROM users u WHERE u.id='${g}'`);
  const played = () => +z.sql(`SELECT COALESCE(SUM(minutes_used), 0) FROM sessions WHERE user_id='${g}' AND status='completed'`);
  const snap0 = snap(), lang0 = snap0.split('|')[0], min0 = played();
  if (min0 >= 200 * 60) z.crash(`у ${G} уже ${min0} минут — нужен гость меньше чем с 200 ч`);
  const day = z.clubDay(-30);
  const sid = z.sql(`INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total,
    minutes_used, base_rate_pln, effective_rate_pln, ended_reason)
    SELECT '${g}', id, club_id, 'completed', (('${day}'::date + time '12:00') AT TIME ZONE 'Europe/Warsaw'),
    (('${day}'::date + time '12:00') AT TIME ZONE 'Europe/Warsaw'), 0, 0, 0, 0, 'k12' FROM computers WHERE name='PC-08' RETURNING id`);
  z.atEnd(() => {
    z.sql(`DELETE FROM sessions WHERE id='${sid}'`);
    z.sql(`UPDATE users SET language='${lang0}' WHERE id='${g}'`);
    z.check(`гость после проверки тот же: часы, язык, монеты, опыт, ачивки (${snap0})`, played() === min0 && snap() === snap0, snap());
  });
  const hours = h => z.sql(`UPDATE sessions SET minutes_used=${h * 60 - min0}, minutes_total=${h * 60 - min0} WHERE id='${sid}'`);
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());

  console.log('— Исходники киоска и PWA: трёх имён нет');
  for (const f of ['shell.html', 'app.html']) {
    const src = fs.readFileSync(path.join(z.WEB_DIR, f), 'utf8');
    z.check(`${f}: нет «${ALL3.join('», «')}»`, has3(src).length === 0, J(has3(src)));
  }

  console.log('— 200 ч: ранг «Легенда», следующий тайный');
  hours(200);
  const me1 = (await z.api('GET', '/me', undefined, G)).data;
  const r2 = await z.api('GET', '/me/economy', undefined, G), d2 = r2.data || {};
  const nx = d2.next_rank || {}, s7 = (d2.ranks || [])[6] || {};
  z.check('API 200 ч: ранг 6 «Легенда», сыграно 200 ч', r2.status === 200 && !!d2.rank && d2.rank.level === 6
    && d2.rank.name === 'Легенда' && !d2.rank.secret && d2.hours_played === 200, J([r2.status, d2.rank, d2.hours_played]));
  z.check('API 200 ч: в ответе нет ни одного из трёх имён', has3(J(d2)).length === 0, J(has3(J(d2))));
  z.check('API 200 ч: следующий ранг — «***», secret, 400 ч, осталось 200, перевода нет',
    nx.name === '***' && nx.secret === true && nx.level === 7 && nx.min_hours === 400 && nx.hours_left === 200 && !nx.names, J(nx));
  z.check('API 200 ч: седьмой в списке — «***», secret; часы и бонусы видны', s7.name === '***' && s7.secret === true
    && s7.level === 7 && s7.min_hours === 400 && s7.coin_mult > 1 && s7.xp_mult > 1 && !s7.names, J(s7));
  z.check('API 200 ч: шестой в списке — «Легенда», не тайный', (d2.ranks || [])[5] && d2.ranks[5].name === 'Легенда'
    && !d2.ranks[5].secret, J((d2.ranks || [])[5]));

  // киоск без входа: me и economy — ответы сервера этому гостю, дальше — функции самого киоска
  const kctx = await br.newContext({ viewport: { width: 1280, height: 800 } });
  await kctx.addInitScript(REC);
  const k = await kctx.newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell', { waitUntil: 'load' });
  const kView = () => { const c = document.createElement('div'); c.innerHTML = renderCalc(); openRanks();
    return { calc: c.innerText.replace(/\s+/g, ' ').trim(), rn: (c.querySelector('.rn') || {}).textContent,
      list: document.getElementById('ranksList').innerText.replace(/\s+/g, ' ').trim(), head: document.getElementById('rankName').textContent }; };
  const K2 = await k.evaluate(([m, e, f]) => { me = m; economy = e; renderTop(); return (0, eval)('(' + f + ')')(); }, [me1, d2, kView.toString()]);
  const want2 = await k.evaluate(() => T('rank_next', { n: 200, r: '***' }).replace(/<[^>]+>/g, ''));
  z.check(`киоск 200 ч: шапка «Легенда», в калькуляторе «${want2}»`, K2.head === 'Легенда' && K2.calc.includes(want2), J([K2.head, K2.calc.slice(0, 160)]));
  z.check('киоск 200 ч: «Все ранги и бонусы» — седьмой «***», имён нет', K2.list.includes('***') && has3(K2.list + K2.calc).length === 0, K2.list);

  console.log('— PWA 200 ч (ru): «***» в строке «до …»');
  const pwa = async (l, w = 390) => {
    z.sql(`UPDATE users SET language='${l}' WHERE id='${g}'`);
    await z.sleep(2500); // лимитер стенда: вход PWA — залп запросов
    const ctx = await br.newContext({ viewport: { width: w, height: 844 }, isMobile: true, hasTouch: true });
    await ctx.addInitScript(v => { try { localStorage.setItem('1448_lang', v); } catch (e) {} }, l);
    await ctx.addInitScript(REC);
    const a = await ctx.newPage();
    a.on('pageerror', e => z.jsErrors.push(`PWA ${l}: ` + e.message));
    await a.goto(z.BASE + '/app', { waitUntil: 'load' });
    await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
    const ok = await a.waitForFunction(() => typeof me !== 'undefined' && me && me.id && typeof economy !== 'undefined'
      && economy && economy.rank && document.getElementById('rankName').textContent !== '—', null, { timeout: 20000 }).then(() => true, () => false);
    z.check(`PWA ${l.toUpperCase()}: гость вошёл, ранг пришёл с сервера`, ok, '');
    return { ctx, a, ok };
  };
  const card = async a => { await a.click('button[data-tab="progress"]');
    await a.waitForFunction(() => document.getElementById('rankCardName'), null, { timeout: 8000 }).catch(() => {}); };
  {
    const { ctx, a, ok } = await pwa('ru');
    if (ok) {
      await card(a); await z.sleep(400);
      const P = await a.evaluate(() => { const c = document.getElementById('rankCard'), nm = document.getElementById('rankCardName'),
        pt = c && c.querySelector('.hbar + .muted');
        return { head: document.getElementById('rankName').textContent, name: nm && nm.textContent, next: pt && pt.textContent.trim(),
          body: document.body.innerText, data: JSON.stringify(economy), rec: window.__rk.head.map(x => x[1]) }; });
      z.check('PWA 200 ч: шапка и карточка — «Легенда», без открытия по буквам', P.head === 'Легенда' && P.name === 'Легенда'
        && J(P.rec.filter(t => t !== '—')) === J(['Легенда']), J([P.head, P.name, P.rec]));
      z.check(`PWA 200 ч: строка «до …» — «${P.next}» с «***»`, !!P.next && P.next.includes('***'), J(P.next));
      z.check('PWA 200 ч: ни в странице, ни в её данных трёх имён нет', has3(P.body + P.data).length === 0, J(has3(P.body + P.data)));
    }
    await ctx.close();
  }

  console.log('— 400 ч: ранг взят — имя открывается по буквам');
  hours(400);
  const r4 = await z.api('GET', '/me/economy', undefined, G), d4 = r4.data || {}, t7 = (d4.ranks || [])[6] || {};
  const names3 = o => !!o && LANGS3.every(l => o[l] === NAMES[l]);
  z.check('API 400 ч: ранг 7 «Бессмертный» с переводом, без secret, дальше рангов нет', r4.status === 200 && !!d4.rank
    && d4.rank.level === 7 && d4.rank.name === NAMES.ru && names3(d4.rank.names) && !d4.rank.secret && !d4.next_rank && d4.hours_played === 400,
    J([r4.status, d4.rank, d4.next_rank]));
  z.check('API 400 ч: седьмой в списке — с именем и переводом', t7.name === NAMES.ru && names3(t7.names) && !t7.secret, J(t7));

  const K4 = await k.evaluate(e => { economy = e; renderTop(); }, d4).then(() => z.sleep(2600)).then(() => k.evaluate(id => ({
    rec: window.__rk.head, flag: localStorage.getItem('1448_rk_reveal_' + id + '_head'), head: document.getElementById('rankName').textContent }), me1.id));
  const kr = reveal(K4.rec, NAMES.ru);
  z.check(`киоск 400 ч: шапка открыла «${NAMES.ru}» по буквам — ${kr.got.length} кадров за ${kr.ms} мс`, kr.ok && kr.ms >= 1500 && kr.ms <= 3500
    && K4.head === NAMES.ru, J(kr.got));
  z.check('киоск 400 ч: флаг открытия на этом устройстве стоит', K4.flag === '1', J(K4.flag));
  const K4b = await k.evaluate(() => { const n = window.__rk.head.length; renderTop();
    return { same: window.__rk.head.length === n, head: document.getElementById('rankName').textContent }; });
  z.check('киоск 400 ч: второй показ шапки — сразу имя, без кадров', K4b.same && K4b.head === NAMES.ru, J(K4b));
  const KL = await k.evaluate(f => { const keep = { STR, LANG }, out = {}, view = (0, eval)('(' + f + ')');
    for (const l of ['ru', 'en', 'pl']) { LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]); out[l] = view(); }
    LANG = keep.LANG; STR = keep.STR; return out; }, kView.toString());
  for (const l of LANGS3) z.check(`киоск 400 ч ${l.toUpperCase()}: калькулятор и список — «${NAMES[l]}», без «***»`,
    KL[l].rn === NAMES[l] && KL[l].list.includes(NAMES[l]) && !KL[l].list.includes('***'), J([KL[l].rn, KL[l].list.slice(-120)]));
  await kctx.close();

  for (const l of LANGS3) {
    const L = l.toUpperCase(), name = NAMES[l];
    const { ctx, a, ok } = await pwa(l);
    if (!ok) { await ctx.close(); continue; }
    await a.waitForFunction(n => document.getElementById('rankName').textContent === n, name, { timeout: 8000 }).catch(() => {});
    await z.sleep(300);
    const H = await a.evaluate(() => ({ rec: window.__rk.head, flag: localStorage.getItem('1448_rk_reveal_' + me.id + '_head') }));
    const hr = reveal(H.rec, name);
    z.check(`PWA ${L} 400 ч: шапка открыла «${name}» по буквам — ${hr.got.length} кадров за ${hr.ms} мс, флаг стоит`,
      hr.ok && hr.ms >= 1500 && hr.ms <= 3500 && H.flag === '1', J([hr.got, H.flag]));
    const H2 = await a.evaluate(() => { const n = window.__rk.head.length; renderTop(); return window.__rk.head.length === n; });
    z.check(`PWA ${L} 400 ч: второй показ шапки — сразу имя`, H2, '');
    await card(a);
    await a.waitForFunction(n => (document.getElementById('rankCardName') || {}).textContent === n, name, { timeout: 8000 }).catch(() => {});
    await z.sleep(300);
    const C = await a.evaluate(() => ({ rec: window.__rk.card, flag: localStorage.getItem('1448_rk_reveal_' + me.id + '_card') }));
    const cr = reveal(C.rec, name);
    z.check(`PWA ${L} 400 ч: карточка ранга открыла «${name}» по буквам, флаг стоит`, cr.ok && C.flag === '1', J([cr.got, C.flag]));
    const C2 = await a.evaluate(async () => { const n = window.__rk.card.length; renderRank(); await new Promise(r => setTimeout(r, 300));
      return window.__rk.card.slice(n).map(x => x[1]); });
    z.check(`PWA ${L} 400 ч: перерисовка карточки — сразу имя, «***» не мелькает`, C2.every(t => t === name), J(C2));
    if (l === 'ru') {
      await z.sleep(2500); // лимитер: перезагрузка — тот же залп запросов
      await a.reload({ waitUntil: 'load' });
      if (await a.isVisible('#nick').catch(() => false)) { await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn'); }
      await a.waitForFunction(n => document.getElementById('rankName').textContent === n, name, { timeout: 20000 }).catch(() => {});
      await z.sleep(800);
      const RR = await a.evaluate(() => window.__rk.head.map(x => x[1]).filter(t => t !== '—'));
      z.check('PWA RU 400 ч: после перезагрузки — сразу имя, без «***»', J(RR) === J([name]), J(RR));
    }
    if (l === 'pl') {
      console.log('— Шапка PWA: 21 имя целиком на 360 и 390 px');
      for (const w of [360, 390]) {
        await a.setViewportSize({ width: w, height: 760 }); await z.sleep(300);
        const bad = await a.evaluate(([geo, sec]) => {
          const fit = (0, eval)('(' + geo + ')'), out = [], co = document.getElementById('coins'), coins0 = co.textContent;
          for (const coins of [coins0, '999 999']) for (const l of ['ru', 'en', 'pl']) for (let n = 1; n <= 7; n++) {
            const name = n < 7 ? Object.assign({}, STR_RU, LANGS[l])['rk_' + n] : sec[l];
            co.textContent = coins; document.getElementById('rankEm').textContent = RANK_EM[n];
            document.getElementById('rankName').textContent = name;
            const r = fit(); if (!r.ok || !r.dot) out.push(`${l} ${n} «${name}» монеты ${coins}${r.ok ? ' (точка в начале строки)' : ''}`);
          }
          co.textContent = coins0; renderTop();
          return out;
        }, [GEO.toString(), NAMES]);
        z.check(`шапка PWA ${w} px: все 21 имя видны целиком (монеты как есть и «999 999»)`, bad.length === 0, bad.join('; '));
      }
    }
    await ctx.close();
  }
  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
