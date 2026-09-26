// З8-и4 «Итог ленты продаж», браузер: в «Товарах» шапка «Продажи смены» — итог по всем продажам клубных суток (как
// отдаёт сервер), а когда строк за сутки больше, чем в списке, — подпись «в списке последние 300 из N». Лента на
// время делается полной: 300 отменённых продаж картой «z8 заглушка» без гостя и товара (в выручку, долг и ящик они
// не идут), в конце заглушки удаляются.
const z = require('./_z');
(async () => {
  const STUB = 'z8 заглушка', boss = z.uid('boss');
  z.sql(`DELETE FROM sales WHERE name = '${STUB}'`); // хвост упавшего прогона
  z.atEnd(() => z.sql(`DELETE FROM sales WHERE name = '${STUB}'`));
  z.sql(`INSERT INTO sales (club_id, name, qty, price_pln, total_pln, method, created_by, voided_at, voided_by, created_at)
    SELECT (SELECT id FROM clubs ORDER BY created_at LIMIT 1), '${STUB}', 1, 1, 1, 'card', '${boss}', now(), '${boss}',
    now() + g * interval '1 millisecond' FROM generate_series(1, 300) AS g`);
  const d = (await z.api('GET', '/admin/sales', undefined, 'admin1')).data;
  const page = await z.openAdmin('admin1');
  await z.nav(page, 'goods');
  const head = await z.text(page, '#salesTotal');
  const money = await page.evaluate(v => fmtPLN(v).replace(/\s+/g, ' '), d.total_pln || 0);
  z.check(`«Продажи смены»: итог ${money} zł — по всем продажам суток, «в списке последние 300 из ${d.total_count}»`,
    d.total_count > 300 && (head || '').includes(`· ${money} zł`) && (head || '').includes(`в списке последние 300 из ${d.total_count}`), head);
  await page.evaluate(() => document.getElementById('salesTotal').scrollIntoView({ block: 'center' }));
  await z.sleep(300);
  await z.shot(page, 'z8_sales');
  // все строки суток в списке — подписи нет (ответ подставляется в странице, стенд не трогаем)
  const plain = await page.evaluate(() => { const keep = lastSalesResp;
    paintSales({ ...keep, total_count: (keep.sales || []).length });
    const t = document.getElementById('salesTotal').textContent; paintSales(keep); return t; });
  z.check('когда в списке все строки суток — подписи «в списке последние» нет', !/в списке последние/.test(plain), plain);
  await z.done();
})().catch(z.crash);
