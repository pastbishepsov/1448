// З9-и5б «Внести заранее», браузер. «Справочники» → «Регулярные расходы»: у шаблона, чей этот месяц уже внесён, —
// «Внести заранее» за следующий; шторка «Расход по шаблону» — «заранее, срок …», дата — сегодня, поля по шаблону; записали
// — тост «… — заранее», остались в «Справочниках», кнопки больше нет (следующий внесён), срок сдвинулся на месяц; в
// «Расходах» у расхода «↻ за <следующий месяц>». Палитра «заранее» ведёт к регулярным расходам.
// Шаблон «z9cp Охрана» стенда удаляется в начале прогона SQL и заводится заново API, в конце — выключен; расходы с
// заметкой «z9cp заранее» сторнируются.
const z = require('./_z');
(async () => {
  const TPL = 'z9cp Охрана', MARK = 'z9cp заранее';
  const cal = z.clubDay(0), D = +cal.slice(8); // «сегодня» шаблонов — клубные сутки (З18-и4д; с полуночи — календарный день клуба)
  const MONTHS = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'];
  const ru = iso => iso.slice(8, 10) + '.' + iso.slice(5, 7) + '.' + iso.slice(0, 4), monthRu = iso => MONTHS[+iso.slice(5, 7) - 1] + ' ' + iso.slice(0, 4);
  const month = k => z.sql(`SELECT (date_trunc('month', '${cal}'::date) + interval '${k} month')::date::text`);
  const dueOf = m => z.sql(`SELECT LEAST('${m}'::date + ${D - 1}, (date_trunc('month', '${m}'::date) + interval '1 month - 1 day')::date)::text`);
  const M = await month(0), NM = await month(1), NNM = await month(2), dueNM = await dueOf(NM), dueNNM = await dueOf(NNM);
  const cats = (await z.api('GET', '/admin/expense-categories', undefined, 'boss')).data.categories || [];
  const other = cats.find(c => c.code === 'other') || {};
  const restore = async () => {
    for (const id of await z.rows(`SELECT id FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
    const id = await z.sql(`SELECT id FROM expense_templates WHERE name = '${TPL}'`);
    if (id) await z.api('PATCH', `/admin/expense-templates/${id}`, { active: false }, 'boss');
  };
  await restore();
  await z.sql(`DELETE FROM expense_templates WHERE name = '${TPL}' RETURNING id`); // заводится заново каждый прогон
  z.atEnd(restore);
  const c = await z.api('POST', '/admin/expense-templates', { name: TPL, category_id: other.id, amount_pln: 150, method: 'bank', day: D, note: MARK }, 'boss');
  const id = ((c.data || {}).template || {}).id;
  const e = await z.api('POST', '/admin/expenses', { op_date: await z.clubDay(0), category_id: other.id, amount_pln: 150, method: 'bank', note: MARK,
    template_id: id, template_month: M }, 'boss'); // этот месяц внесён — «заранее» — следующий

  const boss = await z.openAdmin('boss');
  const after = (act, want) => z.toastAfter(boss, act, want); // З19-и13: тост этого действия, а не #toast через паузу
  const row = () => boss.evaluate(i => { const r = document.querySelector(`#refTpls tr.refRow[data-id="${i}"]`);
    if (!r) return null; const b = [...r.querySelectorAll('button')].find(x => x.textContent === 'Внести заранее');
    return { cells: [...r.cells].map(x => x.innerText.replace(/\s+/g, ' ').trim()), early: b ? b.title : null }; }, id);
  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  await boss.click('#accTabs button[data-t="refs"]'); await z.sleep(1500);
  const r0 = await row();
  if (!z.check(`«Справочники»: у «${TPL}» (этот месяц внесён) — «Внести заранее» за ${monthRu(NM)}, срок «дальше ${ru(dueNM)}»`,
    c.status === 201 && e.status === 201 && r0 && r0.early === `заплатили раньше срока — внести за ${monthRu(NM)}` && r0.cells[5] === `дальше ${ru(dueNM)} Внести заранее`,
    JSON.stringify({ c: c.status, e: e.status, r0 }))) return z.done();
  await boss.evaluate(() => document.getElementById('refTplsHead').scrollIntoView()); await z.sleep(300);
  await z.shot(boss, 'z9_early_refs');
  await boss.click(`#refTpls tr.refRow[data-id="${id}"] button:has-text("Внести заранее")`); await z.sleep(900);
  const f = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, line: (document.getElementById('efTpl') || {}).textContent,
    date: document.getElementById('efDate').value, amount: document.getElementById('efAmount').value, method: (document.querySelector('#efMethod button.on') || {}).textContent }));
  z.check(`шторка «Расход по шаблону»: «за ${monthRu(NM)} — заранее, срок ${ru(dueNM)}», дата — сегодня, сумма и способ по шаблону`,
    f.title === 'Расход по шаблону' && f.line.includes(`Регулярный «${TPL}» за ${monthRu(NM)} — заранее, срок ${ru(dueNM)}: в срок он уже не будет ждать и не напомнит`)
    && f.date === ru(cal) && f.amount === '150' && f.method === 'Банк', JSON.stringify(f));
  await z.shot(boss, 'z9_early_enter');
  const t1 = await after(() => boss.click('#efSave'), 'Расход 150,00 zł внесён'), r1 = await row(), where = await boss.evaluate(() => ({ acc: accTab, refs: !!document.getElementById('refTpls') }));
  z.check(`«Внести расход» → тост «регулярный «${TPL}» за ${monthRu(NM)} — заранее»; остались в «Справочниках»; кнопки нет, срок «дальше ${ru(dueNNM)}»`,
    t1.includes(`Расход 150,00 zł внесён — Прочее · регулярный «${TPL}» за ${monthRu(NM)} — заранее`) && where.acc === 'refs' && where.refs
    && r1 && r1.early === null && r1.cells[5] === `дальше ${ru(dueNNM)}`, JSON.stringify({ t1, where, r1 }));
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  const mk = await boss.evaluate(n => [...document.querySelectorAll('.expList tr.expRow')].filter(x => x.innerText.includes(n))
    .map(x => { const s = x.cells[1].querySelector('span[title]'); return s ? s.title : null; }), MARK);
  z.check(`«Расходы»: у внесённого заранее — «↻ регулярный расход за ${monthRu(NM)}»`, mk.includes(`регулярный расход за ${monthRu(NM)}`), JSON.stringify(mk));
  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'заранее'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(2000);
  const w = await boss.evaluate(() => { const h = document.getElementById('refTplsHead'), r = h ? h.getBoundingClientRect() : null;
    return { tab: curTab, acc: (document.querySelector('#accTabs button.on') || {}).textContent, seen: !!r && r.top >= 0 && r.top < innerHeight }; });
  z.check('палитра: «заранее» → «Регулярные расходы», Enter — «Справочники», блок шаблонов на экране',
    items[0] === 'Регулярные расходы' && w.tab === 'acc' && w.acc === 'Справочники' && w.seen, JSON.stringify({ items, w }));
  await z.done();
})().catch(z.crash);
