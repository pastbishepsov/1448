// З4-и2 (№5), браузер: пакет 50 zł налом виден в составе выручки — в подсказке столбика «Выручка по дням»,
// в «Кто оформлял» (колонка «Пакетов»), в «Персонал → по сменам» (колонка «Пакеты») и «по людям», в KPI
// «Из них пакеты» и в сводке смены (KPI «Пакеты», состав в подсказке «Выручки»). Временный z4packw продаёт
// Guest35 «3 часа Standard»; пакет отменяется, z4packw уволен.
const z = require('./_z');
(async () => {
  await z.hireTemp('z4packw');
  const pack = z.sql("SELECT id FROM time_packages WHERE active AND price_pln = 50 ORDER BY name LIMIT 1");
  const r = await z.api('POST', `/admin/users/${z.uid('Guest35')}/packages`, { package_id: pack, method: 'cash' }, 'z4packw');
  const pid = (r.data.package || {}).id;
  if (pid) z.atEnd(() => z.api('POST', `/admin/user-packages/${pid}/void`, {}, 'boss'));
  z.check('z4packw продал пакет 50 zł налом → 201', r.status === 201 && pid, String(r.status));

  const page = await z.openAdmin('boss');
  await z.nav(page, 'stats'); // «Деньги» за 7 дней: сегодня — последний столбик
  const bar = await page.evaluate(() => { const b = [...document.querySelectorAll('#repBody .bars .bar')].pop(); return b ? b.title : null; });
  z.check('подсказка столбика сегодня: «… · пакеты 50,00 zł»', /пополнения .* · товары .* · пакеты 50,00 zł/.test(bar || ''), bar);
  const row = nick => page.evaluate(n => {
    const tr = [...document.querySelectorAll('#repBody tr')].find(t => t.cells[0] && t.cells[0].textContent.trim().startsWith(n));
    return tr ? [...tr.cells].map(c => ({ t: c.textContent.replace(/\s+/g, ' ').trim(), title: c.title || '' })) : null;
  }, nick);
  const adm = await row('z4packw');
  z.check('«Кто оформлял»: z4packw — 50,00 zł, «Пакетов» 1 · 50,00 zł, в подсказке состав', adm && adm[1].t === '50,00 zł'
    && adm[4].t === '1 · 50,00 zł' && /пакеты 50,00 zł/.test(adm[1].title), JSON.stringify(adm));
  await page.locator('#repBody table').last().scrollIntoViewIfNeeded(); // «Кто оформлял»
  await z.shot(page, 'z4_revenue_money');

  await page.click('#repTabs button[data-t="staff"]');
  await z.sleep(1500);
  const kpi = await z.text(page, '#repBody .kpis');
  z.check('KPI «Из них пакеты 50,00 zł»', /Из них пакеты 50,00 zł/.test(kpi || ''), kpi);
  const shifts = await page.evaluate(() => [...document.querySelectorAll('#repBody table')][0].rows[0].textContent);
  const packsCell = await page.evaluate(() => [...[...document.querySelectorAll('#repBody table')][0].rows].slice(1)
    .map(tr => tr.cells[4].textContent.trim()).join(' | '));
  z.check('«по сменам»: колонка «Пакеты», в одной из смен 50,00 zł · 1', /Пакеты/.test(shifts) && /50,00 zł · 1/.test(packsCell), packsCell);
  await page.locator('#repBody table').first().scrollIntoViewIfNeeded();
  await z.shot(page, 'z4_revenue_staff');
  const per = await row('z4packw');
  z.check('«по людям»: z4packw — 50,00 zł, «0 · 0 · 1», в подсказке состав', per && per[3].t === '50,00 zł' && per[4].t === '0 · 0 · 1'
    && /пакеты 50,00 zł/.test(per[3].title), JSON.stringify(per));

  const sh = await page.evaluate(() => { const k = document.getElementById('shiftKpis');
    return k ? { text: k.textContent.replace(/\s+/g, ' ').trim(), title: k.querySelector('.kpi').title } : null; });
  z.check('сводка смены: KPI «Пакеты 50,00 zł», в подсказке «Выручки» — пакеты 50,00 zł', sh && /Пакеты 50,00 zł/.test(sh.text)
    && /пакеты 50,00 zł/.test(sh.title), JSON.stringify(sh));
  await page.locator('#shiftKpis').scrollIntoViewIfNeeded();
  await z.shot(page, 'z4_revenue_shift');
  await z.done();
})().catch(z.crash);
