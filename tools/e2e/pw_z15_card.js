// З15-и5 «Карточка сотрудника: периоды, телефон», браузер (№30; З-Р20). Бывшего сотрудника z15m (уволен 10 дней назад
// с причиной) владелец возвращает «Назначить» в «Персонале» — тост «z15m снова в команде — может входить в админку; новый
// период работы с сегодня, прежние — в карточке». Его снова увольняют; в «Уволенных» у z15m в «Работал» — оба периода по строке, в карточке —
// «Периоды работы» с причинами. Телефон «+48 500 000 012» сохраняется без пробелов, «600 000» — отказ «Телефон — в
// международном формате: + и 6–15 цифр». Телефон 360 — карточка без прокрутки вбок. z15m — временный сотрудник: его
// периоды удаляются в начале; денег не трогает.
const z = require('./_z');
(async () => {
  const M = 'z15m', R1 = 'z15 переезд', R2 = 'z15 учёба';
  const mid = await z.hireTemp(M);
  z.sql(`DELETE FROM staff_periods WHERE user_id = '${mid}'`);
  z.sql(`UPDATE staff_profiles SET phone = '', leave_on = NULL, leave_by = NULL, leave_reason = '' WHERE user_id = '${mid}'`);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const ru = iso => iso.slice(8, 10) + '.' + iso.slice(5, 7) + '.' + iso.slice(0, 4);
  const D20 = z.clubDay(-20), D10 = z.clubDay(-10), D0 = z.clubDay(0);
  const r1 = await z.api('PUT', `/admin/staff/${mid}/profile`, { hired_at: D20 }, 'boss');
  const r2 = await z.api('POST', `/admin/staff/${mid}/dismiss`, { date: D10, reason: R1 }, 'boss');
  z.check(`z15m работал ${D20} — ${D10} и уволен с причиной`, r1.status === 200 && r2.status === 200, `${r1.status} ${r2.status}`);

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'staff');
  await boss.fill('#stNick', M);
  await boss.click('button[onclick="promoteStaff()"]');
  await z.sleep(1500);
  const t1 = norm(await z.text(boss, '#toast'));
  z.check('«Назначить» бывшего сотрудника — тост «z15m снова в команде — может входить в админку; новый период работы с сегодня, '
    + 'прежние — в карточке»', t1 === `${M} снова в команде — может входить в админку; новый период работы с сегодня, прежние — в карточке`
    && z.sql(`SELECT role FROM users WHERE id = '${mid}'`) === 'admin', t1);
  const r3 = await z.api('POST', `/admin/staff/${mid}/dismiss`, { reason: R2 }, 'boss');

  const archive = async p => { await z.nav(p, 'staff'); await p.click('#s-staff button:has-text("Уволенные")'); await z.sleep(1500); };
  const cell = (p, nick, i) => p.evaluate(([n, k]) => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    return tr ? tr.cells[k].innerText.trim() : null; }, [nick, i]);
  const openCard = async (p, nick) => { await p.evaluate(n => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    const b = tr && [...tr.querySelectorAll('button')].find(x => x.textContent.trim() === 'Карточка'); if (b) b.click(); }, nick); await z.sleep(1200); };
  await archive(boss);
  const worked = await cell(boss, M, 2);
  z.check(`«Уволенные»: у z15m в «Работал» — оба периода по строке: «${ru(D20)} — ${ru(D10)}» и «${ru(D0)} — ${ru(D0)}»`,
    r3.status === 200 && worked === `${ru(D20)} — ${ru(D10)}\n${ru(D0)} — ${ru(D0)}`, JSON.stringify(worked));
  await openCard(boss, M);
  const periods = await boss.evaluate(() => { const e = document.getElementById('scPeriods'); return e ? e.innerText.trim() : null; });
  z.check('карточка: «Периоды работы» — с датами и причинами', periods === `${ru(D20)} — ${ru(D10)} · ${R1}\n${ru(D0)} — ${ru(D0)} · ${R2}`,
    JSON.stringify(periods));
  await z.shot(boss, 'z15_card_periods');
  await boss.fill('#sc_phone', '+48 500 000 012');
  await boss.click('#drawer button:has-text("Сохранить")');
  await z.sleep(1500);
  const t2 = norm(await z.text(boss, '#toast'));
  await archive(boss);
  await openCard(boss, M);
  const ph = await boss.evaluate(() => document.getElementById('sc_phone').value);
  z.check('телефон «+48 500 000 012» — тост «Карточка z15m сохранена», в карточке +48500000012', t2 === `Карточка ${M} сохранена`
    && ph === '+48500000012', JSON.stringify({ t2, ph }));
  await boss.fill('#sc_phone', '600 000');
  await boss.click('#drawer button:has-text("Сохранить")');
  await z.sleep(1500);
  const t3 = norm(await z.text(boss, '#toast'));
  z.check('телефон «600 000» — отказ «Телефон — в международном формате: + и 6–15 цифр», в базе прежний',
    t3 === 'Телефон — в международном формате: + и 6–15 цифр'
    && z.sql(`SELECT phone FROM staff_profiles WHERE user_id = '${mid}'`) === '+48500000012', t3);
  await boss.evaluate(() => closeDrawer());

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const phn = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await archive(phn);
  await openCard(phn, M);
  const fit = await phn.evaluate(() => { const b = document.getElementById('scPeriods'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(phn, 'z15_card_periods_phone');
  z.check('телефон 360: «Периоды работы» в карточке — без прокрутки вбок', fit.sw <= 360 && fit.box, JSON.stringify(fit));
  await phn.evaluate(() => closeDrawer());
  await z.done();
})().catch(z.crash);
