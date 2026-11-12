// З18-и2 «Каталог: дифф команд» (№33 «а»; из №40 — корень игр и лимит импорта), браузер: «Каталог → Импорт JSON».
// Временная запись z18cat_p (steam://rungameid/9, API), корень игр клуба пуст. Файл экспорта с корнем E:\z18pw — через
// «Взять из файла»: z18cat_p → C:\Z18\p<прогон>.exe (свой у прогона — строка журнала только его), новая z18cat_q —
// {GAMES}\Q\q.exe. Превью: «команды запуска меняются у 1 · ⚠ путь на диске — у 1»; у z18cat_p — «target
// steam://rungameid/9 → C:\Z18\p….exe ⚠ путь на диске» (оранжевым), у z18cat_q — «target {GAMES}\Q\q.exe» без пометки;
// корень — «у клуба он не задан», галочка снята. Галочка → «Сохранить 2 записей» — один тост «Импорт: … · корень игр:
// E:\z18pw», в базе корень и записи; в «Журнале» — «z18cat_p: target «steam://rungameid/9» → «C:\Z18\p….exe»». Тот же
// файл ещё раз — «такой же, как у клуба», без галочки и без строк «было → стало»; неверный корень — оранжевая плашка «…
// Импорт корень не тронет». Больше 1 МБ — «Импорт — до 1 МБ за раз …» без запроса на сервер; файл 5 МБ — «Файл 5,0 МБ
// …», не читается. Телефон 360 — превью в ширину экрана. В конце z18cat_* удаляются, корень игр — как был (SQL-ом).
const z = require('./_z');
(async () => {
  const IDS = ['z18cat_p', 'z18cat_q'];
  const RUN = String(Date.now()).slice(-6), PEXE = `C:\\Z18\\p${RUN}.exe`; // свой путь у прогона — строка журнала только этого прогона
  const ROOT0 = z.rows("SELECT 'v:' || value FROM settings WHERE key = 'games_root'"); // [] — строки нет, ['v:'] — пустая
  const tidy = () => z.sql(`DELETE FROM catalog_apps WHERE id IN (${IDS.map(i => `'${i}'`).join(', ')})`);
  tidy();
  z.atEnd(tidy);
  z.atEnd(() => {
    z.sql("DELETE FROM settings WHERE key = 'games_root'");
    if (ROOT0.length) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('games_root', '${ROOT0[0].slice(2).replace(/'/g, "''")}', now())`);
  });
  z.sql("DELETE FROM settings WHERE key = 'games_root'");
  z.sql("INSERT INTO settings (key, value, updated_at) VALUES ('games_root', '', now())");
  const root = () => z.sql("SELECT value FROM settings WHERE key = 'games_root'");
  const app = i => z.sql(`SELECT coalesce(target, '∅') FROM catalog_apps WHERE id = '${i}'`);
  const s0 = (await z.api('POST', '/admin/catalog', { id: 'z18cat_p', name: 'Z18 P', category: 'game', target: 'steam://rungameid/9' }, 'boss')).status;
  z.check('z18cat_p — steam://rungameid/9, корень игр клуба пуст (API, SQL)', s0 === 200 && app('z18cat_p') === 'steam://rungameid/9' && root() === '', `${s0}`);

  const FILE = { format: '1448-catalog', games_root: 'E:\\z18pw', apps: [
    { id: 'z18cat_p', name: 'Z18 P', category: 'game', target: PEXE },
    { id: 'z18cat_q', name: 'Z18 Q', category: 'game', target: '{GAMES}\\Q\\q.exe' }] };
  const page = await z.openAdmin('boss');
  let sent = 0;
  page.on('request', r => { if (r.url().includes('/admin/catalog/import')) sent++; });
  await z.nav(page, 'catalog');
  const openImp = async () => { await page.click('button:has-text("Импорт JSON")'); await page.waitForSelector('#impText', { timeout: 5000 }); };
  const fileInput = '#drawerBody input[type=file]';
  const pick = (name, buffer) => page.setInputFiles(fileInput, { name, mimeType: 'application/json', buffer });
  const prev = () => page.evaluate(() => { const r = document.getElementById('impResult'); if (!r) return null;
    const t = e => e ? e.innerText.replace(/\s+/g, ' ').trim() : null;
    const disk = r.querySelector('.impDiff .impDisk'), cb = document.getElementById('impRoot'), rt = r.querySelector('.impRoot');
    return { sum: t(r.firstElementChild), rows: [...r.querySelectorAll('.impRow')].map(x => ({ head: t(x.querySelector('.impHead')),
      diff: [...x.querySelectorAll('.impDiff')].map(t) })), disk: disk ? getComputedStyle(disk).color : null,
      cb: cb ? cb.checked : null, root: t(rt), warn: !!(rt && rt.classList.contains('setwarn')), text: t(r) }; });
  const settle = () => page.waitForFunction(() => { const r = document.getElementById('impResult'); return !!r && r.textContent.trim() !== ''; },
    null, { timeout: 8000 }).catch(() => {});

  await openImp();
  await pick('catalog-1448.json', Buffer.from(JSON.stringify(FILE, null, 2)));
  await settle();
  await z.sleep(400);
  const p1 = await prev();
  z.check('«Взять из файла» — сразу превью: «команды запуска меняются у 1 · ⚠ путь на диске — у 1»',
    !!p1 && /Всего 2: новых 1, обновится 1 · команды запуска меняются у 1 · ⚠ путь на диске — у 1$/.test(p1.sum || ''), JSON.stringify(p1 && p1.sum));
  z.check('z18cat_p — «target steam://rungameid/9 → C:\\Z18\\p….exe ⚠ путь на диске», пометка оранжевым; z18cat_q (новая) — '
    + 'только что будет, {GAMES} без пометки', !!p1 && JSON.stringify(p1.rows) === JSON.stringify([
      { head: 'z18cat_p Z18 P → target', diff: [`target steam://rungameid/9 → ${PEXE} ⚠ путь на диске`] },
      { head: 'z18cat_q Z18 Q новая', diff: ['target {GAMES}\\Q\\q.exe'] }]) && p1.disk === 'rgb(251, 146, 60)', JSON.stringify(p1 && [p1.rows, p1.disk]));
  z.check('корень из файла: «Корень игр в файле — E:\\z18pw, у клуба он не задан …», галочка снята',
    !!p1 && p1.cb === false && (p1.root || '').startsWith('Корень игр в файле — E:\\z18pw, у клуба он не задан: записи с {GAMES} сейчас не запустятся.'),
    JSON.stringify(p1 && [p1.cb, p1.root]));
  await z.shot(page, 'z18_catalog_preview');

  if (await page.$('#impRoot')) await page.check('#impRoot');
  const toast = await z.toastAfter(page, async () => {
    if (await page.$('#impResult button:has-text("Сохранить")')) await page.click('#impResult button:has-text("Сохранить")'); }, 'Импорт:'); // З19-и13: тост этого действия, а не #toast через паузу
  z.check('галочка и «Сохранить 2 записей» — один тост «Импорт: 2 записей (+1 новых, 1 обновлено) · корень игр: E:\\z18pw»; '
    + 'в базе корень и обе команды', toast === 'Импорт: 2 записей (+1 новых, 1 обновлено) · корень игр: E:\\z18pw'
    && root() === 'E:\\z18pw' && app('z18cat_p') === PEXE && app('z18cat_q') === '{GAMES}\\Q\\q.exe'
    && await page.evaluate('catGamesRoot') === 'E:\\z18pw', `${toast} · ${root()} · ${app('z18cat_p')} · ${app('z18cat_q')}`);

  await z.nav(page, 'grants');
  const LINE = `z18cat_p: target «steam://rungameid/9» → «${PEXE}»`;
  await page.waitForFunction(t => ((document.getElementById('auditTable') || {}).innerText || '').includes(t), LINE, { timeout: 8000 }).catch(() => {});
  const jr = await page.evaluate(t => { const r = [...document.querySelectorAll('#auditTable tr')].find(x => x.innerText.includes(t));
    return r ? [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) : null; }, LINE);
  z.check('«Журнал»: «каталог: импорт» — «z18cat_p: target «steam://rungameid/9» → «C:\\Z18\\p….exe»» этого прогона',
    !!jr && jr[1] === 'каталог: импорт' && jr[3] === LINE, JSON.stringify(jr));

  await z.nav(page, 'catalog');
  await openImp();
  await page.fill('#impText', JSON.stringify(FILE));
  await page.click('#drawerBody button:has-text("Проверить")');
  await settle();
  await z.sleep(400);
  const p2 = await prev();
  z.check('тот же файл ещё раз — «Корень игр в файле — E:\\z18pw, такой же, как у клуба», галочки нет, строк «было → стало» нет',
    !!p2 && p2.root === 'Корень игр в файле — E:\\z18pw, такой же, как у клуба.' && p2.cb === null && p2.rows.length === 2
    && p2.rows.every(x => x.diff.length === 0 && / без изменений$/.test(x.head)), JSON.stringify(p2 && [p2.root, p2.rows]));
  await page.fill('#impText', JSON.stringify({ ...FILE, games_root: 'games' }));
  await page.click('#drawerBody button:has-text("Проверить")');
  await z.sleep(900);
  const p3 = await prev();
  z.check('неверный корень в файле — оранжевая плашка «⚠ Корень игр: «G:\\Games», … — а в файле games. Импорт корень не тронет.»',
    !!p3 && p3.warn && p3.root === '⚠ Корень игр: «G:\\Games», «\\\\nas\\games» или «/mnt/games» — а в файле games. Импорт корень не тронет.'
    && p3.cb === null, JSON.stringify(p3 && [p3.warn, p3.root]));

  const n0 = sent;
  await page.evaluate(() => { document.getElementById('impText').value = JSON.stringify({ apps: [{ id: 'z18cat_big', name: 'x'.repeat(1153434), category: 'game' }] }); });
  await page.click('#drawerBody button:has-text("Проверить")');
  await z.sleep(600);
  const big = await z.text(page, '#impResult');
  await pick('big.json', Buffer.alloc(5 << 20, 32));
  await z.sleep(600);
  const huge = await z.text(page, '#impResult'), len = await page.evaluate(() => document.getElementById('impText').value.length);
  z.check('больше 1 МБ — «⚠ Импорт — до 1 МБ за раз, а здесь 1,1 МБ: раздели файл на части», на сервер не ушло; файл 5 МБ — '
    + '«⚠ Файл 5,0 МБ — импорт берёт до 1 МБ за раз …», не читается', big === '⚠ Импорт — до 1 МБ за раз, а здесь 1,1 МБ: раздели файл на части'
    && huge === '⚠ Файл 5,0 МБ — импорт берёт до 1 МБ за раз: раздели файл на части' && sent === n0 && len < (2 << 20),
    JSON.stringify({ big, huge, sent: sent - n0, len }));

  // телефон: превью со строками «было → стало» в ширину экрана
  await z.sleep(2500); // лимитер входа с одного адреса
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 800 } });
  await z.nav(ph, 'catalog');
  await ph.evaluate(f => { openCatalogImport(); document.getElementById('impText').value = JSON.stringify(f); impPreview(); },
    { games_root: '\\\\nas-z18\\very-long-share-name\\games', apps: [{ id: 'z18cat_p', target: '{GAMES}\\Some Very Long Folder Name\\bin\\win64\\p.exe',
      args: ['--launch-product=z18_product_with_long_name', '--dir=D:\\Games\\Z18'], cover_url: 'https://example.com/covers/z18cat_p-cover-art-large.jpg' }] });
  await ph.waitForFunction(() => document.querySelectorAll('#impResult .impDiff').length > 0, null, { timeout: 8000 }).catch(() => {});
  await z.sleep(800); // шторка ещё выезжает
  const fit = await ph.evaluate(() => { const b = document.getElementById('drawerBody');
    const els = [...document.querySelectorAll('#impResult .impDiff, #impResult .impRoot, #impResult .impHead')];
    return { n: els.length, ok: els.every(e => e.getBoundingClientRect().right <= innerWidth + 0.5 && e.getBoundingClientRect().left >= 0),
      over: b.scrollWidth - b.clientWidth }; });
  await z.shot(ph, 'z18_catalog_phone');
  z.check('телефон 360: строки «было → стало», корень игр и заголовки записей — в ширину экрана, вбок не прокручивается',
    fit.n >= 5 && fit.ok && fit.over <= 0, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
