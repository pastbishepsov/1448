// З5-и2 (№10), браузер: отчёт «Монеты» — у отменённой выдачи видно, сколько монет вернулось («400 — отменена,
// вернулось 200»), «Должны временем» — и в минутах, и в злотых, у «Погашено» — подсказка «за вычетом
// вернувшегося». Guest37: выдача 60 минут Standard → 30 минут игры за PC-06 → отмена. Сессия удаляется.
const z = require('./_z');
(async () => {
  const g = z.uid('Guest37'), std = z.sql("SELECT id FROM zones WHERE name='Standard'");
  z.sql(`UPDATE users SET coins_balance = GREATEST(coins_balance, 1000) WHERE id='${g}'`);
  const r = await z.api('POST', `/admin/users/${g}/redeem`, { zone_id: std, minutes: 60 }, 'admin1');
  const pc = z.sql("SELECT id FROM computers WHERE name='PC-06'");
  const s = await z.api('POST', `/admin/computers/${pc}/session`, { guest: 'Guest37' }, 'admin1');
  const sid = s.data.session_id;
  if (sid) z.atEnd(() => z.sql(`DELETE FROM sessions WHERE id='${sid}'`));
  z.sql(`UPDATE sessions SET started_at = now() - interval '29 minutes 10 seconds', ready_at = now() - interval '29 minutes 10 seconds' WHERE id='${sid}'`);
  await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  const v = await z.api('POST', `/admin/coin-redemptions/${r.data.id}/void`, {}, 'admin1');
  z.check('выдача 60 мин → игра 30 мин → отмена: вернулось 200 монет', r.status === 200 && v.status === 200 && v.data.coins_returned === 200,
    `${r.status} ${v.status} ${JSON.stringify(v.data)}`);

  const page = await z.openAdmin('boss');
  await z.nav(page, 'stats');
  await page.click('#repTabs button[data-t="coins"]');
  await z.sleep(1500);
  const k = await page.evaluate(() => {
    const kpi = re => [...document.querySelectorAll('#repBody .kpi')].find(e => re.test(e.textContent.trim()));
    const owed = kpi(/^Должны временем/), red = kpi(/^Погашено за период/);
    return { owed: owed ? owed.textContent.replace(/\s+/g, ' ').trim() : null, red: red ? red.title : null };
  });
  z.check('«Должны временем» — минуты и злотые', /^Должны временем .+ · [\d\s]+,\d\d zł/.test(k.owed || ''), k.owed);
  z.check('«Погашено за период» — подсказка «за вычетом вернувшегося при отмене»', /за вычетом монет, вернувшихся при отмене/.test(k.red || ''), k.red);
  const row = await page.evaluate(id => {
    const tr = [...document.querySelectorAll('#repBody tr')].find(t => t.querySelector(`button[onclick*="${id}"]`) === null
      && /Guest37/.test(t.textContent) && /отменена, вернулось 200/.test(t.textContent));
    return tr ? tr.textContent.replace(/\s+/g, ' ').trim() : null;
  }, r.data.id);
  z.check('в «Погашениях за период»: «400 — отменена, вернулось 200»', /400 — отменена, вернулось 200/.test(row || ''), row);
  await page.locator('#repBody .sect', { hasText: 'Погашения за период' }).scrollIntoViewIfNeeded();
  await z.shot(page, 'z5_coins');
  await z.done();
})().catch(z.crash);
