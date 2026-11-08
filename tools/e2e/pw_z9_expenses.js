// З9-и2 «Экран „Расходы“» (№65), браузер: у владельца в «Бухгалтерии» — вкладка «Расходы». Внести: дата (сегодня),
// категория, контрагент — новый заводится при записи, сумма и VAT (под ними — «без VAT»), способ, документ, заметка,
// фото документа (браузер сжимает до 500 КБ, показывается по токену владельца). Найти: строка сверху, итог периода
// вырос, фильтры по категории и способу. Поправить — в шторке по клику на строку. Сторно — только с причиной: строка
// бледнеет, итог — как был. В «Журнале» — «расход: внесён / правка / сторно»; палитра «аренда» ведёт сюда.
// Контрагент «z9e Арендодатель UI» живёт между прогонами; расходы с заметкой «z9e проверка» в конце сторнируются.
const z = require('./_z');
const fs = require('fs'), path = require('path');
(async () => {
  const MARK = 'z9e проверка', CP = 'z9e Арендодатель UI';
  const stornoMine = async () => {
    for (const id of z.sql(`SELECT string_agg(id::text, ' ') FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`).split(' ').filter(Boolean))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
  };
  await stornoMine(); // хвост упавшего прогона
  z.atEnd(stornoMine);
  const total = async () => (await z.api('GET', '/admin/expenses?preset=month', undefined, 'boss')).data.totals.amount_pln;
  const t0 = await total();
  // картинка документа: 1200×900, чтобы сжатие в браузере было настоящим
  const png = path.join(z.OUT, 'z9e_doc.png');

  const page = await z.openAdmin('boss');
  await page.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  const pic = await page.evaluate(() => { const c = document.createElement('canvas'); c.width = 1200; c.height = 900;
    const g = c.getContext('2d'); g.fillStyle = '#fff'; g.fillRect(0, 0, 1200, 900); g.fillStyle = '#123';
    for (let i = 0; i < 40; i++){ g.font = (18 + i % 7) + 'px sans-serif'; g.fillText('FV z9e/1 · Arendodatel · 1234,56 zł · pozycja ' + i, 20, 30 + i * 21); }
    return c.toDataURL('image/png').split(',')[1]; });
  fs.writeFileSync(png, Buffer.from(pic, 'base64'));
  await z.nav(page, 'acc');
  const tab = await page.$('#accTabs button[data-t="exp"]');
  if (!z.check('«Бухгалтерия»: вкладка «Расходы» рядом с «Касса»', !!tab)) return z.done();
  await tab.click(); await z.sleep(1500);
  const k0 = await z.text(page, '#expTotal');
  z.check('«Расходы за период» — итог месяца, как на сервере', k0 === (await page.evaluate(v => fmtPLN(v).replace(/\s+/g, ' '), t0)) + ' zł', `${k0} / ${t0}`);
  await z.shot(page, 'z9_expenses_empty');

  await page.click('#expNewBtn'); await z.sleep(500);
  const dateVal = await page.inputValue('#efDate'), today = await page.evaluate(d => ruDate(d), z.clubDay(0)); // З18-и4д: клубные сутки
  z.check('шторка «Новый расход»: дата — сегодня (клубные сутки) дд.мм.гггг', (await z.text(page, '#drawerTitle')) === 'Новый расход' && dateVal === today, `${dateVal} / ${today}`);
  await page.click('#efSave'); await z.sleep(700);
  const tCat = (await z.text(page, '#toast')) || '';
  await page.selectOption('#efCat', { label: 'Аренда' });
  await page.fill('#efCp', CP);
  await page.fill('#efAmount', '1234.56'); await page.fill('#efVat', '230.85');
  const net = await z.text(page, '#efNet');
  await page.click('#efMethod button:has-text("Банк")');
  await page.fill('#efDoc', 'FV z9e/1'); await page.fill('#efNote', MARK + ': аренда из экрана');
  await page.setInputFiles('#efPhoto', png);
  z.check('без категории — «Выбери категорию»; под суммой — «без VAT: 1 003,71 zł»', /Выбери категорию/.test(tCat) && net === 'без VAT: 1 003,71 zł', `${tCat} / ${net}`);
  await z.shot(page, 'z9_expense_form');
  await page.click('#efSave'); await z.sleep(2500);
  const t1 = (await z.text(page, '#toast')) || '';
  z.check('записано: «✓ Расход 1 234,56 zł внесён — Аренда · фото документа приложено»',
    t1.includes('Расход 1 234,56 zł внесён — Аренда · фото документа приложено'), t1);
  const row = await page.evaluate(() => { const r = document.querySelector('#accBody tr.expRow'); return r ? { id: r.dataset.id, cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) } : null; });
  const db = row && z.sql(`SELECT c.name || '|' || COALESCE(p.name,'') || '|' || e.gross_grosz || '|' || e.vat_grosz || '|' || e.method || '|' || (e.photo IS NOT NULL)::text || '|' || octet_length(e.photo)
    FROM expenses e JOIN expense_categories c ON c.id = e.category_id LEFT JOIN counterparties p ON p.id = e.counterparty_id WHERE e.id = '${row.id}'`);
  const [cat, cp, gross, vat, method, hasPhoto, bytes] = (db || '').split('|');
  z.check('строка сверху: Аренда, новый контрагент, документ с 📎, 1 234,56; в базе — то же, фото сжато ≤ 500 КБ',
    row && row.cells[1] === 'Аренда' && row.cells[2] === CP && /FV z9e\/1 📎/.test(row.cells[3]) && row.cells[5] === '1 234,56'
    && cat === 'Аренда' && cp === CP && gross === '123456' && vat === '23085' && method === 'bank' && hasPhoto === 'true' && +bytes > 0 && +bytes <= 500 * 1024,
    JSON.stringify({ row, db }));
  z.check('итог месяца вырос на 1 234,56', Math.abs((await total()) - t0 - 1234.56) < 0.005, `${t0} → ${await total()}`);
  if (!row) return z.done();

  await page.selectOption('#expCat', { label: 'Аренда' }); await z.sleep(1200);
  const inCat = await page.evaluate(id => !!document.querySelector(`#accBody tr.expRow[data-id="${id}"]`), row.id);
  await page.selectOption('#expMethod', 'card'); await z.sleep(1200);
  const inCard = await page.evaluate(id => !!document.querySelector(`#accBody tr.expRow[data-id="${id}"]`), row.id);
  await page.selectOption('#expCat', ''); await z.sleep(1200);
  await page.selectOption('#expMethod', ''); await z.sleep(1200);
  z.check('фильтры: «Аренда» — расход на месте, способ «Карта» — его нет', inCat && !inCard, `${inCat} ${inCard}`);
  await z.shot(page, 'z9_expenses');

  await page.click(`#accBody tr.expRow[data-id="${row.id}"]`); await z.sleep(1500);
  const img = await page.evaluate(() => { const i = document.querySelector('#efPhotoBox img'); return i ? { w: i.naturalWidth, src: i.src.slice(0, 5) } : null; });
  z.check('клик по строке — шторка «Расход» с фото документа (по токену, blob)', (await z.text(page, '#drawerTitle')) === 'Расход' && img && img.w > 0 && img.src === 'blob:', JSON.stringify(img));
  await page.fill('#efAmount', '1300');
  await page.click('#efSave'); await z.sleep(1800);
  const t2 = (await z.text(page, '#toast')) || '';
  const sum2 = await page.evaluate(id => { const r = document.querySelector(`#accBody tr.expRow[data-id="${id}"]`); return r ? r.cells[5].innerText.replace(/\s+/g, ' ').trim() : null; }, row.id);
  z.check('правка суммы → «✓ Расход поправлен», в строке 1 300,00', /Расход поправлен/.test(t2) && sum2 === '1 300,00', `${t2} / ${sum2}`);

  await page.click(`#accBody tr.expRow[data-id="${row.id}"]`); await z.sleep(1200);
  await page.click('#efStorno'); await z.sleep(200); await page.click('#efStorno'); await z.sleep(700);
  const t3 = (await z.text(page, '#toast')) || '';
  await page.fill('#efReason', 'внесли для проверки');
  await z.shot(page, 'z9_expense_storno');
  await page.click('#efStorno'); await z.sleep(200); await page.click('#efStorno'); await z.sleep(1800);
  const t4 = (await z.text(page, '#toast')) || '';
  const off = await page.evaluate(id => { const r = document.querySelector(`#accBody tr.expRow[data-id="${id}"]`); return r ? { off: r.classList.contains('expOff'), note: r.cells[7].innerText } : null; }, row.id);
  z.check('сторно без причины — «нужна причина»; с причиной — «✓ Расход сторнирован», строка бледная с причиной, итог — как был',
    /нужна причина/.test(t3) && /Расход сторнирован/.test(t4) && off && off.off && /внесли для проверки/.test(off.note)
    && Math.abs((await total()) - t0) < 0.005, JSON.stringify({ t3, t4, off }));

  await page.evaluate(() => nav('hall')); await z.sleep(600);
  await page.evaluate(() => palOpen()); await page.fill('#palInp', 'аренда'); await z.sleep(500);
  const items = await page.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await page.keyboard.press('Enter'); await z.sleep(1500);
  const where = await page.evaluate(() => ({ tab: curTab, acc: (document.querySelector('#accTabs button.on') || {}).textContent }));
  z.check('палитра: «аренда» → «Расходы», Enter — «Бухгалтерия», вкладка «Расходы»', items[0] === 'Расходы' && where.tab === 'acc' && where.acc === 'Расходы',
    JSON.stringify({ items, where }));
  await z.nav(page, 'grants');
  const jr = await page.evaluate(() => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => /расход: /.test(t)).slice(0, 4));
  z.check('«Журнал»: «расход: внесён», «расход: правка», «расход: сторно» — по-русски', ['внесён', 'правка', 'сторно'].every(w => jr.some(t => t.includes('расход: ' + w))), JSON.stringify(jr));
  await z.done();
})().catch(z.crash);
