// З10-и6 «Цены поставки — экран «Закупки»», браузер (решения основателя 28–29.09). Владелец: «Бухгалтерия» → «Закупки» —
// своё число на вкладке, «Ждут цен» и «Оценённые». «Оценить» — шторка: строки «пришло · ушло», у строки «штук × цена за
// штуку» и «+ цена», итог по поставке, контрагент (новый заводится при записи), документ, дата, способ, «оплачен»,
// категория расхода (по умолчанию «Закупка товаров»), VAT по документу. Цены не сходятся со штуками — тост без запроса.
// «Записать цены» — тост «Поставка оценена: 140,00 zł · расход заведён — Закупка товаров», поставка в «Оценённых», проданное
// — по первой цене. «Цены» у оценённой — переоценка, расход переписан; поправили его сумму вручную — пометка у поставки.
// «Расходы»: у расхода поставки «📦», в шторке — откуда он и ссылка на цены. «Товары» → «Поставки» у владельца — «Цены»;
// у админа нет ни кнопки, ни «Закупок» в палитре. Журнал «закупка: цены», палитра «закупки». Телефон 360 — без прокрутки.
// Данные: позиции «z10p Цены окно» и «z10p Цены ящик» (категория z10) заводятся каждый прогон и в конце удаляются вместе с
// поставками; контрагент «z10p Поставщик UI» живёт между прогонами (первый прогон заводит его с экрана); расходы поставок
// в конце сторнируются; продажа — картой, в уборке отменяется (З18-и4в: иначе копится в «Что покупают» стенда).
const z = require('./_z');
(async () => {
  const A = 'z10p Цены окно', B = 'z10p Цены ящик', MARK = 'z10p цены', CP = 'z10p Поставщик UI';
  const sups = [];
  const gone = async () => { // расходы поставок проверки — по заметке «поставка от …: z10p Цены …» (и после упавшего прогона)
    for (const eid of z.rows(`SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%z10p Цены%'`))
      await z.api('POST', `/admin/expenses/${eid}/storno`, { reason: 'уборка проверки' }, 'boss');
    for (const n of [A, B]) for (const id of z.rows(`SELECT id FROM goods WHERE name = '${n}'`)) {
      for (const s of z.rows(`SELECT id FROM sales WHERE good_id = '${id}' AND voided_at IS NULL`)) // З18-и4в
        await z.api('POST', `/admin/sales/${s}/void`, {}, 'boss');
      z.dropGood(id);
    }
  };
  await gone(); z.atEnd(gone);
  const mk = async (name, price) => ((await z.api('POST', '/admin/goods', { name, category: 'z10', price_pln: price }, 'boss')).data.good || {}).id;
  const ga = await mk(A, 5), gb = await mk(B, 4);
  const receive = async (lines, note) => { const id = ((await z.api('POST', '/admin/supplies', { lines, note }, 'admin1')).data.supply || {}).id; sups.push(id); return id; };
  const sid = await receive([{ good_id: ga, qty: 30 }, { good_id: gb, qty: 20 }], `${MARK}: накладная UI`);
  const sale = ((await z.api('POST', '/admin/sales', { good_id: ga, qty: 12, method: 'card' }, 'admin1')).data.sale || {}).id;
  const ov = (await z.api('GET', '/admin/overview', undefined, 'boss')).data, sup0 = ov.supplies_waiting || 0, due0 = ov.expenses_due || 0;
  const today = z.clubDay(0).split('-').reverse().join('.'); // дата документа — сегодня клубных суток (З18-и4д): расход в «Расходах» за этот месяц
  const lots = (gid, s) => z.rows(`SELECT part || ' ' || qty_in || '/' || qty_left || ' @' || COALESCE(price_grosz::text, '-') FROM stock_lots WHERE good_id = '${gid}' AND supply_id = '${s}' ORDER BY part`);

  const boss = await z.openAdmin('boss');
  const toast = async (p, ms = 2000) => { await z.sleep(ms); return (await z.text(p, '#toast')) || ''; };
  const tabs = () => boss.evaluate(() => { const t = document.querySelector('#accTabs button[data-t="pur"]'), b = document.getElementById('accBadge');
    return { text: t ? t.textContent : null, n: t && t.dataset.n ? +t.dataset.n : 0, rail: b && !b.classList.contains('hidden') ? +b.textContent : 0 }; });
  const rowOf = (tbl, id) => boss.evaluate(([t, i]) => { const r = document.querySelector(`#${t} tr.purRow[data-id="${i}"]`);
    return r ? [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) : null; }, [tbl, id]);
  const drawer = () => boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent,
    heads: [...document.querySelectorAll('#purLines .purHead')].map(h => h.innerText.replace(/\s+/g, ' ').trim()),
    pieces: [...document.querySelectorAll('#purLines .purLine')].map(l => [...l.querySelectorAll('.purPiece')].map(p => [...p.querySelectorAll('input')].map(i => i.value).join('×'))),
    total: (document.getElementById('purTotal') || {}).innerText, exp: ((document.getElementById('purExp') || {}).innerText || '').replace(/\s+/g, ' '),
    flag: [...document.querySelectorAll('#drawer .purFlag')].map(x => x.innerText).join(' '),
    cat: (document.getElementById('purCat') || {}).selectedOptions ? document.getElementById('purCat').selectedOptions[0].text : null,
    method: (document.querySelector('#purMethod button.on') || {}).textContent, save: (document.getElementById('purSave') || {}).textContent }));
  const piece = (li, pi) => `#purLines .purLine:nth-child(${li}) > .purPiece:nth-child(${pi + 1}) input`; // первый ребёнок строки — шапка

  await z.nav(boss, 'acc');
  const hasPur = await boss.evaluate(() => !!document.querySelector('#accTabs button[data-t="pur"]')); // на старом коде вкладки нет
  if (hasPur){ await boss.click('#accTabs button[data-t="pur"]'); await z.sleep(1800); }
  const t0 = await tabs(), w0 = await rowOf('purWait', sid);
  if (!z.check(`«Бухгалтерия» → «Закупки»: вкладка со своим числом ${sup0}; поставка — в «Ждут цен» с «Оценить»`,
    t0.text === 'Закупки' && t0.n === sup0 && t0.rail === due0 + sup0 && w0 && w0[2] === `${A} ×30, ${B} ×20` && w0[4] === 'Оценить', JSON.stringify({ t0, w0 }))) return z.done();
  await boss.evaluate(() => document.getElementById('purWaitHead').scrollIntoView()); await z.sleep(300);
  await z.shot(boss, 'z10_pricing_list');

  section('оценка');
  await boss.click(`#purWait tr.purRow[data-id="${sid}"] button`); await z.sleep(1200);
  const d0 = await drawer();
  z.check('шторка «Цены поставки»: строки «пришло 30 · ушло 12» и «пришло 20», по куску на строку без цены, категория «Закупка товаров», способ «Банк»',
    d0.title === 'Цены поставки' && d0.heads[0] === `${A} пришло 30 · ушло 12` && d0.heads[1] === `${B} пришло 20` && JSON.stringify(d0.pieces) === '[["30×"],["20×"]]'
    && d0.cat === 'Закупка товаров' && d0.method === 'Банк' && d0.save === 'Записать цены', JSON.stringify(d0));
  await boss.fill(piece(1, 1) + ':nth-of-type(2)', '2.5');
  await boss.click('#purSave');
  const e1 = await toast(boss, 700);
  z.check(`без цены у «${B}» — тост «цена за штуку …», запроса нет`, e1 === `«${B}»: цена за штуку — от 0, не больше двух знаков после запятой`
    && z.sql(`SELECT status FROM supplies WHERE id = '${sid}'`) === 'waiting', e1);
  await boss.fill(piece(1, 1) + ':nth-of-type(1)', '20');
  await boss.click('#purLines .purLine:nth-child(1) > button'); await z.sleep(400); // «+ цена» — второй кусок с оставшимися 10
  const d1 = await drawer();
  await boss.fill(piece(1, 2) + ':nth-of-type(2)', '2.8');
  await boss.fill(piece(2, 1) + ':nth-of-type(2)', '3.1');
  await boss.fill('#purCp', CP); await boss.fill('#purDoc', 'FV z10p/1'); await boss.fill('#purDate', today); await boss.fill('#purVat', '23');
  const d2 = await drawer();
  z.check('«+ цена» — второй кусок с оставшимися 10 шт (цена 2,50 сохранилась); итог по поставке 140,00 zł',
    JSON.stringify(d1.pieces[0]) === '["20×2.5","10×"]' && d2.total.replace(/\s+/g, ' ') === 'Итого по поставке 140,00 zł', JSON.stringify({ d1: d1.pieces, total: d2.total }));
  await z.shot(boss, 'z10_pricing_drawer');
  await boss.click('#purSave');
  const t1 = await toast(boss), row1 = await rowOf('purDone', sid), w1 = await rowOf('purWait', sid), tb1 = await tabs();
  z.check('«Записать цены» → тост «Поставка оценена: 140,00 zł · расход заведён — Закупка товаров»; поставка — в «Оценённых»: документ, контрагент, 140,00 zł, «Закупка товаров · Банк»; число «Закупок» и бейдж — меньше на 1',
    t1 === 'Поставка оценена: 140,00 zł · расход заведён — Закупка товаров' && !w1 && row1 && row1[1] === `FV z10p/1 от ${today}` && row1[2] === CP
    && row1[3] === `${A} ×30, ${B} ×20` && row1[4] === '140,00 zł' && row1[5] === 'Закупка товаров · Банк' && tb1.n === sup0 - 1 && tb1.rail === due0 + sup0 - 1,
    JSON.stringify({ t1, row1, tb1 }));
  z.check('на складе: у «окна» два куска — проданные 12 ушли по первой цене (20/8 @250, 10/10 @280), «ящик» 20/20 @310',
    JSON.stringify(lots(ga, sid)) === '["1 20/8 @250","2 10/10 @280"]' && JSON.stringify(lots(gb, sid)) === '["1 20/20 @310"]', JSON.stringify([lots(ga, sid), lots(gb, sid)]));

  section('переоценка');
  await boss.click(`#purDone tr.purRow[data-id="${sid}"] button`); await z.sleep(1200);
  const d3 = await drawer();
  z.check('«Цены» у оценённой: «Цены поставки — переоценка», куски как записаны, расход поставки «140,00 zł · Закупка товаров … — переоценка его перепишет»',
    d3.title === 'Цены поставки — переоценка' && JSON.stringify(d3.pieces) === '[["20×2.5","10×2.8"],["20×3.1"]]'
    && d3.exp === `Расход поставки: 140,00 zł · Закупка товаров · ${today} — переоценка его перепишет` && d3.save === 'Переоценить', JSON.stringify(d3));
  await boss.click('#purLines .purLine:nth-child(1) > .purPiece:nth-child(3) button'); await z.sleep(400); // убрать второй кусок
  await boss.fill(piece(1, 1) + ':nth-of-type(1)', '30'); await boss.fill(piece(1, 1) + ':nth-of-type(2)', '2.7');
  await boss.click('#purSave');
  const t2 = await toast(boss), eid = z.sql(`SELECT expense_id FROM supplies WHERE id = '${sid}'`);
  z.check('переоценка «окна» 30 × 2,70 → тост «Переоценка: 143,00 zł · расход переписан»; расход — 143,00',
    t2 === 'Переоценка: 143,00 zł · расход переписан' && z.sql(`SELECT gross_grosz FROM expenses WHERE id = '${eid}'`) === '14300'
    && JSON.stringify(lots(ga, sid)) === '["1 30/18 @270"]', JSON.stringify({ t2, lots: lots(ga, sid) }));
  await z.api('PATCH', `/admin/expenses/${eid}`, { amount_pln: 150 }, 'boss'); // поправили сумму расхода вручную
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1200);
  await boss.click('#accTabs button[data-t="pur"]'); await z.sleep(1800);
  const row2 = await rowOf('purDone', sid);
  await boss.evaluate(() => document.getElementById('purDoneHead').scrollIntoView()); await z.sleep(300);
  await z.shot(boss, 'z10_pricing_flag');
  z.check('сумму расхода поправили на 150 → у поставки пометка «расход поправлен вручную: 150,00 zł, не равен поставке (+7,00) — переоценка его не тронет»',
    row2 && row2[5].endsWith('расход поправлен вручную: 150,00 zł, не равен поставке (+7,00) — переоценка его не тронет'), JSON.stringify(row2));

  section('«Расходы» и «Товары»');
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1800);
  const er = await boss.evaluate(i => { const r = document.querySelector(`.expList tr.expRow[data-id="${i}"]`); const m = r && r.cells[1].querySelector('span[title]');
    return r ? { mark: m ? m.textContent : null, title: m ? m.title : null } : null; }, eid);
  await boss.click(`.expList tr.expRow[data-id="${eid}"]`); await z.sleep(900);
  const es = await z.text(boss, '#efSup');
  await boss.click('#efSup a'); await z.sleep(1500);
  const d4 = await drawer();
  z.check('«Расходы»: у расхода поставки «📦 расход поставки — цены в «Закупках»», в шторке — откуда он; «Цены поставки» открывают переоценку с пометкой',
    er && er.mark === '📦' && er.title === 'расход поставки — цены в «Закупках»' && es.startsWith('Расход поставки — его завела оценка в «Закупках».')
    && d4.title === 'Цены поставки — переоценка' && d4.flag.includes('расход поправлен вручную: 150,00 zł'), JSON.stringify({ er, es, d4: [d4.title, d4.flag] }));
  await boss.evaluate(() => closeDrawer());
  const sid2 = await receive([{ good_id: gb, qty: 2 }], `${MARK}: подарок`);
  await z.nav(boss, 'goods'); await z.sleep(800);
  const btns = await boss.evaluate(i => { const r = document.querySelector(`#suppliesBox tr.supRow[data-id="${i}"]`);
    return r ? [...r.querySelectorAll('button')].map(b => b.textContent) : null; }, sid2);
  await boss.click(`#suppliesBox tr.supRow[data-id="${sid2}"] button:has-text("Цены")`); await z.sleep(1200);
  await boss.fill(piece(1, 1) + ':nth-of-type(2)', '0');
  await boss.click('#purSave');
  const t3 = await toast(boss), gone2 = await boss.evaluate(i => !document.querySelector(`#suppliesBox tr.supRow[data-id="${i}"]`), sid2);
  z.check('«Товары» → «Поставки» у владельца: «Исправить» и «Цены»; подарок по 0 → тост «Поставка оценена: 0,00 zł — сумма 0, расхода нет», из «ждут цен» ушла',
    JSON.stringify(btns) === '["Исправить","Цены"]' && t3 === 'Поставка оценена: 0,00 zł — сумма 0, расхода нет' && gone2, JSON.stringify({ btns, t3, gone2 }));

  section('журнал, палитра, админ, телефон');
  await z.nav(boss, 'grants');
  const jr = await boss.evaluate(() => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => t.includes('закупка: цены')).slice(0, 3));
  z.check('«Журнал»: «закупка: цены» — строки с ценами, сумма, способ', jr.some(t => t.includes(`${A} 20 × 2,50 + 10 × 2,80, ${B} 20 × 3,10 · 140,00 zł · банк · FV z10p/1`))
    && jr.some(t => t.includes('переоценка — поставка от ')), JSON.stringify(jr));
  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'закупки'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(2000);
  const pw = await boss.evaluate(() => ({ tab: curTab, acc: (document.querySelector('#accTabs button.on') || {}).textContent }));
  z.check('палитра владельца: «закупки» → «Закупки», Enter — «Бухгалтерия», вкладка «Закупки»', items[0] === 'Закупки' && pw.tab === 'acc' && pw.acc.startsWith('Закупки'),
    JSON.stringify({ items, pw }));
  const sid3 = await receive([{ good_id: gb, qty: 3 }], `${MARK}: для телефона`);
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'goods');
  const ab = await adm.evaluate(i => { const r = document.querySelector(`#suppliesBox tr.supRow[data-id="${i}"]`);
    return r ? [...r.querySelectorAll('button')].map(b => b.textContent) : null; }, sid3);
  await adm.evaluate(() => palOpen()); await adm.fill('#palInp', 'закупки'); await z.sleep(500);
  const ai = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  z.check('админ: в «Поставках» только «Исправить», «Закупок» в палитре нет', JSON.stringify(ab) === '["Исправить"]' && !ai.includes('Закупки'), JSON.stringify({ ab, ai }));
  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // три окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => accGo('pur')); await z.sleep(3500);
  const p1 = await ph.evaluate(i => ({ sw: document.documentElement.scrollWidth, row: !!document.querySelector(`#purWait tr.purRow[data-id="${i}"]`) }), sid3);
  await z.shot(ph, 'z10_pricing_phone');
  await ph.click(`#purWait tr.purRow[data-id="${sid3}"] button`); await z.sleep(1200);
  await ph.click('#purLines .purLine:nth-child(1) > button'); await z.sleep(400);
  const p2 = await ph.evaluate(() => { const d = document.getElementById('drawer').getBoundingClientRect();
    return { sw: document.documentElement.scrollWidth, fit: [...document.querySelectorAll('#purLines .purPiece')].every(l => [...l.children].every(c => c.getBoundingClientRect().right <= d.right + 1)) }; });
  await z.shot(ph, 'z10_pricing_phone_drawer');
  z.check('телефон 360: «Закупки» и шторка цен — без горизонтальной прокрутки, куски в ширину шторки', p1.sw <= 360 && p1.row && p2.sw <= 360 && p2.fit,
    JSON.stringify({ p1, p2 }));
  await z.done();
})().catch(z.crash);

function section(t) { console.log(`\n── ${t}`); }
