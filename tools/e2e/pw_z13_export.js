// З13-и7 «Вкладка «Выгрузка»», браузер (№75, №76; решения основателя 04.10, З-Р37). «Бухгалтерия» → «Выгрузка» —
// только владельцу: по умолчанию прошлый месяц, ‹ › листают месяцы (дальше текущего — нет), свои даты; XLSX или CSV,
// заголовки по-польски или по-русски (выбор помнит браузер); профиль учёта строкой со ссылкой в «Экономику»; листы с
// числом строк — как превью сервера (&preview=1); «Скачать» — файл с именем от сервера, выгрузка — в журнале. Ниже —
// «Файл» (уходят только изменённые ключи) и «Реквизиты фирмы» (NIP с контрольной суммой). «Экономика» → «Бухгалтерия» —
// форма учёта первым полем и ставка ryczałtu времени: от формы — первые листы. «Справочники» — колонка KPiR и счёт
// категории расходов, ставки ryczałtu позиций по категориям («Всей категории» со взводом, «Позиции» — шторкой). Журнал —
// по-русски; палитра — «выгрузка», «бухгалтер», «kpir»; у админа нет ни вкладки, ни палитры, превью — 403. Телефон 360 —
// без прокрутки вбок.
// Данные: позиции «z13x Piwo» и «z13x Chipsy» (категория «z13x») заводятся каждый прогон и в конце удаляются; у «Аренды»
// на время проверки — колонка 15 и счёт 402, в конце — как были; настройки «Бухгалтерии», файла и реквизиты фирмы — как
// были.
const z = require('./_z');
const { execFileSync } = require('child_process');
(async () => {
  const MARK = 'z13x';
  const KEYS = ['acc_form', 'acc_ryczalt_time', 'acc_vat_payer', 'acc_vat_moment', 'acc_csv_sep', 'acc_csv_decimal', 'acc_csv_enc', 'acc_date_fmt',
    'acc_kpkw', 'acc_firm_name', 'acc_firm_nip', 'acc_firm_address'].map(k => `'${k}'`).join(', ');
  const orig = z.rows(`SELECT key || '|' || value FROM settings WHERE key IN (${KEYS})`).map(r => { const i = r.indexOf('|'); return [r.slice(0, i), r.slice(i + 1)]; });
  const put = (k, v) => z.sql(`INSERT INTO settings (key, value) VALUES ('${k}', '${String(v).replace(/'/g, "''")}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`);
  const cats = async () => (await z.api('GET', '/admin/expense-categories', undefined, 'boss')).data.categories || [];
  const rent0 = (await cats()).find(c => c.code === 'rent') || {};
  const gone = async () => {
    for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE '${MARK} %'`)) z.dropGood(id);
    z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`);
    for (const [k, v] of orig) put(k, v);
  };
  await gone(); z.atEnd(gone);
  z.atEnd(async () => { if (rent0.id) await z.api('PATCH', `/admin/expense-categories/${rent0.id}`, { kpir_col: rent0.kpir_col, account: rent0.account || '' }, 'boss'); });
  z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`); // с известного начала: KPiR, плательщик VAT, при трате, файл по умолчанию, реквизитов нет
  if (rent0.id && (rent0.kpir_col !== 15 || rent0.account !== '402')) // «Аренда» — как после миграции 084
    await z.api('PATCH', `/admin/expense-categories/${rent0.id}`, { kpir_col: 15, account: '402' }, 'boss');
  const mk = async name => ((await z.api('POST', '/admin/goods', { name, category: MARK, price_pln: 5, stock: 5 }, 'boss')).data.good || {}).id;
  const gp = await mk(MARK + ' Piwo'), gc = await mk(MARK + ' Chipsy');
  const rent = (await cats()).find(c => c.code === 'rent') || {}, gr = z.rows(`SELECT ryczalt_rate FROM goods WHERE name LIKE '${MARK} %'`).join();
  if (!z.check('данные: позиции «z13x Piwo» и «z13x Chipsy» заведены — ryczałt 3 %; у «Аренды» — колонка 15, счёт 402',
    gp && gc && gr === '30,30' && rent.kpir_col === 15 && rent.account === '402', JSON.stringify({ gp, gc, gr, rent }))) return z.done();

  const addm = (m, k) => { const d = new Date(+m.slice(0, 4), +m.slice(5, 7) - 1 + k, 1); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0'); };
  const MN = ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'];
  const mname = m => MN[+m.slice(5, 7) - 1] + ' ' + m.slice(0, 4);
  const dmy = iso => iso.slice(8, 10) + '.' + iso.slice(5, 7) + '.' + iso.slice(0, 4);
  const lastDay = m => m + '-' + String(new Date(+m.slice(0, 4), +m.slice(5, 7), 0).getDate()).padStart(2, '0');
  const TODAY = z.clubDay(0), CUR = TODAY.slice(0, 7), PREV = addm(CUR, -1), PREV2 = addm(CUR, -2);
  const preview = async (q, fmt = 'xlsx', lang = 'pl') => (await z.api('GET', `/admin/export?${q}&format=${fmt}&lang=${lang}&preview=1`, undefined, 'boss')).data;
  const screen = p => p.evaluate(() => {
    const t = id => ((document.getElementById(id) || {}).textContent || '').replace(/\s+/g, ' ').trim();
    const on = id => [...document.querySelectorAll(`#${id} button.on`)].map(b => b.dataset.f || b.dataset.l).join();
    const mn = document.getElementById('outMonthName'), next = document.querySelector('#outMonth button:last-child');
    return { month: t('outMonthName'), monthOn: !!mn && mn.classList.contains('on'), next: next ? next.disabled : null, label: t('outLabel'),
      prof: t('outProf'), name: t('outName'), fmt: on('outFmt'), lang: on('outLang'), head: [...document.querySelectorAll('#outSheets th')].map(x => x.textContent).join('|'),
      sheets: [...document.querySelectorAll('#outSheets tr')].slice(1).map(r => [...r.cells].map(c => c.textContent.replace(/\s+/g, ' ').trim())) };
  });
  // листы на экране — как у сервера: номер, имя (у CSV — и файл в архиве), число данных
  const same = (s, pv, csv) => s.sheets.length === (pv.sheets || []).length && s.sheets.every((r, i) => r[0] === String(i + 1)
    && r[1] === pv.sheets[i].name + (csv ? pv.sheets[i].file : '') && (r[2].replace(/\s/g, '').match(/^\d+/) || [''])[0] === String(pv.sheets[i].rows));
  // действие, после которого «Выгрузка» собирается заново: ждём, пока новая сборка дорисуется
  const outWait = async (p, act) => {
    const s = await p.evaluate(() => typeof outSeq === 'undefined' ? -1 : outSeq);
    await act();
    await p.waitForFunction(s => outSeq > s && !!document.getElementById('outSheets') && !document.getElementById('outMain').classList.contains('ovBusy'), s, { timeout: 30000 }).catch(() => {});
    await z.sleep(300);
  };
  const zipNames = f => JSON.parse(execFileSync('python3', ['-I', '-c', 'import sys, json, zipfile; print(json.dumps(zipfile.ZipFile(sys.argv[1]).namelist()))', f]).toString());

  const boss = await z.openAdmin('boss');
  const puts = []; // правки с экрана: «PUT /settings {…}»
  boss.on('request', r => { if (['PUT', 'PATCH'].includes(r.method()) && /\/admin\/(settings|export\/requisites|expense-categories|ryczalt|goods\/)/.test(r.url()))
    puts.push(r.method() + ' ' + r.url().replace(/^.*\/admin/, '') + ' ' + (r.postData() || '')); });
  await z.nav(boss, 'acc');
  const tabs = await boss.evaluate(() => [...document.querySelectorAll('#accTabs button')].map(b => b.textContent).join());
  z.check('«Бухгалтерия»: вкладки «Обзор», «Касса», «Расходы», «Закупки», «Справочники», «Выгрузка»', tabs === 'Обзор,Касса,Расходы,Закупки,Справочники,Выгрузка', tabs);
  await outWait(boss, () => boss.click('#accTabs button[data-t="out"]'));
  const P0 = await preview('preset=prev_month'), s0 = await screen(boss);
  z.check(`«Выгрузка» — прошлый месяц: «${mname(PREV)}», ${dmy(PREV + '-01')} — ${dmy(lastDay(PREV))}, клубные сутки; XLSX и по-польски; файл «1448-eksport-${PREV}.xlsx»`,
    s0.month === mname(PREV) && s0.monthOn && s0.next === false && s0.label.startsWith(`${dmy(PREV + '-01')} — ${dmy(lastDay(PREV))} · клубные сутки с`)
    && s0.fmt === 'xlsx' && s0.lang === 'pl' && s0.name === `1448-eksport-${PREV}.xlsx` && P0.file === s0.name, JSON.stringify(s0).slice(0, 600));
  z.check('листы с числом строк — как превью сервера: «O eksporcie», «Zapisy do KPiR», «Wszystkie operacje»…; у «Sprzedaż wg dni i stawek» — дни с продажами, '
    + 'у «Raport kasowy» — документы', same(s0, P0, false) && s0.head === '№|Лист|Данных'
    && s0.sheets.slice(0, 3).map(r => r[1]).join('|') === 'O eksporcie|Zapisy do KPiR|Wszystkie operacje'
    && /с продажами$/.test((s0.sheets.find(r => r[1] === 'Sprzedaż wg dni i stawek') || [])[2] || '')
    && /документ/.test((s0.sheets.find(r => r[1] === 'Raport kasowy') || [])[2] || ''), JSON.stringify({ s: s0.sheets.slice(0, 5), p: (P0.sheets || []).slice(0, 5) }));
  z.check('профиль строкой: «KPiR — книга доходов и расходов», после «О выгрузке» — «Записи для KPiR», «плательщик VAT · VAT при трате», '
    + 'ссылка «поменять — «Экономика» → «Бухгалтерия»»', /^Профиль учёта: KPiR — книга доходов и расходов — после «О выгрузке» — «Записи для KPiR»/.test(s0.prof)
    && / · плательщик VAT · VAT при трате · /.test(s0.prof) && /поменять — «Экономика» → «Бухгалтерия»$/.test(s0.prof), s0.prof);
  await z.shot(boss, 'z13_export');

  await outWait(boss, () => boss.click('#outMonth button:last-child'));
  const s1 = await screen(boss);
  await outWait(boss, () => boss.click('#outMonth button:first-child'));
  await outWait(boss, () => boss.click('#outMonth button:first-child'));
  const s2 = await screen(boss);
  z.check(`› — этот месяц «${mname(CUR)}»: с 1-го по сегодня, «месяц ещё идёт», дальше › нельзя; ‹ ‹ — «${mname(PREV2)}» целиком`,
    s1.month === mname(CUR) && s1.label.startsWith(`${dmy(CUR + '-01')} — ${dmy(TODAY)} ·`) && /месяц ещё идёт/.test(s1.label) && s1.next === true
    && s1.name === `1448-eksport-${CUR}.xlsx` && s2.month === mname(PREV2) && s2.label.startsWith(`${dmy(PREV2 + '-01')} — ${dmy(lastDay(PREV2))} ·`)
    && !/месяц ещё идёт/.test(s2.label) && s2.next === false, JSON.stringify({ s1: [s1.month, s1.label, s1.next, s1.name], s2: [s2.month, s2.label, s2.next] }));
  await boss.fill('#outFrom', dmy(PREV + '-05')); await boss.fill('#outTo', dmy(PREV + '-20'));
  await outWait(boss, () => boss.click('#accBody button:has-text("Свои даты")'));
  const s3 = await screen(boss), P3 = await preview(`from=${PREV}-05&to=${PREV}-20`);
  z.check(`свои даты 05–20 «${mname(PREV)}»: подпись и файл «1448-eksport-${PREV}-05_${PREV}-20.xlsx», листы — как у сервера; месяц в переключателе не выбран`,
    s3.label.startsWith(`${dmy(PREV + '-05')} — ${dmy(PREV + '-20')} ·`) && !s3.monthOn && s3.month === mname(PREV)
    && s3.name === `1448-eksport-${PREV}-05_${PREV}-20.xlsx` && same(s3, P3, false), JSON.stringify([s3.label, s3.monthOn, s3.name]));
  await outWait(boss, () => boss.click('#outMonthName'));
  await outWait(boss, () => boss.click('#outFmt button[data-f="csv"]'));
  await outWait(boss, () => boss.click('#outLang button[data-l="ru"]'));
  const s4 = await screen(boss), P4 = await preview('month=' + PREV, 'csv', 'ru');
  z.check(`месяц целиком, CSV по-русски: у листа — файл в архиве («01_o_vygruzke.csv», «02_zapisi_kpir.csv»…), архив «1448-vygruzka-${PREV}.zip», листы — как у сервера`,
    s4.monthOn && s4.fmt === 'csv' && s4.lang === 'ru' && s4.name === `1448-vygruzka-${PREV}.zip` && s4.head === '№|Лист и файл в архиве|Данных' && same(s4, P4, true)
    && s4.sheets.slice(0, 2).map(r => r[1]).join('|') === 'О выгрузке01_o_vygruzke.csv|Записи для KPiR02_zapisi_kpir.csv', JSON.stringify(s4.sheets.slice(0, 3)));
  await boss.reload({ waitUntil: 'load' });
  await boss.waitForSelector('#rail', { state: 'visible', timeout: 15000 });
  await z.sleep(1500);
  await boss.evaluate(() => { const t = document.getElementById('tour'); if (t && !t.classList.contains('hidden')) tourSkip(); });
  await outWait(boss, () => boss.evaluate(() => accGo('out')));
  const s5 = await screen(boss);
  z.check('после перезагрузки формат и язык — как выбрали (CSV, по-русски), месяц — снова прошлый', s5.fmt === 'csv' && s5.lang === 'ru' && s5.month === mname(PREV)
    && s5.monthOn, JSON.stringify([s5.fmt, s5.lang, s5.month]));

  // ── «Скачать» ──
  const T0 = z.sql('SELECT now()::text');
  let dl;
  const t1 = await z.toastAfter(boss, async () => { [dl] = await Promise.all([boss.waitForEvent('download', { timeout: 60000 }), boss.click('#outGet')]); }, 'скачан'); // З19-и13: тост этого действия, а не #toast через паузу
  const zl = zipNames(await dl.path());
  z.check(`«Скачать» CSV: «1448-vygruzka-${PREV}.zip» — архив, файлы — как в списке листов; тост «Файл … скачан — выгрузка в журнале»`,
    dl.suggestedFilename() === `1448-vygruzka-${PREV}.zip` && JSON.stringify(zl) === JSON.stringify((P4.sheets || []).map(x => x.file))
    && /Файл «1448-vygruzka-\d{4}-\d{2}\.zip» скачан — выгрузка в журнале/.test(t1 || ''), JSON.stringify({ name: dl.suggestedFilename(), zl: zl.slice(0, 3), t1 }));
  await outWait(boss, () => boss.click('#outFmt button[data-f="xlsx"]'));
  await outWait(boss, () => boss.click('#outLang button[data-l="pl"]'));
  const [dx] = await Promise.all([boss.waitForEvent('download', { timeout: 60000 }), boss.click('#outGet')]);
  const xl = zipNames(await dx.path());
  await z.sleep(800);
  const audit = z.rows(`SELECT details FROM admin_actions WHERE action = 'export' AND created_at >= '${T0}' ORDER BY created_at`);
  const per = `${dmy(PREV + '-01')} — ${dmy(lastDay(PREV))}`;
  z.check(`«Скачать» XLSX: «1448-eksport-${PREV}.xlsx» — книга Excel; обе выгрузки — в журнале: «период · формат · язык · операций»`,
    dx.suggestedFilename() === `1448-eksport-${PREV}.xlsx` && xl.includes('xl/workbook.xml') && audit.length === 2
    && audit[0].startsWith(`${per} · CSV · RU · операций: `) && audit[1].startsWith(`${per} · XLSX · PL · операций: `), JSON.stringify({ name: dx.suggestedFilename(), audit }));

  // ── «Файл» и «Реквизиты фирмы» ──
  const fc = await boss.evaluate(() => ({ why: document.querySelector('#outFileCard .why').textContent,
    sel: ['out_sep', 'out_dec', 'out_enc', 'out_date', 'out_kpkw'].map(id => { const e = document.getElementById(id); return e ? [e.value, [...e.options].map(o => o.textContent)] : null; }) }));
  z.check('«Файл»: по умолчанию «;», запятая, UTF-8 с BOM, ДД.ММ.ГГГГ, KP/KW с начала месяца; «уточните у своего бухгалтера»',
    fc.sel.every(Boolean) && fc.sel.map(x => x[0]).join() === '0,0,0,0,0' && fc.sel.map(x => x[1].length).join() === '3,2,2,2,3'
    && fc.sel[2][1][1].startsWith('Windows-1250') && fc.sel[4][1].join('|') === 'с начала месяца — KP 1/10/2026|с начала года — KP 1/2026|без номеров'
    && /уточните у своего бухгалтера/.test(fc.why), JSON.stringify(fc));
  const p0 = puts.length;
  await boss.selectOption('#out_sep', '1'); await boss.selectOption('#out_date', '1');
  const fh = await z.text(boss, '#outFileHint');
  const t2 = await z.toastAfter(boss, () => outWait(boss, () => boss.click('#outFileSave')), 'Настройки файла сохранены');
  const fp = puts.slice(p0);
  const fdb = z.sql("SELECT string_agg(key || '=' || value, ',' ORDER BY key) FROM settings WHERE key IN ('acc_csv_sep', 'acc_date_fmt')");
  z.check('«,» и ГГГГ-ММ-ДД: «Изменено: 2»; «Сохранить» — один PUT только с ними, в базе — 1 и 1, тост «Настройки файла сохранены»',
    fh === 'Изменено: 2' && fp.length === 1 && fp[0] === 'PUT /settings {"acc_csv_sep":1,"acc_date_fmt":1}' && fdb === 'acc_csv_sep=1,acc_date_fmt=1'
    && /Настройки файла сохранены/.test(t2 || ''), JSON.stringify({ fh, fp, fdb, t2 }));
  z.sql("DELETE FROM settings WHERE key IN ('acc_csv_sep', 'acc_date_fmt')");

  const R0 = (await z.api('GET', '/admin/export/requisites', undefined, 'boss')).data;
  const ph0 = await boss.evaluate(() => ['out_rname', 'out_raddr', 'out_rnip'].map(id => { const e = document.getElementById(id); return e ? [e.placeholder, e.value] : null; }));
  await boss.fill('#out_rname', MARK + ' Nowak Gry Sp. z o.o.');
  await boss.fill('#out_rnip', '111-222-33-33');
  const warn1 = await boss.evaluate(() => !document.getElementById('outNipWarn').classList.contains('hidden'));
  await boss.fill('#out_rnip', '111-222-33-32');
  const warn2 = await boss.evaluate(() => !document.getElementById('outNipWarn').classList.contains('hidden'));
  await boss.fill('#out_raddr', 'ul. Firmowa 2, 03-000 Warszawa');
  const rh = await z.text(boss, '#outReqHint'), p1 = puts.length;
  const t3 = await z.toastAfter(boss, () => boss.click('#outReqSave'), 'Реквизиты сохранены');
  await boss.waitForFunction(() => (document.getElementById('outReqHint') || {}).textContent === 'Правок нет', undefined, { timeout: 15000 }).catch(() => {});
  const R1 = (await z.api('GET', '/admin/export/requisites', undefined, 'boss')).data, rp = puts.slice(p1);
  const nipShown = await boss.evaluate(() => document.getElementById('out_rnip').value);
  z.check('«Реквизиты фирмы»: пустые — подсказки с названием и адресом клуба; NIP с несходящейся суммой — предупреждение, верный — без; «Изменено: 3»; '
    + '«Сохранить» — название, NIP цифрами и адрес на сервере, в поле NIP — с дефисами, тост «Реквизиты сохранены»',
    !!R0.club_name && ph0[0][0] === R0.club_name && ph0[1][0] === R0.club_address && ph0.every(x => x[1] === '') && warn1 && !warn2 && rh === 'Изменено: 3'
    && rp.length === 1 && R1.name === MARK + ' Nowak Gry Sp. z o.o.' && R1.nip === '1112223332' && R1.address === 'ul. Firmowa 2, 03-000 Warszawa'
    && nipShown === '111-222-33-32' && /Реквизиты сохранены/.test(t3 || ''), JSON.stringify({ ph0, R0, warn1, warn2, rh, rp, R1, nipShown, t3 }));

  // ── форма учёта в «Экономике» и первые листы ──
  await z.nav(boss, 'settings');
  await boss.waitForSelector('#set_accform', { timeout: 20000 }).catch(() => {});
  const sf = await boss.evaluate(() => {
    const sel = id => { const e = document.getElementById(id); return e ? { v: e.value, o: [...e.options].map(o => o.textContent), card: e.closest('.setcard').querySelector('h4').textContent } : null; };
    const card = document.getElementById('set_accform');
    return { form: sel('set_accform'), ry: sel('set_rytime'), first: card ? card.closest('.setcard').querySelector('select, input').id : '',
      note: ((document.getElementById('setFormNote') || {}).textContent || '').replace(/\s+/g, ' ').trim() };
  });
  const RR = (await z.api('GET', '/admin/ryczalt/rates', undefined, 'boss')).data.rates || [];
  z.check('«Экономика» → «Бухгалтерия»: первое поле — «Форма учёта»: KPiR (по умолчанию), Ryczałt, полная; «Ставка ryczałtu времени» — ставки сервера, '
    + '15 %; пояснение — «Записи для KPiR»', !!sf.form && sf.form.card === 'Бухгалтерия' && sf.first === 'set_accform' && sf.form.v === '0'
    && sf.form.o.join('|') === 'KPiR — księga przychodów i rozchodów|Ryczałt — ewidencja przychodów|Полная бухгалтерия — księgi rachunkowe'
    && !!sf.ry && sf.ry.card === 'Бухгалтерия' && sf.ry.v === '150' && RR.length === 10 && sf.ry.o.join('|') === RR.map(r => r.label).join('|')
    && /«Записи для KPiR»/.test(sf.note), JSON.stringify(sf));
  const p2 = puts.length;
  await boss.selectOption('#set_accform', '1');
  const note1 = await z.text(boss, '#setFormNote');
  await boss.click('#setSave');
  await boss.waitForFunction(() => (document.getElementById('setHint') || {}).textContent === 'Правок нет', undefined, { timeout: 15000 }).catch(() => {});
  const fput = puts.slice(p2);
  await outWait(boss, () => boss.evaluate(() => accGo('out')));
  const s6 = await screen(boss);
  put('acc_form', 2);
  await outWait(boss, () => boss.evaluate(() => renderOut()));
  const s7 = await screen(boss);
  z.sql("DELETE FROM settings WHERE key = 'acc_form'");
  z.check('«Ryczałt»: пояснение — доходы по ставкам ryczałtu; «Сохранить» — PUT {acc_form: 1}; в «Выгрузке» — профиль «ryczałt», время и пакеты — 15 %, '
    + 'вторым листом — «Ewidencja przychodów»; полная — «Rejestr sprzedaży VAT», «Rejestr zakupów VAT», «Raport kasowy» сразу после «O eksporcie»',
    /доходы по ставкам ryczałtu/.test(note1 || '') && fput.join(' ¶ ') === 'PUT /settings {"acc_form":1}' && /^Профиль учёта: ryczałt/.test(s6.prof)
    && /время и пакеты — 15 %/.test(s6.prof) && (s6.sheets[1] || [])[1] === 'Ewidencja przychodów' && /^Профиль учёта: полная бухгалтерия/.test(s7.prof)
    && s7.sheets.slice(1, 4).map(r => r[1]).join('|') === 'Rejestr sprzedaży VAT|Rejestr zakupów VAT|Raport kasowy'
    && s7.sheets.filter(r => r[1] === 'Raport kasowy').length === 1, JSON.stringify({ note1, fput, s6: [s6.prof, s6.sheets[1]], s7: s7.sheets.slice(0, 5).map(r => r[1]) }));

  // ── «Справочники»: колонка KPiR и счёт, ставки ryczałtu ──
  await boss.click('#accTabs button[data-t="refs"]');
  await boss.waitForSelector('#refRy', { timeout: 30000 }).catch(() => {});
  await z.sleep(500);
  const catRow = () => boss.evaluate(id => { const r = document.querySelector(`#refCats tr.refRow[data-id="${id}"]`);
    return r ? [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) : null; }, rent.id);
  const head = await boss.evaluate(() => [...document.querySelectorAll('#refCats tr:first-child th')].map(t => t.textContent).join('|'));
  const c0 = await catRow();
  await boss.click(`#refCats tr.refRow[data-id="${rent.id}"] button:has-text("Изменить")`);
  await boss.waitForSelector('#rcEditKpir', { timeout: 5000 }).catch(() => {});
  const ko = await boss.evaluate(() => { const e = document.getElementById('rcEditKpir'); return e ? [e.value, [...e.options].map(o => o.textContent).join('|')] : null; });
  await boss.selectOption('#rcEditKpir', '13'); await boss.fill('#rcEditAcct', '402-01');
  const p3 = puts.length;
  await boss.click('#rcSave');
  await boss.waitForFunction(id => { const r = document.querySelector(`#refCats tr.refRow[data-id="${id}"]`); return !!r && r.cells[4].innerText.includes('402-01'); },
    rent.id, { timeout: 15000 }).catch(() => {});
  const c1 = await catRow(), cp = puts.slice(p3);
  const cl = z.sql(`SELECT details FROM admin_actions WHERE action = 'expense_category_update' ORDER BY created_at DESC LIMIT 1`);
  z.check('«Справочники» → «Категории»: колонки «KPiR» и «Счёт», у «Аренды» — 15 и 402; в шторке — колонка (12, 13, 14, 15, «не в книгу») и счёт; '
    + '13 и 402-01 — один PATCH с ними, строка — 13 и 402-01, в журнале «Аренда: KPiR 15 → 13; счёт 402 → 402-01»',
    head === 'Категория|Какая|Закупка|KPiR|Счёт|' && !!c0 && c0[3] === '15' && c0[4] === '402' && !!ko && ko[0] === '15'
    && ko[1] === '12 — закупка товаров и материалов|13 — побочные затраты закупки|14 — зарплаты|15 — прочие расходы|не в книгу'
    && cp.length === 1 && /"kpir_col":13/.test(cp[0]) && /"account":"402-01"/.test(cp[0]) && !!c1 && c1[3] === '13' && c1[4] === '402-01'
    && cl === `${rent.name}: KPiR 15 → 13; счёт 402 → 402-01`, JSON.stringify({ head, c0, ko, cp, c1, cl }));
  const ryRow = () => boss.evaluate(m => { const r = [...document.querySelectorAll('#refRy tr.refRow')].find(x => x.cells[0].innerText.trim() === m);
    return r ? { i: r.dataset.i, cells: [...r.cells].slice(0, 3).map(c => c.innerText.replace(/\s+/g, ' ').trim()) } : null; }, MARK);
  const ryWait = re => boss.waitForFunction(([m, src]) => { const r = [...document.querySelectorAll('#refRy tr.refRow')].find(x => x.cells[0].innerText.trim() === m);
    return !!r && new RegExp(src).test(r.cells[2].innerText.trim()); }, [MARK, re.source], { timeout: 15000 }).catch(() => {});
  const note2 = await z.text(boss, '#refRyNote'), y0 = await ryRow();
  z.check('«Ставки ryczałtu»: форма — KPiR — «в выгрузку ставки не идут», время и пакеты — 15 %; «z13x» — 2 позиции по 3 %', !!y0 && y0.cells.join('|') === `${MARK}|2|3 %`
    && /сейчас — KPiR — книга доходов и расходов, в выгрузку ставки не идут/.test(note2 || '') && /Время за ПК и пакеты — 15 %/.test(note2 || ''), JSON.stringify({ y0, note2 }));
  if (!y0) return z.done();
  await boss.selectOption(`#ry_${y0.i}`, '85');
  const p4 = puts.length, btn = `#refRy tr.refRow[data-i="${y0.i}"] .ryPick button`;
  await boss.click(btn);
  await z.sleep(300);
  const armed = await boss.evaluate(s => (document.querySelector(s) || {}).textContent, btn), early = puts.length - p4;
  const t4 = await z.toastAfter(boss, () => boss.click(btn), 'поменяли у');
  await ryWait(/^8,5 %$/);
  const y1 = await ryRow();
  await boss.click(`#refRy tr.refRow[data-i="${y1.i}"] button:has-text("Позиции")`);
  await boss.waitForSelector('#ryList', { timeout: 5000 }).catch(() => {});
  const items = await boss.evaluate(() => [...document.querySelectorAll('#ryList .ryItem')].map(x => [x.querySelector('span').textContent.trim(), x.querySelector('select').value]));
  await boss.selectOption(`#ryList .ryItem[data-id="${gp}"] select`, '20');
  await ryWait(/разные/);
  const y2 = await ryRow(), rp2 = puts.slice(p4);
  const gs = z.rows(`SELECT name || '|' || ryczalt_rate FROM goods WHERE name LIKE '${MARK} %' ORDER BY name`).join();
  await z.shot(boss, 'z13_refs_ry');
  z.check('8,5 % «Всей категории» — взвод «Всем 2 — 8,5 %?», второй клик — PUT, тост «поменяли у 2 из 2», строка — 8,5 %; «Позиции» — шторка: обе по 8,5 %, '
    + 'пиву 2 % — PATCH позиции; строка — «разные: 8,5 % — 1, 2 % — 1», в базе — 85 и 20', armed === 'Всем 2 — 8,5 %?' && early === 0 && !!y1 && y1.cells[2] === '8,5 %'
    && /ryczałt 8,5 % — поменяли у 2 из 2/.test(t4 || '') && items.length === 2 && items.every(x => x[1] === '85')
    && rp2.join(' ¶ ') === `PUT /ryczalt/goods {"category":"${MARK}","ryczalt_rate":85} ¶ PATCH /goods/${gp} {"ryczalt_rate":20}`
    && !!y2 && y2.cells[2] === 'разные: 8,5 % — 1, 2 % — 1' && gs === `${MARK} Chipsy|85,${MARK} Piwo|20`, JSON.stringify({ armed, early, y1, t4, items, rp2, y2, gs }));
  await boss.evaluate(() => closeDrawer());

  // ── журнал и палитра ──
  await z.nav(boss, 'grants');
  await boss.waitForSelector('#auditTable tr', { timeout: 15000 }).catch(() => {});
  await z.sleep(500);
  const jr = await boss.evaluate(() => [...document.querySelectorAll('#auditTable tr')].slice(1, 80).map(r => r.cells[1].innerText.trim() + ' — ' + r.cells[3].innerText.trim()));
  const has = (label, re) => jr.some(x => x.startsWith(label + ' — ') && re.test(x));
  z.check('«Журнал» по-русски: «выгрузка бухгалтеру» с периодом и форматом, «реквизиты фирмы», «товары: ставка ryczałtu категории», правка категории — «KPiR 15 → 13»',
    has('выгрузка бухгалтеру', / · XLSX · PL · операций: /) && has('реквизиты фирмы', /NIP: — → 1112223332/)
    && has('товары: ставка ryczałtu категории', /категория «z13x»: ryczałt → 8,5 % \(2 позиции\)/) && jr.some(x => /: KPiR 15 → 13; счёт 402 → 402-01$/.test(x)),
    JSON.stringify(jr.slice(0, 12)));
  const pal = async (p, q) => { await p.evaluate(() => palOpen()); await p.fill('#palInp', q); await z.sleep(500);
    return p.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent)); };
  const pv1 = await pal(boss, 'выгрузка');
  await outWait(boss, () => boss.keyboard.press('Enter'));
  const at = await boss.evaluate(() => ({ tab: curTab, acc: accTab, on: (document.querySelector('#accTabs button.on') || {}).textContent }));
  const pv2 = await pal(boss, 'бухгалтер');
  await boss.keyboard.press('Escape');
  const pv3 = await pal(boss, 'kpir');
  await boss.keyboard.press('Escape');
  const pv4 = await pal(boss, 'форма учёта');
  await boss.keyboard.press('Enter');
  await z.sleep(2500);
  const af = await boss.evaluate(() => ({ tab: curTab, focus: (document.activeElement || {}).id }));
  z.check('палитра владельца: «выгрузка» → «Выгрузка бухгалтеру» первой, Enter — «Бухгалтерия» → «Выгрузка»; «бухгалтер» — она же; «kpir» — «Форма учёта» и '
    + '«Колонки KPiR и ставки ryczałtu»; «форма учёта» → Enter — «Экономика», фокус на «Форме учёта»',
    pv1[0] === 'Выгрузка бухгалтеру' && at.tab === 'acc' && at.acc === 'out' && at.on === 'Выгрузка' && pv2.includes('Выгрузка бухгалтеру')
    && pv3.includes('Форма учёта') && pv3.includes('Колонки KPiR и ставки ryczałtu') && pv4[0] === 'Форма учёта' && af.tab === 'settings' && af.focus === 'set_accform',
    JSON.stringify({ pv1, at, pv2, pv3, pv4, af }));

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const adm = await z.openAdmin('admin1');
  const ap = await pal(adm, 'выгрузка');
  await adm.keyboard.press('Escape');
  const ar = await adm.evaluate(() => document.getElementById('r-acc').classList.contains('hidden'));
  const as = (await z.api('GET', '/admin/export?preset=prev_month&preview=1', undefined, 'admin1')).status;
  z.check('админ: «Бухгалтерии» в меню нет, в палитре «Выгрузки бухгалтеру» нет, превью — 403', ar && !ap.includes('Выгрузка бухгалтеру') && as === 403,
    JSON.stringify({ ap, ar, as }));

  await adm.goto('about:blank');
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'acc');
  await outWait(ph, () => ph.click('#accTabs button[data-t="out"]'));
  const pc = await ph.evaluate(() => { const g = document.getElementById('outGet'); g.scrollIntoView({ block: 'center' });
    const r = g.getBoundingClientRect(), rows = [...document.querySelectorAll('#outSheets tr')].slice(1), W = innerWidth;
    return { sw: document.documentElement.scrollWidth, btn: r.left >= 0 && r.right <= W, fit: rows.every(x => x.getBoundingClientRect().right <= W), n: rows.length,
      cards: [...document.querySelectorAll('#outFileCard, #outReqCard')].every(x => x.getBoundingClientRect().right <= W) }; });
  await z.sleep(300);
  await z.shot(ph, 'z13_export_phone');
  await ph.click('#accTabs button[data-t="refs"]');
  await ph.waitForSelector('#refRy', { timeout: 30000 }).catch(() => {});
  await z.sleep(500);
  const pr = await ph.evaluate(() => { const h = document.getElementById('refRyHead'); if (h) h.scrollIntoView({ block: 'start' });
    return { sw: document.documentElement.scrollWidth, n: document.querySelectorAll('#refRy tr.refRow').length,
      fit: [...document.querySelectorAll('#refRy tr.refRow, #refCats tr.refRow')].every(x => x.getBoundingClientRect().right <= innerWidth) }; });
  await z.sleep(300);
  await z.shot(ph, 'z13_refs_ry_phone');
  z.check('телефон 360: «Выгрузка» и «Справочники» без прокрутки вбок; листы — карточками, «Скачать», «Файл» и «Реквизиты» — в ширину экрана',
    pc.sw <= 360 && pc.btn && pc.fit && pc.cards && pc.n === (P0.sheets || []).length && pr.sw <= 360 && pr.fit && pr.n > 0, JSON.stringify({ pc, pr }));
  await z.done();
})().catch(z.crash);
