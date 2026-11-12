// З13-и2 «„Бухгалтерия“ в „Экономике“ и ставки на экране» (OWNER.md, трек З; решения основателя 04.10, З-Р37), браузер.
// «Экономика» → «Бухгалтерия» — выбор из вариантов, а не числа: плательщик VAT, когда VAT с пополнения кошелька, ставка
// пополнения (ставки — с сервера), подсказка «уточните у своего бухгалтера», пояснение «как считаем» по выбранному; смена
// расчёта VAT — с предупреждением и взводом на «Сохранить», ставка пополнения — без. «Ставка VAT» в шторках зоны, пакета
// и позиции и колонкой в списках владельца; смена зоны в шторке пакета выбор не сбрасывает; не тронутая ставка в запрос не
// уходит; «ко всей категории» — одной кнопкой со взводом; новой позиции ставка подставляется по категории, пока её не
// выбрали руками. Палитра: «vat» → «Ставки VAT» → карточка «Бухгалтерия». У админа ставок на экране и в палитре нет, ставку
// категории сервер ему не даёт. Телефон 360 — без прокрутки вбок.
// Данные: позиции «z13u Суп» и «z13u Салат» (категория «z13u») заводятся каждый прогон и удаляются в конце; настройки
// «Бухгалтерии», ставки зоны Standard и пакета «3 часа Standard» в конце — как были.
const z = require('./_z');
(async () => {
  const CAT = 'z13u', A = 'z13u Суп', B = 'z13u Салат';
  const KEYS = "'acc_vat_payer', 'acc_vat_moment', 'acc_topup_vat'";
  const accOrig = z.rows(`SELECT key || '|' || value FROM settings WHERE key IN (${KEYS})`).map(r => r.split('|'));
  const zone = z.sql("SELECT id FROM zones WHERE name = 'Standard'"), zoneVat = z.sql(`SELECT vat_pct FROM zones WHERE id = '${zone}'`);
  const pack = z.sql("SELECT id FROM time_packages WHERE name = '3 часа Standard'");
  const [packVat, packZone] = z.sql(`SELECT vat_pct || '|' || zone_id FROM time_packages WHERE id = '${pack}'`).split('|');
  const drop = () => { for (const n of [A, B]) for (const id of z.rows(`SELECT id FROM goods WHERE name = '${n}'`)) z.dropGood(id); };
  drop();
  z.atEnd(() => {
    drop();
    z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`);
    for (const [k, v] of accOrig) z.sql(`INSERT INTO settings (key, value) VALUES ('${k}', '${v}')`);
    z.sql(`UPDATE zones SET vat_pct = ${zoneVat} WHERE id = '${zone}'`);
    z.sql(`UPDATE time_packages SET vat_pct = ${packVat}, zone_id = '${packZone}' WHERE id = '${pack}'`);
  });
  // с известного начала: настройки по умолчанию (плательщик, при трате, 23 %), у зоны и пакета — 23 %
  z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`);
  z.sql(`UPDATE zones SET vat_pct = 23 WHERE id = '${zone}'`);
  z.sql(`UPDATE time_packages SET vat_pct = 23 WHERE id = '${pack}'`);
  const mk = async (name, price) => ((await z.api('POST', '/admin/goods', { name, category: CAT, price_pln: price }, 'boss')).data.good || {}).id;
  const ga = await mk(A, 12), gb = await mk(B, 15);
  if (!z.check('данные: позиции «z13u Суп» и «z13u Салат» заведены, у зоны Standard и пакета «3 часа Standard» — 23 %', ga && gb && zone && pack,
    JSON.stringify({ ga, gb, zone, pack }))) return z.done();

  const audit = act => +z.sql(`SELECT count(*) FROM admin_actions WHERE action = '${act}'`);
  const lastAudit = act => z.sql(`SELECT details FROM admin_actions WHERE action = '${act}' ORDER BY created_at DESC LIMIT 1`);
  const setting = key => z.sql(`SELECT value FROM settings WHERE key = '${key}'`);
  const vatOf = (table, id) => z.sql(`SELECT vat_pct FROM ${table} WHERE id = '${id}'`);
  const reqs = [];
  const watch = p => p.on('request', r => {
    if (['PUT', 'PATCH', 'POST'].includes(r.method()) && /\/admin\/(settings|zones|packages|goods|vat\/goods)/.test(r.url()))
      reqs.push({ m: r.method(), u: r.url().replace(/^.*\/api\/v1/, ''), b: (() => { try { return JSON.parse(r.postData() || 'null'); } catch (e) { return null; } })() });
  });
  const last = (m, re) => [...reqs].reverse().find(r => r.m === m && re.test(r.u)) || null;
  // на HEAD (до итерации) выборов нет — проверки падают, но прогон доходит до конца
  const pick = async (p, sel, v) => { if (await p.$(sel)) await p.selectOption(sel, v); };
  const click = async (p, sel) => { const e = await p.$(sel); if (e && await e.isVisible()) await e.click(); };
  const saveDrawer = p => click(p, '#drawerBody button:has-text("Сохранить")');
  // строка списка: шапка и ставка (innerText — без подписи «VAT», она только на телефоне)
  const vatCell = (p, wrap, names) => p.evaluate(([wrap, names]) => {
    const t = document.querySelector(wrap + ' table');
    if (!t) return null;
    const head = [...t.rows[0].cells].map(c => c.textContent.trim()), i = head.indexOf('VAT');
    const one = n => { const r = [...t.rows].find(r => r.cells[0] && r.cells[0].innerText.split(' — ')[0].trim() === n); return r && i >= 0 ? r.cells[i].innerText.trim() : null; };
    return { head, vats: names.map(one) };
  }, [wrap, names]);

  // ── «Экономика» → «Бухгалтерия» ──
  const page = await z.openAdmin('boss');
  watch(page);
  await z.nav(page, 'settings');
  await page.waitForSelector('#set_vatpayer', { timeout: 20000 }).catch(() => {});
  const card = await page.evaluate(() => {
    const c = [...document.querySelectorAll('#setGrid .setcard')].find(c => (c.querySelector('h4') || {}).textContent === 'Бухгалтерия');
    const sel = id => { const e = document.getElementById(id); return e && e.tagName === 'SELECT' ? { v: e.value, o: [...e.options].map(o => o.textContent) } : null; };
    return c ? { why: c.querySelector('.why').textContent, payer: sel('set_vatpayer'), moment: sel('set_vatmoment'), topup: sel('set_topupvat'),
      inCard: ['set_vatpayer', 'set_vatmoment', 'set_topupvat'].every(id => c.contains(document.getElementById(id))),
      note: (document.getElementById('setVatNote') || {}).textContent || '' } : null;
  });
  const rates = (await z.api('GET', '/admin/vat/rates', undefined, 'boss')).data.rates || [];
  z.check('«Экономика» → карточка «Бухгалтерия»: подсказка «уточните у своего бухгалтера», три выбора из вариантов, а не числа',
    !!card && /уточните у своего бухгалтера/.test(card.why) && card.inCard && !!card.payer && !!card.moment && !!card.topup, JSON.stringify(card));
  z.check('варианты: «Да — czynny podatnik VAT» / «Нет — освобождён»; «При трате — bon różnego przeznaczenia» / «При пополнении — bon jednego przeznaczenia»; ставки пополнения — с сервера: 23 %, 8 %, 5 %, 0 %, zw',
    !!card && !!card.payer && card.payer.o.join('|') === 'Да — czynny podatnik VAT|Нет — освобождён (zwolniony z VAT)'
    && card.moment.o.join('|') === 'При трате — bon różnego przeznaczenia|При пополнении — bon jednego przeznaczenia'
    && card.topup.o.join('|') === rates.map(r => r.label).join('|') && card.topup.o.join('|') === '23 %|8 %|5 %|0 %|zw',
    JSON.stringify(card && card.payer && [card.payer.o, card.moment.o, card.topup.o]));
  z.check('по умолчанию — плательщик VAT, при трате, 23 %; пояснение: «Пополнение кошелька — без VAT», VAT — при трате по ставкам зоны, пакета и позиции',
    !!card && !!card.payer && card.payer.v === '1' && card.moment.v === '0' && card.topup.v === '23'
    && /Пополнение кошелька — без VAT/.test(card.note) && /по ставке зоны/.test(card.note) && /по ставке позиции/.test(card.note),
    JSON.stringify(card && card.payer && [card.payer.v, card.moment.v, card.topup.v, card.note]));
  await page.evaluate(() => { const e = document.getElementById('set_vatpayer'); if (e) e.closest('.setcard').scrollIntoView({ block: 'center' }); });
  await z.sleep(300);
  await z.shot(page, 'z13_settings');

  const s0 = audit('settings_update'), n0 = reqs.length;
  await pick(page, '#set_vatmoment', '1');
  await pick(page, '#set_topupvat', '8');
  const st = await page.evaluate(() => ({ hint: (document.getElementById('setHint') || {}).textContent || '',
    warn: !!document.getElementById('setVatWarn') && !document.getElementById('setVatWarn').classList.contains('hidden'),
    note: (document.getElementById('setVatNote') || {}).textContent || '',
    on: ['set_vatmoment', 'set_topupvat'].every(id => (document.getElementById(id) || { classList: { contains: () => false } }).classList.contains('changed')) }));
  z.check('«При пополнении» и 8 %: оба выбора подсвечены, «Изменено: 2 — Бухгалтерия», предупреждение про прошлые месяцы, пояснение «VAT — с пополнения, по ставке 8 %»',
    st.on && /Изменено: 2 — Бухгалтерия/.test(st.hint) && st.warn && /с пополнения, по ставке 8 %/.test(st.note), JSON.stringify(st));
  await z.shot(page, 'z13_settings_moment');
  await click(page, '#setSave');
  await z.sleep(500);
  const armed = await z.text(page, '#setSave');
  z.check('первый клик «Сохранить» — взвод «Ещё раз — сменить расчёт VAT», запроса нет', /Ещё раз — сменить расчёт VAT/.test(armed || '') && reqs.length === n0,
    `${armed}; запросов +${reqs.length - n0}`);
  await click(page, '#setSave');
  await z.sleep(1800);
  const put1 = last('PUT', /\/admin\/settings$/);
  z.check('второй клик — один PUT {acc_vat_moment: 1, acc_topup_vat: 8}; в базе 1 и 8; в журнале одна строка «acc_topup_vat: 23→8, acc_vat_moment: 0→1»',
    reqs.length === n0 + 1 && !!put1 && JSON.stringify(put1.b) === JSON.stringify({ acc_vat_moment: 1, acc_topup_vat: 8 })
    && setting('acc_vat_moment') === '1' && setting('acc_topup_vat') === '8'
    && audit('settings_update') === s0 + 1 && lastAudit('settings_update') === 'acc_topup_vat: 23→8, acc_vat_moment: 0→1',
    JSON.stringify({ put1, audit: audit('settings_update') - s0, last: lastAudit('settings_update') }));

  await pick(page, '#set_vatpayer', '0');
  const ex = await page.evaluate(() => ({ note: (document.getElementById('setVatNote') || {}).textContent || '',
    warn: !!document.getElementById('setVatWarn') && !document.getElementById('setVatWarn').classList.contains('hidden') }));
  await click(page, '#setUndo');
  await z.sleep(300);
  const back = await page.evaluate(() => ({ v: (document.getElementById('set_vatpayer') || {}).value, hint: (document.getElementById('setHint') || {}).textContent || '',
    warn: !!document.getElementById('setVatWarn') && !document.getElementById('setVatWarn').classList.contains('hidden'),
    note: (document.getElementById('setVatNote') || {}).textContent || '' }));
  z.check('«Нет — освобождён»: пояснение «освобождён от VAT: в выгрузке не будет колонок VAT», предупреждение; «Вернуть как было» — снова «Да», «Правок нет», пояснение «с пополнения»',
    /освобождён от VAT/.test(ex.note) && /не будет колонок VAT/.test(ex.note) && ex.warn
    && back.v === '1' && /Правок нет/.test(back.hint) && !back.warn && /с пополнения/.test(back.note), JSON.stringify({ ex, back }));

  const n1 = reqs.length;
  await pick(page, '#set_topupvat', '5');
  await click(page, '#setSave');
  await z.sleep(1800);
  const put2 = last('PUT', /\/admin\/settings$/);
  z.check('только ставка пополнения (8 % → 5 %) — без взвода: один клик, PUT {acc_topup_vat: 5}, в базе 5',
    reqs.length === n1 + 1 && !!put2 && JSON.stringify(put2.b) === '{"acc_topup_vat":5}' && setting('acc_topup_vat') === '5', JSON.stringify(put2));

  // ── зоны ──
  const zt = await vatCell(page, '#zonesTable', ['Standard']);
  z.check('«Зоны и цена часа»: колонка VAT после цены часа, у Standard — 23 %',
    !!zt && zt.head.join('|').startsWith('Зона|Цена часа|VAT|') && zt.vats[0] === '23 %', JSON.stringify(zt));
  await page.evaluate(id => zoneEdit(id), zone);
  await z.sleep(500);
  const zd = await page.evaluate(() => { const s = document.getElementById('zn_vat');
    return s ? { v: s.value, o: [...s.options].map(o => o.textContent), label: (document.querySelector('label[for="zn_vat"]') || {}).textContent || '' } : null; });
  z.check('шторка зоны: «Ставка VAT за время — уточните у своего бухгалтера», выбор из ставок, стоит 23 %',
    !!zd && zd.v === '23' && zd.o.join('|') === '23 %|8 %|5 %|0 %|zw' && /уточните у своего бухгалтера/.test(zd.label), JSON.stringify(zd));
  await pick(page, '#zn_vat', '8');
  await z.shot(page, 'z13_zone');
  await saveDrawer(page);
  await z.sleep(1800);
  const zp = last('PATCH', /\/admin\/zones\//), zt2 = await vatCell(page, '#zonesTable', ['Standard']);
  z.check('«Сохранить»: PATCH с vat_pct 8, в базе 8, в списке 8 %, в журнале «VAT 23 % → 8 %»',
    !!zp && !!zp.b && zp.b.vat_pct === 8 && vatOf('zones', zone) === '8' && !!zt2 && zt2.vats[0] === '8 %' && /VAT 23 % → 8 %/.test(lastAudit('zone_update')),
    JSON.stringify({ zp, zt2, log: lastAudit('zone_update') }));
  await page.evaluate(id => zoneEdit(id), zone);
  await z.sleep(500);
  const zv = await page.evaluate(() => (document.getElementById('zn_vat') || {}).value);
  await saveDrawer(page);
  await z.sleep(1800);
  const zp2 = last('PATCH', /\/admin\/zones\//);
  z.check('снова шторка зоны: стоит 8 %; «Сохранить», не тронув ставку, — vat_pct в запросе нет, в базе 8',
    zv === '8' && !!zp2 && zp2 !== zp && !!zp2.b && !('vat_pct' in zp2.b) && vatOf('zones', zone) === '8', JSON.stringify({ zv, zp2 }));
  await page.evaluate(() => zoneEdit(''));
  await z.sleep(400);
  const nz = await page.evaluate(() => (document.getElementById('zn_vat') || {}).value);
  await page.evaluate(() => closeDrawer());
  z.check('новая зона — 23 %', nz === '23', nz);

  // ── пакеты ──
  const pt = await vatCell(page, '#packsTable', ['3 часа Standard']);
  z.check('«Пакеты времени»: колонка VAT после цены, у «3 часа Standard» — 23 %',
    !!pt && pt.head.join('|') === 'Пакет|Зона|Время|Цена|VAT|Час выходит|Срок|' && pt.vats[0] === '23 %', JSON.stringify(pt));
  await page.evaluate(id => packEdit(id), pack);
  await z.sleep(500);
  await pick(page, '#pk_vat', '5');
  const zoneBtn = other => page.evaluate(([other, std]) => { const b = [...document.querySelectorAll('#drawerBody .segs button')]
    .find(b => other ? !b.classList.contains('on') : b.textContent.startsWith(std)); if (b) b.click(); return !!b; }, [other, 'Standard']);
  await zoneBtn(true);
  await z.sleep(300);
  const away = await page.evaluate(() => ({ v: (document.getElementById('pk_vat') || {}).value, zone: packZone }));
  await zoneBtn(false);
  await z.sleep(300);
  await z.shot(page, 'z13_pack');
  await saveDrawer(page);
  await z.sleep(1800);
  const pp = last('PATCH', /\/admin\/packages\//), pt2 = await vatCell(page, '#packsTable', ['3 часа Standard']);
  z.check('шторка пакета: выбрал 5 %, сменил зону — выбор 5 % на месте; вернул зону, «Сохранить» — PATCH с vat_pct 5 и прежней зоной, в базе 5, в списке 5 %',
    away.v === '5' && away.zone !== packZone && !!pp && !!pp.b && pp.b.vat_pct === 5 && pp.b.zone_id === packZone
    && vatOf('time_packages', pack) === '5' && !!pt2 && pt2.vats[0] === '5 %', JSON.stringify({ away, pp, pt2 }));

  // ── ценник ──
  await z.nav(page, 'goods');
  await page.waitForSelector('#goodsTable table', { timeout: 20000 }).catch(() => {});
  const gt = await vatCell(page, '#goodsTable', [A, B]);
  z.check('«Ценник и остатки» у владельца: колонка VAT после цены, у новых позиций — 23 %',
    !!gt && gt.head.join('|').startsWith('Позиция|Категория|Цена|VAT|') && gt.vats.join() === '23 %,23 %', JSON.stringify(gt));
  await page.evaluate(id => editGood(id), ga);
  await z.sleep(500);
  const gd = await page.evaluate(() => ({ v: (document.getElementById('gd_vat') || {}).value, btn: ((document.getElementById('gd_vatcat') || {}).textContent || '').trim(),
    label: (document.querySelector('label[for="gd_vat"]') || {}).textContent || '' }));
  z.check('шторка позиции: «Ставка VAT — уточните у своего бухгалтера», стоит 23 %, рядом «Ко всей категории «z13u»»',
    gd.v === '23' && gd.btn === 'Ко всей категории «z13u»' && /уточните у своего бухгалтера/.test(gd.label), JSON.stringify(gd));
  await pick(page, '#gd_vat', '5');
  const nb = reqs.length, gv0 = audit('goods_vat');
  await click(page, '#gd_vatcat');
  await z.sleep(300);
  const armedCat = await z.text(page, '#gd_vatcat');
  await z.shot(page, 'z13_goods_cat');
  z.check('«Ко всей категории» с 5 % — первый клик: взвод «Всем 2 — 5 %?», запроса нет', armedCat === 'Всем 2 — 5 %?' && reqs.length === nb,
    `${armedCat}; запросов +${reqs.length - nb}`);
  const toastT = await z.toastAfter(page, () => click(page, '#gd_vatcat'), 'поменяли у'); // З19-и13: тост этого действия, а не #toast через паузу
  const vput = last('PUT', /\/admin\/vat\/goods$/), gt2 = await vatCell(page, '#goodsTable', [A, B]);
  z.check('второй клик — PUT /admin/vat/goods {category: «z13u», vat_pct: 5}: у обеих в базе 5, в списке 5 %, тост «поменяли у 2 из 2», в журнале строка категории',
    !!vput && JSON.stringify(vput.b) === JSON.stringify({ category: CAT, vat_pct: 5 }) && vatOf('goods', ga) === '5' && vatOf('goods', gb) === '5'
    && !!gt2 && gt2.vats.join() === '5 %,5 %' && /поменяли у 2 из 2/.test(toastT || '') && audit('goods_vat') === gv0 + 1
    && /«z13u»: VAT → 5 %/.test(lastAudit('goods_vat')), JSON.stringify({ vput, toastT, gt2, log: lastAudit('goods_vat') }));
  await page.evaluate(() => closeDrawer());

  await page.evaluate(() => editGood(''));
  await z.sleep(400);
  const auto = [];
  const autoState = async () => auto.push(await page.evaluate(() => [(document.getElementById('gd_vat') || {}).value,
    ((document.getElementById('gd_vatcat') || {}).textContent || '').trim()].join(' · ')));
  if (await page.$('#gd_cat')) { await page.fill('#gd_cat', CAT); await autoState(); await page.fill('#gd_cat', 'z13u-нет'); await autoState(); }
  await pick(page, '#gd_vat', '0');
  if (await page.$('#gd_cat')) { await page.fill('#gd_cat', CAT); await autoState(); }
  await page.evaluate(() => closeDrawer());
  z.check('новая позиция: категория «z13u» — 5 % (как у всей категории), «z13u-нет» — 23 %; выбрал 0 % руками — категория его уже не меняет',
    auto.join(' | ') === '5 · Ко всей категории «z13u» | 23 · Ко всей категории «z13u-нет» | 0 · Ко всей категории «z13u»', auto.join(' | '));

  await page.evaluate(id => editGood(id), ga);
  await z.sleep(500);
  await pick(page, '#gd_vat', '8');
  await saveDrawer(page);
  await z.sleep(2500);
  const gp = last('PATCH', /\/admin\/goods\//), gt3 = await vatCell(page, '#goodsTable', [A, B]);
  z.check('«Суп»: 8 % и «Сохранить» — PATCH с vat_pct 8; в базе и в списке «Суп» — 8 %, «Салат» — 5 %; в журнале «VAT 5 % → 8 %»',
    !!gp && !!gp.b && gp.b.vat_pct === 8 && vatOf('goods', ga) === '8' && vatOf('goods', gb) === '5' && !!gt3 && gt3.vats.join() === '8 %,5 %'
    && /VAT 5 % → 8 %/.test(lastAudit('good_update')), JSON.stringify({ gp, gt3, log: lastAudit('good_update') }));
  await page.evaluate(id => editGood(id), gb);
  await z.sleep(500);
  const bv = await page.evaluate(() => (document.getElementById('gd_vat') || {}).value);
  await saveDrawer(page);
  await z.sleep(2500);
  const gp2 = last('PATCH', /\/admin\/goods\//);
  z.check('«Салат»: в шторке 5 %; «Сохранить», не тронув ставку, — vat_pct в запросе нет, в базе 5',
    bv === '5' && !!gp2 && gp2 !== gp && gp2.u.endsWith(gb) && !!gp2.b && !('vat_pct' in gp2.b) && vatOf('goods', gb) === '5', JSON.stringify({ bv, gp2 }));
  await page.evaluate(() => document.getElementById('goodsTable').scrollIntoView({ block: 'end' }));
  await z.sleep(300);
  await z.shot(page, 'z13_goods');

  // ── палитра ──
  await page.evaluate(() => palOpen());
  await page.fill('#palInp', 'vat');
  await z.sleep(500);
  const items = await page.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await page.keyboard.press('Enter');
  await z.sleep(2500);
  const pv = await page.evaluate(() => { const e = document.getElementById('set_vatpayer'), r = e ? e.closest('.setcard').getBoundingClientRect() : null;
    return { tab: curTab, seen: !!r && r.top >= 0 && r.bottom <= innerHeight }; });
  z.check('палитра владельца: «vat» → «Ставки VAT», Enter — «Экономика», карточка «Бухгалтерия» на экране', items[0] === 'Ставки VAT' && pv.tab === 'settings' && pv.seen,
    JSON.stringify({ items, pv }));

  // ── админ ──
  await page.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'goods');
  await adm.waitForSelector('#goodsTable table', { timeout: 20000 }).catch(() => {});
  const ah = await adm.evaluate(() => { const t = document.querySelector('#goodsTable table');
    return t ? { head: [...t.rows[0].cells].map(c => c.textContent.trim()), edit: !!t.querySelector('button[onclick^="editGood"]') } : null; });
  await adm.evaluate(() => palOpen());
  await adm.fill('#palInp', 'vat');
  await z.sleep(500);
  const aitems = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await adm.keyboard.press('Escape');
  const aput = await z.api('PUT', '/admin/vat/goods', { category: CAT, vat_pct: 23 }, 'admin1');
  z.check('админ: в «Ценнике» колонки VAT нет и «Правки» нет, в палитре «Ставок VAT» нет; ставку категории сервер ему не даёт — 403, в базе как было',
    !!ah && !ah.head.includes('VAT') && !ah.edit && !aitems.includes('Ставки VAT') && aput.status === 403 && vatOf('goods', ga) === '8' && vatOf('goods', gb) === '5',
    JSON.stringify({ ah, aitems, status: aput.status }));

  // ── телефон ──
  await adm.goto('about:blank');
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'settings');
  await ph.waitForSelector('#set_vatpayer', { timeout: 20000 }).catch(() => {});
  const pc = await ph.evaluate(() => { const e = document.getElementById('set_vatpayer'); if (!e) return null;
    const c = e.closest('.setcard'); c.scrollIntoView({ block: 'start' });
    return { sw: document.documentElement.scrollWidth, fit: ['set_vatpayer', 'set_vatmoment', 'set_topupvat', 'setVatNote']
      .every(id => { const r = document.getElementById(id).getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth; }) }; });
  await z.sleep(400);
  await z.shot(ph, 'z13_settings_phone');
  const pz = await ph.evaluate(() => { const r = [...document.querySelectorAll('#zonesTable tr')].find(t => t.cells[0] && t.cells[0].innerText.trim() === 'Standard');
    if (!r) return null; r.scrollIntoView({ block: 'center' });
    return { text: r.innerText.replace(/\s+/g, ' ').trim(), fit: r.getBoundingClientRect().right <= innerWidth }; });
  await z.sleep(300);
  await z.shot(ph, 'z13_zones_phone');
  await ph.evaluate(id => zoneEdit(id), zone);
  await z.sleep(500);
  const pd = await ph.evaluate(() => { const s = document.getElementById('zn_vat'); return s ? s.getBoundingClientRect().right <= innerWidth : null; });
  await z.shot(ph, 'z13_zone_phone');
  await ph.evaluate(() => closeDrawer());
  await z.nav(ph, 'goods');
  await ph.waitForSelector('#goodsTable table', { timeout: 20000 }).catch(() => {});
  await ph.evaluate(id => editGood(id), ga);
  await z.sleep(500);
  const pg = await ph.evaluate(() => { const s = document.getElementById('gd_vat'), b = document.getElementById('gd_vatcat');
    return s && b ? { fit: [s, b].every(e => { const r = e.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth; }), sw: document.documentElement.scrollWidth } : null; });
  await z.shot(ph, 'z13_goods_phone');
  z.check('телефон 360: «Бухгалтерия» с выборами и пояснением в ширину экрана; у зоны в карточке списка — «VAT 8 %»; в шторках зоны и позиции ставка и «ко всей категории» влезают; прокрутки вбок нет',
    !!pc && pc.fit && pc.sw <= 360 && !!pz && /VAT\s*8 %/.test(pz.text) && pz.fit && pd === true && !!pg && pg.fit && pg.sw <= 360,
    JSON.stringify({ pc, pz, pd, pg }));
  await z.done();
})().catch(z.crash);
