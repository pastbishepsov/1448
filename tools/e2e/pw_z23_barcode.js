// З23-и5б «Штрихкоды — экран» (№54 «в»), браузер. Сканер — быстрый набор цифр и Enter (здесь — keyboard.type с паузой
// 8 мс). «Товары»: скан кладёт позицию в чек (повтор — +1), UPC-A из 12 цифр находит EAN-13 с ведущим нулём, чужой код —
// «не привязан»; одна цифра, как и была, открывает раздел. Шторка приёмки: скан — строка позиции +1 (нет строки — новая).
// Шторка пересчёта: скан — «насчитали» +1, даже если курсор стоял в поле другой позиции (поле возвращается как было);
// медленный набор в поле — обычный ввод, не скан. Карточка позиции (владелец): «Привязать сканом» — код в поле,
// «Сохранить» — штрихкод у позиции, и скан находит её. Данные — позиции «z23k …» (категория z23k), остатки —
// корректировками; в конце позиции удаляются SQL-ом (dropGood). Поставку и акт проверка не сохраняет.
const z = require('./_z');
(async () => {
  const wipe = () => { for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE 'z23k %'`)) z.dropGood(id); };
  wipe();
  z.atEnd(wipe);
  const A = '5901234123457', B = '96385074', U = '036000291452', C = '4006381333931';
  const ids = {};
  for (const [name, code, n] of [['z23k Кола', A, 10], ['z23k Вода', B, 5], ['z23k UPC', U, 3], ['z23k Без кода', '', 2]]) {
    ids[name] = (((await z.api('POST', '/admin/goods', { name, category: 'z23k', price_pln: 5, ...(code ? { barcode: code } : {}) }, 'boss')).data || {}).good || {}).id;
    await z.api('POST', `/admin/goods/${ids[name]}/stock`, { delta: n, reason: 'adjust', note: 'z23k: для проверки сканера' }, 'boss');
  }
  const scan = async (page, code) => { await page.keyboard.type(code, { delay: 8 }); await page.keyboard.press('Enter'); await z.sleep(500); };
  const blur = page => page.evaluate(() => { if (document.activeElement) document.activeElement.blur(); });
  const toastNow = page => page.evaluate(() => (document.getElementById('toast') || {}).textContent || '');

  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'goods'); await z.sleep(1500);
  const has = await adm.evaluate(() => typeof scanTo === 'function' && !!document.getElementById('scanHint'));
  if (!has) { // сканера нет (админка до З23-и5б) — FAIL по каждой проверке, а не обрыв
    for (const n of ['скан в чек', 'UPC-A', 'чужой код', 'цифра — раздел', 'приёмка', 'пересчёт', 'медленный ввод', 'карточка'])
      z.check(n + ': сканера нет', false);
    return z.done();
  }
  // в чек
  await blur(adm);
  await scan(adm, A); const t1 = await toastNow(adm);
  await scan(adm, A);
  await scan(adm, U);
  const c1 = await adm.evaluate(ids => ({ kola: cart[ids['z23k Кола']] || 0, upc: cart[ids['z23k UPC']] || 0, tab: curTab }), ids);
  z.check(`«Товары»: скан Колы дважды — в чеке 2 («${t1}»), раздел не сменился`, c1.kola === 2 && c1.tab === 'goods' && /в чек/.test(t1), JSON.stringify(c1));
  z.check('UPC-A из 12 цифр нашёл позицию с EAN-13 0036000291452', c1.upc === 1, JSON.stringify(c1));
  await scan(adm, C); const t2 = await toastNow(adm);
  const c2 = await adm.evaluate(() => Object.values(cart).reduce((a, b) => a + b, 0));
  z.check(`чужой код — «${t2}», чек не тронут`, /не привязан ни к одной позиции/.test(t2) && c2 === 3, JSON.stringify({ t2, c2 }));
  await adm.evaluate(() => cartClear());
  await blur(adm);
  await adm.keyboard.press('1'); await z.sleep(600);
  const tab1 = await adm.evaluate(() => curTab);
  z.check('одна цифра, как и была, открывает раздел: «1» — «Зал»', tab1 === 'hall', tab1);
  await z.nav(adm, 'goods'); await z.sleep(1500);

  // приёмка
  await adm.click('#supNewBtn'); await z.sleep(800); await blur(adm);
  await scan(adm, B); await scan(adm, B); await scan(adm, A);
  const sl = await adm.evaluate(() => (supForm.lines || []).filter(l => l.good).map(l => [(goodsData.find(g => g.id === l.good) || {}).name, l.qty]));
  z.check(`приёмка: Вода дважды — одна строка «2», Кола — новая строка «1»: ${JSON.stringify(sl)}`,
    JSON.stringify(sl) === JSON.stringify([['z23k Вода', '2'], ['z23k Кола', '1']]), JSON.stringify(sl));
  await adm.evaluate(() => closeDrawer()); await z.sleep(500);

  // пересчёт: курсор в поле Колы, скан Воды
  await adm.click('#invNewBtn'); await z.sleep(1200);
  await adm.evaluate(() => { const b = [...document.querySelectorAll('#invCats button')].find(x => x.textContent === 'z23k'); if (b) b.click(); });
  await z.sleep(400);
  const inp = n => `#invLines .invLine[data-g="${ids[n]}"] input`;
  await adm.click(inp('z23k Кола')); await adm.keyboard.type('3', { delay: 150 });
  await scan(adm, B); await scan(adm, B);
  const v = await adm.evaluate(([k, w]) => [document.querySelector(k).value, document.querySelector(w).value], [inp('z23k Кола'), inp('z23k Вода')]);
  const st = await adm.evaluate(ids => invForm.rows.filter(r => /^z23k /.test(r.name)).map(r => [r.name, r.counted]), ids);
  z.check(`пересчёт: курсор в поле Колы («3»), скан Воды дважды — Вода «насчитали» 2, поле Колы как было: ${JSON.stringify(v)}`,
    v[0] === '3' && v[1] === '2' && JSON.stringify(st.find(r => r[0] === 'z23k Кола')) === JSON.stringify(['z23k Кола', '3']), JSON.stringify({ v, st }));
  await z.shot(adm, 'z23_scan_inv');
  await adm.click(inp('z23k Без кода')); await adm.keyboard.type('12345678', { delay: 110 }); await adm.keyboard.press('Enter'); await z.sleep(400);
  const slow = await adm.evaluate(([b, w]) => [document.querySelector(b).value, document.querySelector(w).value], [inp('z23k Без кода'), inp('z23k Вода')]);
  z.check('медленный набор в поле — обычный ввод: «12345678» осталось в поле, Вода не тронута', slow[0] === '12345678' && slow[1] === '2', JSON.stringify(slow));
  await adm.evaluate(() => { invForm = null; closeDrawer(); }); await z.sleep(400);

  // карточка: привязать сканом
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'goods'); await z.sleep(1500);
  await boss.evaluate(id => editGood(id), ids['z23k Без кода']); await z.sleep(600);
  await boss.click('#gd_scan'); await z.sleep(200);
  await boss.keyboard.type(C, { delay: 8 }); await boss.keyboard.press('Enter'); await z.sleep(400);
  const f = await boss.evaluate(() => ({ v: document.getElementById('gd_barcode').value, open: document.getElementById('drawer').classList.contains('open'),
    t: (document.getElementById('toast') || {}).textContent || '' }));
  await z.shot(boss, 'z23_scan_card');
  await boss.click('#drawerBody button[onclick^="saveGood"]'); await z.sleep(1500);
  const saved = z.sql(`SELECT COALESCE(barcode, 'нет') FROM goods WHERE id = '${ids['z23k Без кода']}'`);
  z.check(`карточка: «Привязать сканом» — код ${f.v} в поле («${f.t}»), шторка не закрылась; «Сохранить» — в базе ${saved}`,
    f.v === C && f.open && /считан/.test(f.t) && saved === C, JSON.stringify({ f, saved }));
  await blur(boss);
  await scan(boss, C);
  const c3 = await boss.evaluate(id => cart[id] || 0, ids['z23k Без кода']);
  z.check('привязанный код сканер находит: «Без кода» — в чеке', c3 === 1, c3);
  await boss.evaluate(() => cartClear());
  await z.done();
})().catch(z.crash);
