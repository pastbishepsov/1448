// З19-и17 «Страницы журнала без потерь», браузер (решение основателя 17.11, «Найдено по дороге» З19-и16). Журнал
// админки страницами по 100: чип «Сессии» (строки одного источника) — 100 строк и «Ещё 100», листая — все свои; у
// админа «Все» — полная страница, хотя среди последних действий есть владельческие (смены настроек — их админ не
// видит), и, листая «Ещё 100», каждая своя строка — по разу; владелец: «Сессии» — тоже «Ещё 100», во «Все» — и смены
// настроек. Строки проверки — SQL-ом с пометкой «z19aw»: 130 начал сессий раз в секунду и 10 смен настроек между ними
// (раз в 10 с); в начале и в конце удаляются. На HEAD у «Сессий» «Ещё 100» нет, у админа во «Все» на свежем стенде —
// 91 строка без «Ещё 100», а на стенде с продажами и пополнениями, листая, своих не хватает.
const z = require('./_z');
const N = 130;
(async () => {
  const ADM = z.uid('admin1');
  const wipe = () => z.sql(`DELETE FROM admin_actions WHERE details LIKE 'z19aw %'`);
  wipe(); z.atEnd(wipe);
  z.sql(`INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '${ADM}', 'session_start', 'z19aw #' || i, `
    + `now() - (i || ' seconds')::interval FROM generate_series(1, ${N}) i`);
  z.sql(`INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '${ADM}', 'settings_update', 'z19aw H' || j, `
    + `now() - ((10 * j - 5) || ' seconds')::interval FROM generate_series(1, 10) j`);

  // журнал на экране: строк в таблице, кнопка «Ещё 100», свои строки (по данным страницы, новее — раньше)
  const state = p => p.evaluate(() => {
    const own = audData.map(x => x.text || '').filter(t => t.startsWith('z19aw '));
    return { rows: document.querySelectorAll('#auditTable table tr').length - 1, more: !!document.getElementById('audMoreBtn'),
      mine: own.filter(t => t.startsWith('z19aw #')).map(t => +t.slice(7)), hidden: own.filter(t => t.startsWith('z19aw H')).length };
  });
  const brief = s => JSON.stringify({ ...s, mine: s.mine.length, first: s.mine.slice(0, 3), last: s.mine.slice(-3) });
  const chip = async (p, k) => { await p.click(`#s-grants .segs button[data-k="${k}"]`); await z.idle(p); };
  // «Ещё 100», пока кнопка есть и своих не все (не больше 4 раз)
  const more = async p => {
    for (let i = 0; i < 4; i++) {
      const s = await state(p);
      if (!s.more || s.mine.length >= N) return s;
      await p.click('#audMoreBtn'); await z.idle(p);
    }
    return state(p);
  };
  const all = s => s.mine.length === N && s.mine.every((v, i) => v === i + 1);

  // админ
  const A = await z.openAdmin('admin1');
  await z.nav(A, 'grants');
  await chip(A, 'sessions');
  const a1 = await state(A);
  z.check('админ, чип «Сессии» (строки одного источника): 100 строк и «Ещё 100» (на HEAD кнопки нет)',
    a1.rows === 100 && a1.more, brief(a1));
  const a2 = await more(A);
  z.check(`админ, «Сессии», листая «Ещё 100»: все ${N} своих начал сессий — по разу и по порядку`, all(a2), brief(a2));
  await chip(A, 'all');
  const a3 = await state(A);
  await z.shot(A, 'z19aw_admin_all');
  z.check('админ, «Все»: первая страница — 100 строк и «Ещё 100», хотя среди последних действий 10 смен настроек, которых '
    + 'админ не видит (на HEAD на свежем стенде — 91 строка без «Ещё 100»)', a3.rows === 100 && a3.more && a3.hidden === 0, brief(a3));
  const a4 = await more(A);
  z.check(`админ, «Все», листая «Ещё 100»: все ${N} своих — по разу и по порядку, смен настроек — ни одной`,
    all(a4) && a4.hidden === 0, brief(a4));

  // владелец
  const B = await z.openAdmin('boss');
  await z.nav(B, 'grants');
  await chip(B, 'sessions');
  const b1 = await state(B);
  z.check('владелец, «Сессии»: 100 строк и «Ещё 100» (на HEAD кнопки нет)', b1.rows === 100 && b1.more, brief(b1));
  const b2 = await more(B);
  z.check(`владелец, «Сессии», листая: все ${N} своих — по разу и по порядку`, all(b2), brief(b2));
  await chip(B, 'all');
  const b3 = await more(B);
  z.check(`владелец, «Все», листая: все ${N} своих и 10 смен настроек — их видит только владелец`,
    all(b3) && b3.hidden === 10, brief(b3));
  await z.done();
})().catch(z.crash);
