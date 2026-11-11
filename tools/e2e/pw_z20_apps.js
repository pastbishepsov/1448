// З20-и3 «Каталог показывает» (№57 «Где не установлено»), браузер. По последним снимкам агентов (pc_app_status — здесь
// SQL-ом) GET /admin/catalog отдаёт install — где запись не запустится, GET /admin/computers у ПК — apps_off и apps_at.
// Каталог: у записи бейдж «⚠ нет на z20c-1, z20c-3» / «⚠ путь недостижим на всех (3)», «checking» и «всё на месте» — без
// бейджа; фильтр «Проблемные · N» оставляет только такие записи. Шторка ПК — «Не установлено здесь» с причинами и
// временем снимка; у ПК без снимка — ничего. Видит и админ, не только владелец. Гостей нет: временные ПК z20c-1…3 и
// записи каталога z20c_a, z20c_b, z20c_c, z20c_ok удаляются в начале и в конце (строки снимков — с ними).
const z = require('./_z');
(async () => {
  const wipe = () => z.sql("DELETE FROM computers WHERE name LIKE 'z20c-%'; "
    + "DELETE FROM catalog_apps WHERE id IN ('z20c_a', 'z20c_b', 'z20c_c', 'z20c_ok')");
  wipe();
  z.atEnd(wipe);
  const pcs = [];
  for (const n of ['z20c-1', 'z20c-2', 'z20c-3']) pcs.push(((await z.api('POST', '/admin/computers', { name: n }, 'boss')).data || {}).computer_id);
  const cat = [];
  for (const [id, name] of [['z20c_a', 'z20c A'], ['z20c_b', 'z20c B'], ['z20c_c', 'z20c C'], ['z20c_ok', 'z20c OK']])
    cat.push((await z.api('POST', '/admin/catalog', { id, name, category: 'game', target: 'D:\\z20c\\' + id + '.exe' }, 'boss')).status);
  const hasTable = z.sql("SELECT to_regclass('pc_app_status') IS NOT NULL") === 't';
  if (hasTable) {
    const v = [];
    pcs.forEach((pc, k) => {
      v.push(`('${pc}', 'z20c_a', ${k === 1}, '${k === 1 ? '' : 'target_missing'}')`);
      v.push(`('${pc}', 'z20c_b', false, 'path_unreachable')`, `('${pc}', 'z20c_c', true, 'checking')`, `('${pc}', 'z20c_ok', true, '')`);
    });
    z.sql('INSERT INTO pc_app_status (computer_id, app_id, available, reason) VALUES ' + v.join(', '));
  }
  z.check('ПК z20c-1…3 и записи каталога z20c_a, z20c_b, z20c_c, z20c_ok заведены, снимки агентов — SQL-ом',
    pcs.every(Boolean) && cat.every(s => s === 200 || s === 201) && hasTable, JSON.stringify([pcs, cat, hasTable]));

  const ca = (await z.api('GET', '/admin/catalog', undefined, 'boss')).data || {};
  const ins = ca.install || {};
  z.check('GET /admin/catalog: install — z20c_a: из 3 ПК нет на z20c-1 и z20c-3; z20c_b: путь недостижим на всех 3; '
    + 'z20c_ok: 3 ПК, всё на месте; z20c_c (checking) — не в счёт',
    JSON.stringify(ins.z20c_a) === JSON.stringify({ reported: 3, missing: ['z20c-1', 'z20c-3'], unreachable: [] })
    && JSON.stringify(ins.z20c_b) === JSON.stringify({ reported: 3, missing: [], unreachable: ['z20c-1', 'z20c-2', 'z20c-3'] })
    && JSON.stringify(ins.z20c_ok) === JSON.stringify({ reported: 3, missing: [], unreachable: [] }) && !('z20c_c' in ins),
    JSON.stringify([ins.z20c_a, ins.z20c_b, ins.z20c_ok, ins.z20c_c]));
  const cl = ((await z.api('GET', '/admin/computers', undefined, 'admin1')).data || {}).computers || [];
  const P = n => cl.find(p => p.name === n) || {};
  const offs = n => (P(n).apps_off || []).map(a => `${a.name}:${a.reason}`).join(', ');
  z.check('GET /admin/computers: z20c-1 — apps_off «z20c A: target_missing, z20c B: path_unreachable», есть apps_at; '
    + 'z20c-2 — только z20c B; у PC-01 без снимка — ни apps_at, ни apps_off',
    offs('z20c-1') === 'z20c A:target_missing, z20c B:path_unreachable' && !!P('z20c-1').apps_at
    && offs('z20c-2') === 'z20c B:path_unreachable' && !('apps_at' in P('PC-01')) && !('apps_off' in P('PC-01')),
    JSON.stringify([offs('z20c-1'), P('z20c-1').apps_at, offs('z20c-2'), Object.keys(P('PC-01')).filter(k => /^apps/.test(k))]));

  const row = (page, id) => page.evaluate(id => {
    const tr = [...document.querySelectorAll('#catalogTable tr')].find(r => [...r.cells].some(c => c.textContent.trim() === id));
    const b = tr && tr.querySelector('.instbad');
    return tr ? { badge: b ? b.textContent.trim() : '', title: b ? b.title : '' } : null;
  }, id);
  for (const who of ['boss', 'admin1']) {
    const page = await z.openAdmin(who);
    await z.nav(page, 'catalog');
    const r = { a: await row(page, 'z20c_a'), b: await row(page, 'z20c_b'), c: await row(page, 'z20c_c'), ok: await row(page, 'z20c_ok') };
    z.check(`${who}: каталог — у z20c_a «⚠ нет на z20c-1, z20c-3», у z20c_b «⚠ путь недостижим на всех (3)», у z20c_c и z20c_ok `
      + 'бейджа нет; в подсказке — список ПК и «по снимкам агентов: 3 ПК»',
      !!r.a && r.a.badge === '⚠ нет на z20c-1, z20c-3' && !!r.b && r.b.badge === '⚠ путь недостижим на всех (3)'
      && !!r.c && r.c.badge === '' && !!r.ok && r.ok.badge === '' && /Файла нет: z20c-1, z20c-3/.test(r.a.title)
      && /По снимкам агентов: 3 ПК/.test(r.a.title), JSON.stringify(r));
    if (who === 'boss') {
      await z.shot(page, 'z20_apps_catalog');
      const hasFilter = !!(await page.$('#catFilter button[data-f="bad"]')); // на HEAD фильтра нет — не ждать 30 с
      if (hasFilter) { await page.click('#catFilter button[data-f="bad"]'); await z.sleep(1200); }
      const f = await page.evaluate(() => ({ label: (document.querySelector('#catFilter button[data-f="bad"]') || {}).textContent || '',
        ids: [...document.querySelectorAll('#catalogTable tr')].filter(r => r.querySelector('td')).map(r => r.cells[1].textContent.trim()),
        badges: document.querySelectorAll('#catalogTable .instbad').length }));
      z.check('фильтр «Проблемные · N» — в таблице только записи с бейджем: z20c_a и z20c_b есть, z20c_c и z20c_ok — нет',
        /^Проблемные · \d+$/.test(f.label) && f.ids.includes('z20c_a') && f.ids.includes('z20c_b') && !f.ids.includes('z20c_c')
        && !f.ids.includes('z20c_ok') && f.badges === f.ids.length && +f.label.split('· ')[1] === f.ids.length, JSON.stringify(f));
      if (hasFilter) { await page.click('#catFilter button[data-f=""]'); await z.sleep(1200); }
      const all = await page.evaluate(() => [...document.querySelectorAll('#catalogTable tr')].length);
      z.check('«Все» — снова весь каталог (z20c_c и z20c_ok на месте)', (await row(page, 'z20c_c')) !== null && (await row(page, 'z20c_ok')) !== null,
        JSON.stringify({ all }));
    }
    await z.nav(page, 'hall');
    const drawer = async name => {
      await page.evaluate(n => { const p = hallData.find(x => x.name === n); if (p) openPCDrawer(p.id); }, name);
      await z.sleep(500);
      return page.evaluate(() => { const e = document.getElementById('pcAppsOff'); const d = document.querySelector('.drawer.open, #drawer');
        return { off: e ? e.innerText.replace(/\s+/g, ' ').trim() : null, sub: !!d && /Не установлено здесь/.test(d.textContent), // .dsub — капсом
          at: !!d && /по снимку агента \d\d\.\d\d \d\d:\d\d/.test(d.innerText) }; });
    };
    const d1 = await drawer('z20c-1'), d3 = await drawer('PC-01');
    if (who === 'boss') { await drawer('z20c-1'); await z.shot(page, 'z20_apps_drawer'); }
    z.check(`${who}: шторка z20c-1 — «Не установлено здесь»: «z20c A — файла нет», «z20c B — путь недостижим», «по снимку агента `
      + 'ДД.ММ ЧЧ:ММ»; у PC-01 без снимка — ничего',
      !!d1.off && d1.off === 'z20c A — файла нет z20c B — путь недостижим' && d1.sub && d1.at && d3.off === null && !d3.sub,
      JSON.stringify([d1, d3]));
  }
  await z.done();
})().catch(z.crash);
