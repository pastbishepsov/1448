// З9-и5 «Регулярные расходы», браузер. «Справочники» → «Регулярные расходы»: «Новый регулярный расход» — шторка (название,
// категория, контрагент, сумма, VAT, способ, число месяца с подсказкой про последний день), число 32 — тост, то же имя —
// 409 тостом; заведён на сегодняшнее число — «ждёт внесения», на «Бухгалтерии» бейдж, на вкладке «Расходы» число.
// «Расходы» → «Ждут внесения» → «Внести» — шторка «Расход по шаблону», заполненная по шаблону; записали — тост
// «регулярный … за <месяц>», блок пуст, у расхода ↻, бейдж ушёл. Сторно — месяц снова ждёт. Выключить — бледный, не
// ждёт; включить — снова ждёт; напоминание в канал — тост владельцу. Журнал, палитра «регулярн», админ бейджа не видит,
// телефон без горизонтальной прокрутки.
// Шаблон «z9cp Уборка» стенда удаляется в начале прогона SQL (удаления в API нет), в конце — выключен; контрагент
// «z9cp Клининг» живёт между прогонами; расходы с заметкой «z9cp проверка» сторнируются.
const z = require('./_z');
(async () => {
  const TPL = 'z9cp Уборка', CP = 'z9cp Клининг', MARK = 'z9cp проверка', ALL = ['till', 'bank', 'card', 'owner'];
  const cal = await z.sql("SELECT (now() AT TIME ZONE 'Europe/Warsaw')::date::text"), D = +cal.slice(8);
  const MONTHS = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'];
  const monthRu = MONTHS[+cal.slice(5, 7) - 1] + ' ' + cal.slice(0, 4), ruCal = cal.slice(8) + '.' + cal.slice(5, 7) + '.' + cal.slice(0, 4);
  const cats = (await z.api('GET', '/admin/expense-categories', undefined, 'boss')).data.categories || [];
  const other = cats.find(c => c.code === 'other') || {};
  const tplId = async () => z.sql(`SELECT id FROM expense_templates WHERE name = '${TPL}'`);
  const stornoAll = async () => {
    for (const id of await z.rows(`SELECT id FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
  };
  const restore = async () => {
    await z.api('PUT', '/admin/expense-methods', { on: ALL }, 'boss');
    await stornoAll();
    const id = await tplId();
    if (id) await z.api('PATCH', `/admin/expense-templates/${id}`, { active: false }, 'boss');
  };
  await restore();
  await z.sql(`DELETE FROM expense_templates WHERE name = '${TPL}' RETURNING id`); // заводится заново каждый прогон
  z.atEnd(restore);
  const due0 = (await z.api('GET', '/admin/expenses/due', undefined, 'boss')).data.count || 0;

  const boss = await z.openAdmin('boss');
  const toast = async (ms = 1500) => { await z.sleep(ms); return (await z.text(boss, '#toast')) || ''; };
  const tplRow = () => boss.evaluate(n => { const r = [...document.querySelectorAll('#refTpls tr.refRow')].find(x => x.cells[0].textContent.startsWith(n));
    return r ? { id: r.dataset.id, off: r.classList.contains('refOff'), cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) } : null; }, TPL);
  const dueRow = id => boss.evaluate(i => { const r = document.querySelector(`#expDue tr.dueRow[data-t="${i}"]`);
    return r ? [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) : null; }, id);
  const badge = () => boss.evaluate(() => { const b = document.getElementById('accBadge'), t = document.querySelector('#accTabs button[data-t="exp"]');
    return { rail: b && !b.classList.contains('hidden') ? +b.textContent : 0, tab: t && t.dataset.n ? +t.dataset.n : 0, tabText: t ? t.textContent : null }; });
  const tab = async t => { await boss.click(`#accTabs button[data-t="${t}"]`); await z.sleep(1500); };

  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  await tab('refs');
  const empty = await boss.evaluate(() => ({ head: (document.getElementById('refTplsHead') || {}).textContent, btn: !!document.getElementById('rtNew') }));
  await boss.click('#rtNew'); await z.sleep(700);
  await boss.fill('#rtName', TPL); await boss.selectOption('#rtCat', other.id); await boss.fill('#rtCp', CP);
  await boss.fill('#rtAmount', '350'); await boss.fill('#rtVat', '65.45'); await boss.fill('#rtDay', '31');
  const hint31 = await z.text(boss, '#rtDayHint');
  await boss.fill('#rtDay', '32'); await boss.click('#rtSave');
  const t0 = await toast(900);
  await boss.fill('#rtDay', String(D));
  const dr = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, methods: [...document.querySelectorAll('#rtMethod button')].map(b => b.textContent + (b.classList.contains('on') ? '*' : '')) }));
  z.check('«Справочники»: блок «Регулярные расходы» и «Новый регулярный расход»; шторка — способы (банк по умолчанию), число 31 — «в последний день», 32 — тост',
    empty.head === 'Регулярные расходы' && empty.btn && dr.title === 'Новый регулярный расход' && dr.methods.join(',') === 'Из кассы,Банк*,Карта,Личные деньги владельца'
    && /где такого числа нет — в последний день месяца/.test(hint31) && /Число месяца — от 1 до 31/.test(t0), JSON.stringify({ empty, dr, hint31, t0 }));
  await z.shot(boss, 'z9_recurring_new');
  await boss.click('#rtSave');
  const t1 = await toast(2000), r1 = await tplRow(), b1 = await badge();
  z.check(`завести «${TPL}» 350 zł (VAT 65,45) банком ${D}-го → тост «ждёт внесения уже сейчас», строка «ждёт внесения»; бейдж «Бухгалтерии» и число на «Расходах» +1`,
    t1.includes(`«${TPL}» заведён — ждёт внесения уже сейчас`) && r1 && r1.cells[1] === `Прочее · ${CP}` && r1.cells[2] === '350,00 zł VAT 65,45'
    && r1.cells[3] === 'Банк' && r1.cells[4] === `${D}-го${D > 28 ? ' *' : ''}` && r1.cells[5] === 'ждёт внесения' && b1.rail === due0 + 1 && b1.tab === due0 + 1
    && b1.tabText === 'Расходы', JSON.stringify({ t1, r1, b1, due0 }));
  await z.shot(boss, 'z9_recurring_refs');
  await boss.click('#rtNew'); await z.sleep(700);
  await boss.fill('#rtName', TPL.toUpperCase()); await boss.selectOption('#rtCat', other.id); await boss.fill('#rtAmount', '1'); await boss.fill('#rtDay', '1');
  await boss.click('#rtSave');
  const t2 = await toast();
  await boss.evaluate(() => closeDrawer());
  z.check('то же имя в другом регистре → тост «Шаблон с таким названием уже есть», шторка остаётся', /Шаблон с таким названием уже есть/.test(t2), t2);

  await tab('exp');
  const id = r1 && r1.id, d1 = await dueRow(id);
  z.check(`«Расходы» → «Ждут внесения»: «${TPL} · за ${monthRu}», «срок сегодня», кнопка «Внести»`,
    d1 && d1[0] === `${TPL} · за ${monthRu}` && d1[1] === `Прочее · ${CP}` && d1[2] === '350,00 zł VAT 65,45' && d1[4] === 'срок сегодня' && d1[5] === 'Внести',
    JSON.stringify(d1));
  await z.shot(boss, 'z9_recurring_due');
  await boss.click(`#expDue tr.dueRow[data-t="${id}"] button`); await z.sleep(900);
  const f = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, line: (document.getElementById('efTpl') || {}).textContent,
    date: document.getElementById('efDate').value, cat: document.getElementById('efCat').value, cp: document.getElementById('efCp').value,
    amount: document.getElementById('efAmount').value, vat: document.getElementById('efVat').value, method: (document.querySelector('#efMethod button.on') || {}).textContent }));
  z.check('«Внести» → шторка «Расход по шаблону», заполненная по шаблону: дата — срок, категория, контрагент, сумма, VAT, банк',
    f.title === 'Расход по шаблону' && f.line.includes(`Регулярный «${TPL}» за ${monthRu}`) && f.date === ruCal && f.cat === other.id && f.cp === CP
    && f.amount === '350' && f.vat === '65.45' && f.method === 'Банк', JSON.stringify(f));
  await boss.fill('#efNote', `${MARK}: по шаблону`);
  await z.shot(boss, 'z9_recurring_enter');
  await boss.click('#efSave');
  const t3 = await toast(2000), d2 = await dueRow(id), b2 = await badge();
  const mark = await boss.evaluate(n => { const r = [...document.querySelectorAll('.expList tr.expRow')].find(x => x.innerText.includes(n));
    const s = r && r.cells[1].querySelector('span[title]'); return r ? { id: r.dataset.id, sign: s ? s.textContent : null, title: s ? s.title : null } : null; }, MARK);
  z.check(`«Внести расход» → тост «регулярный «${TPL}» за ${monthRu}»; в «Ждут внесения» его нет, бейдж ушёл; у расхода в списке ↻`,
    t3.includes(`Расход 350,00 zł внесён — Прочее · регулярный «${TPL}» за ${monthRu}`) && d2 === null && b2.rail === due0 && b2.tab === due0
    && mark && mark.sign === '↻' && mark.title === `регулярный расход за ${monthRu}`, JSON.stringify({ t3, d2, b2, mark }));
  await boss.click(`.expList tr.expRow[data-id="${mark && mark.id}"]`); await z.sleep(900);
  const line2 = await z.text(boss, '#efTpl');
  await boss.fill('#efReason', 'проверка: месяц снова ждёт');
  await boss.click('#efStorno'); await z.sleep(300); await boss.click('#efStorno');
  const t4 = await toast(2000), d3 = await dueRow(id);
  z.check('расход по шаблону в шторке: «сторно освободит месяц»; сторно → месяц снова в «Ждут внесения»',
    /Внесён по регулярному расходу за .* — сторно освободит месяц/.test(line2 || '') && /сторнирован/.test(t4) && d3 && d3[4] === 'срок сегодня',
    JSON.stringify({ line2, t4, d3 }));

  await tab('refs');
  await boss.click(`#refTpls tr.refRow[data-id="${id}"] button:has-text("Выключить")`);
  const t5 = await toast(2000), r5 = await tplRow(), b5 = await badge();
  await z.shot(boss, 'z9_recurring_off');
  await boss.click(`#refTpls tr.refRow[data-id="${id}"] button:has-text("Включить")`);
  const t6 = await toast(2000), r6 = await tplRow(), b6 = await badge();
  z.check('«Выключить» → тост «не ждёт и не напоминает», строка бледная «· выключен», срок «—», бейджа нет; «Включить» → «ждёт внесения уже сейчас», бейдж снова',
    t5.includes(`«${TPL}» выключен — не ждёт и не напоминает`) && r5 && r5.off && r5.cells[0] === `${TPL} · выключен` && r5.cells[5] === '—' && b5.rail === due0
    && t6.includes(`«${TPL}» включён — ждёт внесения уже сейчас`) && r6 && !r6.off && r6.cells[5] === 'ждёт внесения' && b6.rail === due0 + 1,
    JSON.stringify({ t5, r5, b5, t6, r6, b6 }));
  await boss.click(`#refTpls tr.refRow[data-id="${id}"] button:has-text("Изменить")`); await z.sleep(800);
  const ed = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, name: document.getElementById('rtName').value, day: document.getElementById('rtDay').value }));
  await boss.fill('#rtAmount', '380'); await boss.click('#rtSave');
  const t7 = await toast(2000), r7 = await tplRow();
  z.check('«Изменить» → шторка «Регулярный расход» с полями шаблона; сумма 380 → «поправлен», в строке 380,00 zł',
    ed.title === 'Регулярный расход' && ed.name === TPL && ed.day === String(D) && /Регулярный расход поправлен/.test(t7) && r7 && r7.cells[2] === '380,00 zł VAT 65,45',
    JSON.stringify({ ed, t7, r7 }));
  await z.sql(`UPDATE expense_templates SET reminded_for = NULL WHERE id = '${id}' RETURNING id`);
  await z.api('POST', '/admin/expenses/due/remind', {}, 'boss');
  const t8 = await toast(1800);
  const msg = await z.sql(`SELECT visible_to || '|' || text FROM staff_messages WHERE text LIKE '%«${TPL}»%' ORDER BY created_at DESC LIMIT 1`);
  z.check('напоминание: в канал «только владельцу» — «Пора внести регулярный расход … 380,00 zł, банк; срок сегодня»; у владельца тост',
    t8.includes('Регулярный расход ждёт внесения — «Бухгалтерия» → «Расходы»') && msg.startsWith('owner|') && msg.includes(`«${TPL}» — 380,00 zł, банк; срок сегодня`),
    JSON.stringify({ t8, msg }));

  await z.nav(boss, 'grants');
  const jr = await boss.evaluate(() => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).slice(0, 40));
  const has = w => jr.some(t => t.includes(w));
  z.check('«Журнал»: «регулярный: новый», «регулярный: правка» (выключен, включён, сумма), «расход: внесён … по шаблону»',
    has('регулярный: новый') && has('регулярный: правка') && jr.some(t => t.includes('расход: внесён') && t.includes(`по шаблону «${TPL}»`)),
    JSON.stringify(jr.filter(t => /регулярный|по шаблону/.test(t)).slice(0, 4)));
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'регулярн'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(2000);
  const where = await boss.evaluate(() => { const h = document.getElementById('refTplsHead'), r = h ? h.getBoundingClientRect() : null;
    return { tab: curTab, acc: (document.querySelector('#accTabs button.on') || {}).textContent, seen: !!r && r.top >= 0 && r.top < innerHeight }; });
  z.check('палитра: «регулярн» → «Регулярные расходы», Enter — «Справочники», блок шаблонов на экране', items[0] === 'Регулярные расходы' && where.tab === 'acc'
    && where.acc === 'Справочники' && where.seen, JSON.stringify({ items, where }));

  const adm = await z.openAdmin('admin1');
  const a = await adm.evaluate(() => ({ acc: !document.getElementById('r-acc').classList.contains('hidden'), badge: !document.getElementById('accBadge').classList.contains('hidden') }));
  z.check('админ: «Бухгалтерии» и её бейджа не видит', !a.acc && !a.badge, JSON.stringify(a));
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'acc');
  await ph.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  const w = await ph.evaluate(i => ({ page: document.documentElement.scrollWidth, due: !!document.querySelector(`#expDue tr.dueRow[data-t="${i}"]`) }), id);
  await z.shot(ph, 'z9_recurring_phone');
  z.check('телефон 360 px: «Ждут внесения» на месте, без горизонтальной прокрутки', w.page <= 360 && w.due, JSON.stringify(w));
  await z.done();
})().catch(z.crash);
