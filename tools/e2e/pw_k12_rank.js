// К12-и4 «Ранги: шапка PWA не режет имя, последний ранг тайный» и К12-и5 «Тайный ранг: открытие по буквам один раз
// на гостя» — проверка в браузере (Chrome без окна).
// Решения основателя 20.11 и 21.11: последний ранг скрыт «***», пока гость его не взял. Прячет СЕРВЕР: имя и его
// перевод (names: ru/en/pl) приходят только гостю, который ранг взял. Когда взял — имя открывается по буквам
// («***» → «Б***» → … → имя, около 2 с) один раз на гостя: раз в киоске (на любом ПК) и раз в PWA (на любом телефоне).
// Помнит сервер: отметки users.rank_reveal_kiosk_at / rank_reveal_app_at (миграция 105); /me/economy?client=kiosk|app
// даёт rank.reveal, пока отметки этого клиента нет, ставит её POST /me/rank-reveal?client=… Часы и бонусы видны всегда.
// 1) в исходниках киоска и PWA трёх имён нет — ни в словарях, ни в RANK_KEYS;
// 2) гость Guest21 с 200 ч (ранг «Легенда», следующий — тайный): в ответе /me/economy трёх имён нет, следующий ранг и
//    седьмой в списке — «***» с secret, часы и бонусы на месте; reveal нет, отметку не поставить (ok: false); киоск —
//    «***» в строке «до …» и в «Все ранги и бонусы»; PWA — «***» в строке «до …» карточки, в странице и данных имён нет;
// 3) тот же гость с 400 ч: сервер прислал имя и перевод; reveal — только с client=kiosk и app, чужой client — 400.
//    Киоск ПК1 (вход токеном, экран оболочки и свой loadAll): без сессии (лок-экран), на «Готов?» и на смене пароля —
//    «***» без отметки; с сессией (подставлена перехватом /me/sessions) — шапка открывает имя по буквам, кадр за
//    кадром; отметка киоска на сервере стоит, PWA — нет; второй показ и новая загрузка — сразу имя; ПК2 (другой браузер) — сразу имя;
//    калькулятор и список — имя на трёх языках. PWA на смене пароля — «***» без отметки.
//    PWA RU (телефон 1) — открытие в шапке и в карточке ранга в тот же
//    заход, отметка PWA стоит; перерисовка карточки и перезагрузка — сразу имя. PWA EN (телефон 2) — сразу имя в шапке
//    и в карточке. PWA PL — отметку PWA сбросили (как у гостя, который ещё не видел): открытие на польском;
// 4) шапка PWA на 360 и 390 px: все 21 имя (7 рангов × 3 языка) видны целиком — и с монетами «999 999»; имя, ушедшее
//    на вторую строку, идёт без точки-разделителя в её начале.
// Часы — одна завершённая сессия гостя 30 дней назад (12 000 минут, потом 24 000): не «сегодня» — отчёты дня её не видят,
// и не старше 90 дней — иначе гость для сгорания монет «пропал», и работа сгорания (раз в 6 ч) срезала бы ему 10%.
// В конце сессия удаляется, язык гостя возвращается, отметки открытия пустые; монеты, опыт, ачивки — те же, что до проверки.
// На 034ee68 (до К12-и4) падают: имена в исходниках, «***» в ответе сервера, открытие по буквам, шапка на 360 px.
// На c0a0106 (до К12-и5) падают: reveal и ручка отметки, отметки на сервере, повторное открытие на ПК2 и телефоне 2, открытие под
// лок-экраном, «Готов?» и экраном смены пароля.
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
    (SELECT count(*) FROM user_achievements a WHERE a.user_id = u.id) || '|' || COALESCE(u.rank_reveal_kiosk_at::text, '-')
    || '|' || COALESCE(u.rank_reveal_app_at::text, '-') FROM users u WHERE u.id='${g}'`);
  // К12-и5: отметки «видел открытие» [киоск, PWA]; у гостя меньше чем с 200 ч их нет — остаться могли бы от упавшего прогона
  const marks0 = () => z.sql(`UPDATE users SET rank_reveal_kiosk_at = NULL, rank_reveal_app_at = NULL WHERE id='${g}'`);
  const marks = () => z.sql(`SELECT (rank_reveal_kiosk_at IS NOT NULL)::text || '|' || (rank_reveal_app_at IS NOT NULL)::text
    FROM users WHERE id='${g}'`).split('|').map(x => x.trim() === 'true');
  marks0();
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
    marks0();
    z.check(`гость после проверки тот же: часы, язык, монеты, опыт, ачивки, отметки открытия (${snap0})`, played() === min0 && snap() === snap0, snap());
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
  const rkOf = async cl => ((await z.api('GET', '/me/economy?client=' + cl, undefined, G)).data || {}).rank || {};
  const k2r = await rkOf('kiosk'), a2r = await rkOf('app'), p2 = await z.api('POST', '/me/rank-reveal?client=kiosk', undefined, G);
  z.check('API 200 ч: reveal нет ни в киоске, ни в PWA; POST отметки — ok: false, отметок нет', k2r.level === 6 && !k2r.reveal
    && a2r.level === 6 && !a2r.reveal && p2.status === 200 && !!p2.data && p2.data.ok === false && J(marks()) === J([false, false]),
    J([k2r.reveal, a2r.reveal, p2.status, p2.data, marks()]));

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
  const pwa = async (l, w = 390, pre = null) => {
    z.sql(`UPDATE users SET language='${l}' WHERE id='${g}'`);
    await z.sleep(2500); // лимитер стенда: вход PWA — залп запросов
    const ctx = await br.newContext({ viewport: { width: w, height: 844 }, isMobile: true, hasTouch: true });
    await ctx.addInitScript(v => { try { localStorage.setItem('1448_lang', v); } catch (e) {} }, l);
    await ctx.addInitScript(REC);
    const a = await ctx.newPage();
    a.on('pageerror', e => z.jsErrors.push(`PWA ${l}: ` + e.message));
    if (pre) await pre(a);
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
  const k4r = await rkOf('kiosk'), a4r = await rkOf('app');
  z.check('API 400 ч: без client reveal нет; с client=kiosk и client=app — reveal: true, открытие впереди',
    !!d4.rank && !('reveal' in d4.rank) && k4r.reveal === true && a4r.reveal === true, J([d4.rank && d4.rank.reveal, k4r.reveal, a4r.reveal]));
  const pX = await z.api('POST', '/me/rank-reveal?client=pc', undefined, G), pN = await z.api('POST', '/me/rank-reveal', undefined, G);
  z.check('API 400 ч: POST отметки с чужим client или без него — 400, отметок нет', pX.status === 400 && pN.status === 400
    && J(marks()) === J([false, false]), J([pX.status, pN.status, marks()]));

  // К12-и5: в киоске шапку видно, только пока идёт сессия этого ПК — без неё лок-экран; ещё её закрывают «Готов?» и
  // смена пароля. Там — «***» и без отметки. Сессию и флаг пароля подставляет перехват в странице (/me/sessions, /me):
  // настоящая сессия — биллинг раз в 30 с — тронула бы монеты гостя и снимок «гость тот же».
  const tk = await z.tok(G), PC8 = z.sql(`SELECT id FROM computers WHERE name='PC-08'`).trim();
  const FAKE = ready => { const t = new Date(Date.now() - 600e3).toISOString();
    return { id: 'k125-fake', user_id: g, computer_id: PC8, status: 'active', started_at: t, ready_at: ready ? t : null,
      ready_deadline: ready ? null : new Date(Date.now() + 120e3).toISOString(), ended_at: null, paused_at: null,
      paused_total_sec: 0, billed_minutes: 10, money_minutes: 10, zero_since: null, base_rate_pln: 10,
      effective_rate_pln: 10, charged_grosz: 0, minutes_total: 0, minutes_used: 0, zone_id: null }; };
  const fake = (page, st) => page.route(u => /\/me(\/sessions)?$/.test(u.pathname), async r => {
    const res = await r.fetch(), j = await res.json();
    if (r.request().url().includes('/me/sessions')) { if (st.sess) j.sessions = [st.sess]; }
    else if (st.pwd) (j.user || j).must_change_password = true;
    await r.fulfill({ response: res, json: j });
  });
  const KS = {};
  await fake(k, KS);
  const kStep = async (st, enter) => { Object.assign(KS, st);
    await z.sleep(2500); // лимитер стенда: loadAll — залп запросов
    await k.evaluate(async ([t, enter]) => { window.__rk.mark = window.__rk.head.length;
      document.getElementById('rankName').textContent = ''; // первый кадр «***» — со своим временем
      if (enter) { setTokens(t, ''); // экран оболочки — как enterShell
        document.getElementById('authView').classList.add('hidden'); document.getElementById('shellView').classList.remove('hidden'); }
      await loadAll(); }, [tk, !!enter]);
    await z.sleep(2600); // открытие (если есть) доигрывает, отметка уходит на сервер
    return k.evaluate(() => ({ rec: window.__rk.head.slice(window.__rk.mark), head: document.getElementById('rankName').textContent,
      lock: document.getElementById('lockOverlay').classList.contains('show') }));
  };
  const hid = (S, lock) => S.head === '***' && S.lock === lock && !S.rec.some(x => x[1] !== '***' && x[1].includes('*'))
    && J(marks()) === J([false, false]);
  const Klk = await kStep({}, true);
  z.check('киоск ПК1 400 ч без сессии (лок-экран): в шапке «***», открытия нет, отметки нет', hid(Klk, true),
    J([Klk.head, Klk.lock, Klk.rec.map(x => x[1]), marks()]));
  const Krd = await kStep({ sess: FAKE(false) });
  z.check('киоск ПК1 400 ч, сессия ждёт «Готов?»: в шапке «***», отметки нет', hid(Krd, false), J([Krd.head, Krd.rec.map(x => x[1]), marks()]));
  const Kpw = await kStep({ sess: FAKE(true), pwd: true });
  z.check('киоск ПК1 400 ч, экран смены пароля: в шапке «***», отметки нет', hid(Kpw, false), J([Kpw.head, Kpw.rec.map(x => x[1]), marks()]));
  const K4 = await kStep({ pwd: false });
  const kr = reveal(K4.rec, NAMES.ru), mK = marks();
  z.check(`киоск ПК1 400 ч: шапка открыла «${NAMES.ru}» по буквам — ${kr.got.length} кадров за ${kr.ms} мс`, kr.ok && kr.ms >= 1500 && kr.ms <= 3500
    && K4.head === NAMES.ru, J(kr.got));
  z.check('киоск ПК1 400 ч: отметка киоска на сервере стоит, PWA — нет', J(mK) === J([true, false]), J(mK));
  await z.sleep(2500);
  const K4b = await k.evaluate(async () => { const n = window.__rk.head.length; renderTop(); await loadAll();
    await new Promise(r => setTimeout(r, 400)); return { same: window.__rk.head.length === n, head: document.getElementById('rankName').textContent }; });
  z.check('киоск ПК1 400 ч: второй показ шапки и новая загрузка — сразу имя, без кадров', K4b.same && K4b.head === NAMES.ru, J(K4b));
  const k4b = await rkOf('kiosk'), a4b = await rkOf('app');
  z.check('API после киоска: client=kiosk — reveal нет, client=app — reveal: true (отметка киоска PWA не гасит)',
    k4b.level === 7 && !k4b.reveal && a4b.reveal === true, J([k4b.reveal, a4b.reveal]));
  const KL = await k.evaluate(f => { const keep = { STR, LANG }, out = {}, view = (0, eval)('(' + f + ')');
    for (const l of ['ru', 'en', 'pl']) { LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]); out[l] = view(); }
    LANG = keep.LANG; STR = keep.STR; return out; }, kView.toString());
  for (const l of LANGS3) z.check(`киоск 400 ч ${l.toUpperCase()}: калькулятор и список — «${NAMES[l]}», без «***»`,
    KL[l].rn === NAMES[l] && KL[l].list.includes(NAMES[l]) && !KL[l].list.includes('***'), J([KL[l].rn, KL[l].list.slice(-120)]));
  await kctx.close();
  await z.sleep(2500);
  const kctx2 = await br.newContext({ viewport: { width: 1280, height: 800 } });
  await kctx2.addInitScript(REC);
  const k2 = await kctx2.newPage();
  k2.on('pageerror', e => z.jsErrors.push('киоск ПК2: ' + e.message));
  await k2.goto(z.BASE + '/shell', { waitUntil: 'load' });
  const KP2 = await k2.evaluate(async t => { setTokens(t, ''); await loadAll(); await new Promise(r => setTimeout(r, 2600));
    return { rec: window.__rk.head.map(x => x[1]), head: document.getElementById('rankName').textContent }; }, tk);
  z.check(`киоск ПК2 400 ч (другой браузер): сразу «${NAMES.ru}», без «***» и кадров`, KP2.head === NAMES.ru
    && !KP2.rec.some(t => t.includes('*')), J(KP2.rec));
  await kctx2.close();

  // К12-и5: PWA под экраном смены пароля (временный пароль) шапку не видно — «***», отметки PWA нет
  { const { ctx, a, ok } = await pwa('ru', 390, p => fake(p, { pwd: true }));
    if (ok) { await z.sleep(2600);
      const P = await a.evaluate(() => ({ head: document.getElementById('rankName').textContent,
        gate: !document.getElementById('pwdGate').classList.contains('hidden'), rec: window.__rk.head.map(x => x[1]) }));
      z.check('PWA RU 400 ч, экран смены пароля: в шапке «***», открытия нет, отметки PWA нет', P.head === '***' && P.gate
        && !P.rec.some(t => t !== '***' && t.includes('*')) && !marks()[1], J([P, marks()])); }
    await ctx.close(); }
  for (const l of LANGS3) {
    const L = l.toUpperCase(), name = NAMES[l], first = l !== 'en'; // RU — телефон 1, EN — телефон 2, PL — отметку сбросили
    if (l === 'pl') z.sql(`UPDATE users SET rank_reveal_app_at = NULL WHERE id='${g}'`); // как у гостя, который ещё не видел
    const { ctx, a, ok } = await pwa(l);
    if (!ok) { await ctx.close(); continue; }
    await a.waitForFunction(n => document.getElementById('rankName').textContent === n, name, { timeout: 8000 }).catch(() => {});
    await z.sleep(300);
    const H = await a.evaluate(() => window.__rk.head), hr = reveal(H, name), mA = marks();
    if (first) z.check(`PWA ${L} 400 ч: шапка открыла «${name}» по буквам — ${hr.got.length} кадров за ${hr.ms} мс, отметка PWA на сервере стоит`,
      hr.ok && hr.ms >= 1500 && hr.ms <= 3500 && mA[1], J([hr.got, mA]));
    else z.check(`PWA ${L} 400 ч, другой телефон: шапка — сразу «${name}», без «***»`, H.length > 0 && H[H.length - 1][1] === name
      && !H.some(x => x[1].includes('*')), J(H.map(x => x[1])));
    const H2 = await a.evaluate(() => { const n = window.__rk.head.length; renderTop(); return window.__rk.head.length === n; });
    z.check(`PWA ${L} 400 ч: второй показ шапки — сразу имя`, H2, '');
    await card(a);
    await a.waitForFunction(n => (document.getElementById('rankCardName') || {}).textContent === n, name, { timeout: 8000 }).catch(() => {});
    await z.sleep(300);
    const C = await a.evaluate(() => window.__rk.card), cr = reveal(C, name);
    if (first) z.check(`PWA ${L} 400 ч: карточка ранга в тот же заход открыла «${name}» по буквам`, cr.ok, J(cr.got));
    else z.check(`PWA ${L} 400 ч, другой телефон: карточка — сразу «${name}», без «***»`, C.length > 0 && C[C.length - 1][1] === name
      && !C.some(x => x[1].includes('*')), J(C.map(x => x[1])));
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
