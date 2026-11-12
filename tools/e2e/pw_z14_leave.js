// З14-и2 «Отложенное увольнение», браузер (№25 «а»; З-Р18). «Персонал» → «Уволить»: по умолчанию — сегодняшние клубные
// сутки, увольнение сейчас, как раньше. Дата позже — шторка, пока её вводят, говорит «Отложенное увольнение. До конца
// … он работает как обычно…», предупреждение о смене сейчас прячется (смену закроет завершение увольнения), внизу —
// со смен после этой даты снимется потом. «Уволить» — тост «уходит …», в списке у ника — «уходит 20.10» и кнопка
// «Отменить увольнение» (со взводом «Точно отменить?»); карточка — «Уходит …»; шторка при назначенном — «Уже
// назначено…». Журнал «Персонал» — «персонал: уходит» и «персонал: увольнение отменено». Последний день прошёл (SQL) —
// вход в админку выводит на экран входа: «Последний рабочий день прошёл — доступа в админку больше нет». Телефон 360 —
// без прокрутки вбок. Данные: временные z14j (входит в админку — смена открывается; назначают и отменяют) и z14k
// (последний день — вчера); оба уходят к концу проверки; денег не трогает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const J = 'z14j', K = 'z14k';
  const uj = await z.hireTemp(J), uk = await z.hireTemp(K);
  const reset = () => z.sql(`UPDATE staff_profiles SET leave_on = NULL, leave_by = NULL, leave_reason = '' WHERE user_id IN ('${uj}', '${uk}')`);
  reset(); // хвост упавшего прогона
  z.atEnd(reset);
  const r = await z.login(J, undefined, { client: 'admin' }); // вход в админку открывает смену
  const entry = z.sql(`SELECT id FROM work_entries WHERE user_id = '${uj}' AND ended_at IS NULL`);
  const TODAY = z.clubDay(0), D3 = z.clubDay(3), D5 = z.clubDay(5), YDAY = z.clubDay(-1);
  const ru = d => d.slice(8, 10) + '.' + d.slice(5, 7) + '.' + d.slice(0, 4), dm = d => d.slice(8, 10) + '.' + d.slice(5, 7);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const leaveOn = u => z.sql(`SELECT COALESCE(leave_on::text, '') || '|' || leave_reason FROM staff_profiles WHERE user_id = '${u}'`);
  if (!z.check('данные: у z14j открыта смена (вход в админку)', r.status === 200 && entry, JSON.stringify({ r: r.status, entry }))) return z.done();

  const boss = await z.openAdmin('boss');
  // строка ника в «Персонале» — по <b> с ником: рядом может стоять «уходит 20.10»
  const row = (p, nick) => p.evaluate(n => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    return tr ? { badge: norm0((tr.cells[0].querySelector('.st.pending') || {}).textContent), title: (tr.cells[0].querySelector('.st.pending') || {}).title || '',
      btns: [...tr.cells[tr.cells.length - 1].querySelectorAll('button')].map(b => norm0(b.textContent)) } : null;
    function norm0(s){ return String(s == null ? '' : s).replace(/\s+/g, ' ').trim(); } }, nick);
  const click = async (p, nick, label) => {
    await p.evaluate(([n, l]) => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
      const b = tr && [...tr.querySelectorAll('button')].find(x => x.textContent.replace(/\s+/g, ' ').trim() === l); if (b) b.click(); }, [nick, label]);
    await z.sleep(1200);
  };
  const staff = async p => { await z.nav(p, 'staff'); await p.waitForSelector('#staffTable tr', { timeout: 15000 }).catch(() => {}); };
  const drawer = p => p.evaluate(() => { const g = id => document.getElementById(id);
    return { date: (g('dm_date') || {}).value, when: g('dmWhen') && !g('dmWhen').classList.contains('hidden') ? g('dmWhen').textContent.replace(/\s+/g, ' ').trim() : '',
      shift: !!g('lvShift') && !g('lvShift').classList.contains('hidden'), note: ((g('dmNote') || {}).textContent || '').replace(/\s+/g, ' ').trim() }; });
  await staff(boss);
  await click(boss, J, 'Уволить');
  const d0 = await drawer(boss);
  z.check(`шторка «Уволить»: по умолчанию — сегодня (${ru(TODAY)}), увольнение сейчас: предупреждение о смене есть, «отложенного» нет`,
    d0.date === ru(TODAY) && d0.shift && d0.when === '' && d0.note.startsWith('Аккаунт останется гостевым'), JSON.stringify(d0));
  await boss.fill('#dm_date', ru(D3));
  await z.sleep(300);
  const d1 = await drawer(boss);
  z.check(`вводят ${ru(D3)} — «Отложенное увольнение. До конца ${ru(D3)} он работает как обычно…», предупреждение о смене сейчас спрятано, `
    + `внизу — «Со смен графика после ${ru(D3)} снимется, когда увольнение завершится.»`,
    d1.when.startsWith(`Отложенное увольнение. До конца ${ru(D3)} он работает как обычно — смена, касса, доступ. Со следующего дня доступа не будет`)
    && d1.when.endsWith(`До того в списке — «уходит ${dm(D3)}» и «Отменить увольнение».`) && !d1.shift
    && d1.note.endsWith(`Со смен графика после ${ru(D3)} снимется, когда увольнение завершится.`), JSON.stringify(d1));
  await z.shot(boss, 'z14_leave_drawer');
  await boss.fill('#dm_reason', 'z14 переезжает');
  const t1 = norm(await z.toastAfter(boss, () => boss.click('#drawer button.danger:has-text("Уволить")'), 'уходит')), r1 = await row(boss, J);
  z.check(`«Уволить» — тост «z14j уходит ${ru(D3)} — до конца дня работает, потом увольнение завершится само»; в списке у ника — «уходит ${dm(D3)}» `
    + 'и «Отменить увольнение», «Уволить» на месте; роль — admin, смена открыта',
    t1 === `${J} уходит ${ru(D3)} — до конца дня работает, потом увольнение завершится само` && r1 && r1.badge === `уходит ${dm(D3)}`
    && r1.btns.includes('Отменить увольнение') && r1.btns.includes('Уволить') && leaveOn(uj) === `${D3}|z14 переезжает`
    && z.sql(`SELECT role FROM users WHERE id = '${uj}'`) === 'admin' && z.sql(`SELECT ended_at IS NULL FROM work_entries WHERE id = '${entry}'`) === 't',
    JSON.stringify({ t1, r1, l: leaveOn(uj) }));
  await z.shot(boss, 'z14_leave_list');

  await click(boss, J, 'Карточка');
  const card = norm(await z.text(boss, '#drawer'));
  z.check(`карточка: «Уходит ${ru(D3)} — последний рабочий день»`, card.includes(`Уходит ${ru(D3)} — последний рабочий день; отменить — «Отменить увольнение» в списке`), card.slice(0, 300));
  await boss.evaluate(() => closeDrawer());
  await click(boss, J, 'Уволить');
  const d2 = await drawer(boss);
  await boss.fill('#dm_date', ru(D5));
  await z.sleep(300);
  const d3 = await drawer(boss);
  z.check(`шторка при назначенном: «Уже назначено: уходит ${ru(D3)} — «Уволить» с этой датой уволит сейчас», с датой позже — «новая дата заменит»`,
    d2.when === `Уже назначено: уходит ${ru(D3)} — «Уволить» с этой датой уволит сейчас.` && d2.shift
    && d3.when.startsWith(`Уже назначено: уходит ${ru(D3)} — новая дата заменит. Отложенное увольнение. До конца ${ru(D5)}`), JSON.stringify({ d2, d3 }));
  await boss.evaluate(() => closeDrawer());

  await click(boss, J, 'Отменить увольнение');
  const armed = (await row(boss, J) || { btns: [] }).btns;
  const t2 = norm(await z.toastAfter(boss, () => click(boss, J, 'Точно отменить?'), 'отменено')), r2 = await row(boss, J);
  z.check(`«Отменить увольнение» → «Точно отменить?» → тост «Увольнение z14j (${ru(D3)}) отменено — остаётся в команде»; у ника — ни «уходит», ни кнопки`,
    armed.includes('Точно отменить?') && t2 === `Увольнение ${J} (${ru(D3)}) отменено — остаётся в команде` && r2 && r2.badge === ''
    && !r2.btns.includes('Отменить увольнение') && leaveOn(uj) === '|', JSON.stringify({ armed, t2, r2, l: leaveOn(uj) }));

  await z.nav(boss, 'grants');
  await boss.click('#s-grants .segs button[data-k="staff"]');
  await z.sleep(1800);
  const au = await boss.evaluate(n => [...document.querySelectorAll('#auditTable tr')].filter(x => x.cells[2] && x.cells[2].innerText.trim() === n)
    .map(x => x.cells[1].innerText.trim() + ' | ' + x.cells[3].innerText.trim()), J);
  z.check('журнал «Персонал»: «персонал: увольнение отменено» и «персонал: уходит … · z14 переезжает»',
    au[0] === `персонал: увольнение отменено | ${J} · увольнение ${D3} отменено` && au[1] === `персонал: уходит | ${J} · уходит ${D3} · z14 переезжает`, JSON.stringify(au.slice(0, 3)));

  // последний день z14k прошёл: вход в админку — экран входа с причиной (фон, раз в 10 минут, мог успеть раньше — тогда роль уже снята)
  z.sql(`UPDATE staff_profiles SET leave_on = '${YDAY}', leave_by = (SELECT id FROM users WHERE nickname = 'boss') WHERE user_id = '${uk}'`);
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const kp = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  kp.on('pageerror', e => z.jsErrors.push(`${K}: ${e.message}`));
  await kp.goto(z.BASE + '/admin', { waitUntil: 'load' });
  await kp.fill('#nick', K); await kp.fill('#pass', z.STAFF_PASS);
  await kp.keyboard.press('Enter');
  await z.sleep(3500);
  const le = await z.text(kp, '#loginErr'), inApp = await kp.evaluate(() => !document.getElementById('appView').classList.contains('hidden'));
  const raced = z.sql(`SELECT role FROM users WHERE id = '${uk}'`) === 'player';
  z.check('z14k, последний день — вчера: вход — экран входа «Последний рабочий день прошёл — доступа в админку больше нет. Если это ошибка, напиши владельцу.»',
    !inApp && (le === 'Последний рабочий день прошёл — доступа в админку больше нет. Если это ошибка, напиши владельцу.'
      || raced && le === 'Доступ к админке закрыт — роль сотрудника снята. Если это ошибка, напиши владельцу.'), JSON.stringify({ le, inApp, raced }));
  await z.shot(kp, 'z14_leave_login');

  await z.api('POST', `/admin/staff/${uj}/dismiss`, { date: D3 }, 'boss');
  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await staff(ph);
  const rp = await row(ph, J), sw0 = await ph.evaluate(() => document.documentElement.scrollWidth);
  const inside = await ph.evaluate(n => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    return !!tr && [...tr.querySelectorAll('button')].every(b => { const q = b.getBoundingClientRect(); return q.left >= 0 && q.right <= innerWidth; }); }, J);
  await z.shot(ph, 'z14_leave_phone_list');
  await click(ph, J, 'Уволить');
  await ph.fill('#dm_date', ru(D5));
  await z.sleep(300);
  const fit = await ph.evaluate(() => { const b = document.getElementById('dmWhen'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(ph, 'z14_leave_phone');
  z.check('телефон 360: «уходит» в списке, три кнопки строки — в экране (переносятся), шторка с отложенным увольнением — без прокрутки вбок',
    rp && rp.badge === `уходит ${dm(D3)}` && inside && sw0 <= 360 && fit.sw <= 360 && fit.box, JSON.stringify({ rp, inside, sw0, fit }));
  await z.done();
})().catch(z.crash);
