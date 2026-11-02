// З17-и1 «Зоны и ПК» (№7, З-Р6), браузер: «Отчёты → Загрузка» за пустые клубные сутки в прошлом с сессиями Guest47 (SQL,
// без денег, ended_reason z17p) — 6 ч на VIP-03 и 2 ч на PC-07; VIP-02 на время — в ремонте (API). В «Зонах» — все ПК
// зоны и «в ремонте 1», занятость «12,5%» (6 ч на двух ПК из трёх); в «Компьютерах» — все ПК, без сессий — с нулями, у
// VIP-02 — «в ремонте»; в подсказке «Занятости» — ремонт и что он не в ёмкости. В конце сессии удаляются, VIP-02 — снова
// свободен (и при падении тоже).
const z = require('./_z');
(async () => {
  const G = 'Guest47', g = z.uid(G);
  const drop = () => z.sql(`DELETE FROM sessions WHERE ended_reason = 'z17p' AND user_id = '${g}'`);
  drop();
  z.atEnd(drop);
  const vip2 = z.sql("SELECT id FROM computers WHERE name = 'VIP-02'");
  const repair = async st => (await z.api('PATCH', `/admin/computers/${vip2}/status`, { status: st }, 'admin1')).status;
  z.atEnd(() => repair('available'));
  const nAll = +z.sql('SELECT count(*) FROM computers');
  const rh = +z.sql("SELECT COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '8')");
  const D = z.sql(`SELECT g::date::text FROM generate_series(((now() AT TIME ZONE 'Europe/Warsaw')::date - 400)::timestamp,
    ((now() AT TIME ZONE 'Europe/Warsaw')::date - 3)::timestamp, interval '1 day') AS g WHERE NOT EXISTS (SELECT 1 FROM sessions
    WHERE started_at >= ((g + interval '${rh} hours') AT TIME ZONE 'Europe/Warsaw')
      AND started_at < ((g + interval '1 day ${rh} hours') AT TIME ZONE 'Europe/Warsaw')) ORDER BY g DESC LIMIT 1`);
  const at = hm => `(('${D}'::date + time '${hm}') AT TIME ZONE 'Europe/Warsaw')`;
  const ins = (pc, frm, to, mins) => z.sql(`INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ready_at,
    ended_at, minutes_total, minutes_used, billed_minutes, base_rate_pln, effective_rate_pln, ended_reason, zone_id, zone_name,
    xp_per_min, coins_per_min) SELECT '${g}', c.id, c.club_id, 'completed', ${at(frm)}, ${at(frm)}, ${at(to)}, ${mins}, ${mins},
    ${mins}, 0, 0, 'z17p', c.zone_id, COALESCE(c.zone, ''), 10, 2 FROM computers c WHERE c.name = '${pc}' RETURNING id`);
  ins('VIP-03', '10:00', '16:00', 360);
  ins('PC-07', '18:00', '20:00', 120);
  const st = await repair('maintenance');
  z.check(`сутки ${D}: 6 ч на VIP-03 и 2 ч на PC-07 (SQL); VIP-02 в ремонте — 200`, !!D && st === 200, `${D} ${st}`);

  const page = await z.openAdmin('boss');
  await z.nav(page, 'stats');
  await page.click('#repTabs button[data-t="load"]');
  await z.sleep(800);
  await page.evaluate(d => { repPeriod = { from: d, to: d }; renderReports(); }, D);
  await page.waitForFunction(() => /Компьютеры/i.test((document.getElementById('repBody') || {}).textContent || ''), null,
    { timeout: 10000 }).catch(() => {});
  await z.sleep(800);
  const t = await page.evaluate(() => {
    const clean = s => s.replace(/\s+/g, ' ').trim();
    const tables = [...document.querySelectorAll('#repBody table')];
    const rowsOf = tb => tb ? [...tb.querySelectorAll('tr')].slice(1).map(tr => [...tr.querySelectorAll('td')].map(td => clean(td.textContent))) : [];
    const kpi = [...document.querySelectorAll('#repBody .kpi')].find(k => /Занятость/.test(k.textContent));
    return { zones: rowsOf(tables[tables.length - 2]), pcs: rowsOf(tables[tables.length - 1]),
      kpi: kpi ? { text: clean(kpi.textContent), title: kpi.title } : null,
      sect: [...document.querySelectorAll('#repBody .sect')].map(s => clean(s.textContent)).find(s => /^Зоны/i.test(s)) || '' };
  });
  const row = (rows, name) => JSON.stringify(rows.find(r => r[0] && r[0].startsWith(name)) || null);
  z.check('«Зоны»: VIP — «3 · в ремонте 1», 6 ч, 1 сессия, «12,5%»; Standard — «9», 2 ч, «0,9%»; под заголовком — от каких ПК',
    row(t.zones, 'VIP') === JSON.stringify(['VIP', '3 · в ремонте 1', '6 ч', '1', '12,5%'])
    && row(t.zones, 'Standard') === JSON.stringify(['Standard', '9', '2 ч', '1', '0,9%'])
    && /от всех ПК зоны, кроме тех, что в ремонте сейчас/.test(t.sect), `${JSON.stringify(t.zones)} · ${t.sect}`);
  const zeros = t.pcs.filter(r => r[2] === '0 ч' && r[3] === '0' && r[4] === '0%').length;
  z.check(`«Компьютеры»: все ${nAll} ПК — VIP-03 «6 ч · 25%», PC-07 «2 ч · 8,3%», остальные — нули; у VIP-02 «в ремонте»`,
    t.pcs.length === nAll && row(t.pcs, 'VIP-03') === JSON.stringify(['VIP-03', 'VIP', '6 ч', '1', '25%'])
    && row(t.pcs, 'PC-07') === JSON.stringify(['PC-07', 'Standard', '2 ч', '1', '8,3%']) && zeros === nAll - 2
    && row(t.pcs, 'VIP-02') === JSON.stringify(['VIP-02 · в ремонте', 'VIP', '0 ч', '0', '0%']), JSON.stringify(t.pcs));
  z.check(`«Занятость» — «3%» (8 ч на ${nAll - 1} ПК), в подсказке — «в ремонте сейчас 1 ПК — в ёмкость не входят»`,
    t.kpi && t.kpi.text.startsWith('Занятость 3%') && t.kpi.title.includes(`${nAll - 1} ПК × 24 ч в сутки`)
    && t.kpi.title.includes('в ремонте сейчас 1 ПК — в ёмкость не входят'), JSON.stringify(t.kpi));
  await page.locator('#repBody .sect').last().scrollIntoViewIfNeeded();
  await z.shot(page, 'z17_zones');
  await z.done();
})().catch(z.crash);
