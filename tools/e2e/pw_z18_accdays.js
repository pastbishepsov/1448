// З18-и4д «Даты «Бухгалтерии» по клубным суткам» (З-Р38, пункт 4; решение основателя 07.11), браузер. Своя граница
// суток — на час-два позже «сейчас» (как ночью до 08:00 у клуба с границей 8: клубные сутки ещё вчерашние), браузер — в
// поясе UTC+14 (его календарь — не клубный). «Новый расход» — дата клубных суток; «Из кассы» — дата формы совпадает с
// днём кассы (вопрос «взяли до сдачи» не теряется); внесённый — в «Расходах» за месяц с этой датой. «Ждут внесения» —
// шаблон на число клубных суток: «срок сегодня», «Внести» — эта дата. «Справочники» — шаблон на число следующих суток:
// «дальше …» и «Внести заранее» с датой клубных суток. В 23:xx своей границей сутки не сдвинуть — расходится только
// календарь браузера; в последние полторы минуты 22:xx проверка ждёт 23:00. Шаблоны «z18dp Аренда» и «z18dp Связь»
// заводятся каждый прогон и удаляются SQL вместе с напоминаниями о них, расходы с заметкой «z18dp проверка»
// сторнируются, граница суток — как была.
const z = require('./_z');
(async () => {
  const T1 = 'z18dp Аренда', T2 = 'z18dp Связь', MARK = 'z18dp проверка', TZB = 'Pacific/Kiritimati';
  let [H, LEFT] = z.sql("SELECT extract(hour FROM now() AT TIME ZONE 'Europe/Warsaw')::int || ' ' || "
    + "(3600 - extract(epoch FROM now() - date_trunc('hour', now())))::int").split(' ').map(Number);
  if (H === 22 && LEFT < 90) { await z.sleep((LEFT + 2) * 1000); H = 23; } // граница 23 вот-вот наступит — ждём её
  const RH = H <= 21 ? H + 2 : H === 22 ? 23 : null; // граница позже «сейчас» — сутки ещё вчерашние; в 23:xx — не сдвинуть
  const cleanup = async () => {
    for (const id of z.rows(`SELECT id FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
    z.sql(`DELETE FROM expense_templates WHERE name IN ('${T1}', '${T2}')`);
    z.sql("DELETE FROM staff_messages WHERE text LIKE '%«z18dp %'");
  };
  await cleanup(); // хвост упавшего прогона
  z.atEnd(cleanup);
  if (RH !== null) {
    const rh0 = z.rows("SELECT value FROM settings WHERE key = 'report_hour'")[0];
    z.atEnd(() => { z.sql("DELETE FROM settings WHERE key = 'report_hour'");
      if (rh0 !== undefined) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${rh0}', now())`); });
    z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${RH}', now()) ON CONFLICT (key) DO UPDATE SET value = '${RH}'`);
  }
  const CD = z.clubDay(0), NX = z.clubDay(1), CAL = z.sql("SELECT (now() AT TIME ZONE 'Europe/Warsaw')::date::text");
  const ru = d => d.slice(8) + '.' + d.slice(5, 7) + '.' + d.slice(0, 4);
  const cats = (await z.api('GET', '/admin/expense-categories', undefined, 'boss')).data.categories || [];
  const other = cats.find(c => c.code === 'other') || {}, rent = cats.find(c => c.code === 'rent') || {};
  const mk = async (name, day, cat, amount) => (((await z.api('POST', '/admin/expense-templates', { name, category_id: cat, amount_pln: amount,
    method: 'bank', day, note: MARK }, 'boss')).data || {}).template || {});
  const a = await mk(T1, +CD.slice(8), rent.id, 1800), b = await mk(T2, +NX.slice(8), other.id, 70);

  const boss = await z.openAdmin('boss', { timezoneId: TZB });
  const toast = async (ms = 2000) => { await z.sleep(ms); return (await z.text(boss, '#toast')) || ''; };
  const form = () => boss.evaluate(() => ({ title: (document.getElementById('drawerTitle') || {}).textContent,
    date: (document.getElementById('efDate') || {}).value || null }));
  const BD = await boss.evaluate(() => isoDate(new Date()));
  z.check(RH !== null ? `своя граница ${RH}:00, сейчас ${H}:xx — клубные сутки ${CD}, календарь клуба ${CAL}; браузер в ${TZB} — ${BD}`
    : `${H}:xx — своей границей сутки не сдвинуть (${CD} = календарь клуба); браузер в ${TZB} — ${BD}`,
    BD !== CD && (RH === null || CD !== CAL) && !!a.id && !!b.id, JSON.stringify({ H, RH, CD, CAL, BD, a: a.id, b: b.id }));

  // «Расходы» → «Новый расход»
  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  await boss.click('#expNewBtn'); await z.sleep(900);
  const f1 = await form();
  z.check(`«Новый расход»: дата — клубные сутки ${ru(CD)}, а не календарь браузера ${ru(BD)}`, f1.title === 'Новый расход' && f1.date === ru(CD), JSON.stringify(f1));
  await boss.click('#efMethod button:has-text("Из кассы")'); await z.sleep(1500);
  const till = await boss.evaluate(() => ({ cash: (expTodayCos || {}).date, form: dateIso(document.getElementById('efDate')),
    cos: ((expTodayCos || {}).list || []).length, ask: !!document.getElementById('efBefore') }));
  z.check(`«Из кассы»: дата формы — день кассы ${CD}; вопрос «взяли до сдачи» — ${till.cos ? 'есть' : 'нет, пересчётов за сутки нет'} (пересчётов — ${till.cos})`,
    till.cash === CD && till.form === CD && till.ask === (till.cos > 0), JSON.stringify(till));
  await z.shot(boss, 'z18_accdays_new');
  await boss.click('#efMethod button:has-text("Банк")');
  await boss.selectOption('#efCat', other.id); await boss.fill('#efAmount', '7'); await boss.fill('#efNote', MARK + ': новый');
  await boss.click('#efSave');
  const t1 = await toast(2500);
  const row = await boss.evaluate(n => { const r = [...document.querySelectorAll('#accBody tr.expRow')].find(x => x.innerText.includes(n));
    return r ? { id: r.dataset.id, cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) } : null; }, MARK + ': новый');
  const db = row ? z.sql(`SELECT op_date FROM expenses WHERE id = '${row.id}'`) : '';
  z.check(`«Внести расход» → тост; строка в «Расходах» за месяц — ${ru(CD)} (в базе ${CD})`,
    t1.includes('Расход 7,00 zł внесён — Прочее') && !!row && row.cells[0] === ru(CD) && db === CD, JSON.stringify({ t1, row, db }));

  // «Ждут внесения» → «Внести»
  const d1 = await boss.evaluate(i => { const r = document.querySelector(`#expDue tr.dueRow[data-t="${i}"]`);
    return r ? [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) : null; }, a.id);
  z.check(`«Ждут внесения»: «${T1}» на ${+CD.slice(8)}-е — «срок сегодня», «Внести»`, !!d1 && d1[4] === 'срок сегодня' && d1[5] === 'Внести', JSON.stringify(d1));
  let f2 = {};
  if (d1) { await boss.click(`#expDue tr.dueRow[data-t="${a.id}"] button`); await z.sleep(900); f2 = await form(); }
  z.check(`«Внести» → «Расход по шаблону», дата — срок, клубные сутки ${ru(CD)}`, f2.title === 'Расход по шаблону' && f2.date === ru(CD), JSON.stringify(f2));
  await boss.evaluate(() => closeDrawer()); await z.sleep(400);

  // «Справочники» → «Регулярные расходы» → «Внести заранее»
  await boss.click('#accTabs button[data-t="refs"]'); await z.sleep(1500);
  const r2 = await boss.evaluate(i => { const r = document.querySelector(`#refTpls tr.refRow[data-id="${i}"]`);
    return r ? r.cells[5].innerText.replace(/\s+/g, ' ').trim() : null; }, b.id);
  z.check(`«Справочники»: «${T2}» на ${+NX.slice(8)}-е — «дальше ${ru(NX)}» и «Внести заранее»`, r2 === `дальше ${ru(NX)} Внести заранее`, r2);
  let f3 = {};
  if (r2 && r2.includes('Внести заранее')) {
    await boss.click(`#refTpls tr.refRow[data-id="${b.id}"] button:has-text("Внести заранее")`); await z.sleep(900); f3 = await form();
  }
  z.check(`«Внести заранее» → «Расход по шаблону», дата — клубные сутки ${ru(CD)}`, f3.title === 'Расход по шаблону' && f3.date === ru(CD), JSON.stringify(f3));
  await z.shot(boss, 'z18_accdays_early');
  await boss.evaluate(() => closeDrawer());
  await z.done();
})().catch(z.crash);
