// З18-и4г «Журнал админа, потолки и отмена — с начала смены», браузер: временный админ z18p на смене, начатой до начала
// текущих суток (запись табеля сдвинута SQL-ом; по возможности — с 20:00 накануне). Его пополнение 6 zł картой «до
// полуночи» — в «Журнале» (чип «Деньги», «Ещё 100» до него), над журналом — «Журнал — с начала твоей смены, с 20:00
// вчера: она началась раньше текущих суток»; «Продажи смены» — «с 20:00 вчера · …» вместо даты суток, продажа «z18p Сок»
// той же смены — в ленте с «Вернуть». Не на смене — подписи нет, лента — с даты суток. Телефон 360 — подпись без прокрутки
// вбок. В конце пополнение и продажа отменены, позиция удалена, записи табеля z18p — тоже, z18p уволен.
const z = require('./_z');
(async () => {
  const NICK = 'z18p', G = z.uid('Guest49'), GOOD = 'z18p Сок';
  const entries = `DELETE FROM work_entries WHERE user_id = (SELECT id FROM users WHERE nickname = '${NICK}')`;
  z.sql(entries);
  z.atEnd(() => z.sql(entries)); // уборка — в обратном порядке: записи табеля — уже после увольнения
  for (const id of z.rows(`SELECT id FROM goods WHERE name = '${GOOD}'`)) z.dropGood(id);
  z.atEnd(() => { for (const id of z.rows(`SELECT id FROM goods WHERE name = '${GOOD}'`)) z.dropGood(id); });
  const good = ((await z.api('POST', '/admin/goods', { name: GOOD, category: 'z18', price_pln: 4, stock: 5 }, 'boss')).data.good || {}).id;
  await z.hireTemp(NICK);
  const page = await z.openAdmin(NICK); // вход открывает смену
  const uidN = z.uid(NICK);
  const DF = `'${(await z.api('GET', '/admin/overview', undefined, NICK)).data.day_from}'::timestamptz`;
  const S = `'${z.sql(`SELECT greatest(${DF} - interval '4 hours', now() - interval '23 hours 55 minutes')`)}'::timestamptz`;
  const M = `'${z.sql(`SELECT ${S} + (${DF} - ${S}) / 2`)}'::timestamptz`;
  const hm = e => z.sql(`SELECT to_char((${e}) AT TIME ZONE 'Europe/Warsaw', 'HH24:MI')`);
  const ago = z.sql(`SELECT CASE WHEN ((${S}) AT TIME ZONE 'Europe/Warsaw')::date = (now() AT TIME ZONE 'Europe/Warsaw')::date - 1 THEN ' вчера' ELSE '' END`);
  z.sql(`UPDATE work_entries SET started_at = ${S} WHERE user_id = '${uidN}' AND ended_at IS NULL`);
  const dep = (await z.api('POST', `/admin/users/${G}/deposit`, { amount_pln: 6, method: 'card' }, NICK)).data.deposit_id;
  const sale = ((await z.api('POST', '/admin/sales', { good_id: good, method: 'card' }, NICK)).data.sale || {}).id;
  z.atEnd(() => z.api('POST', `/admin/deposits/${dep}/void`, {}, 'boss'));
  z.atEnd(() => z.api('POST', `/admin/sales/${sale}/void`, {}, 'boss'));
  z.sql(`UPDATE deposits SET created_at = ${M} WHERE id = '${dep}'`);
  z.sql(`UPDATE sales SET created_at = ${M}, paid_at = ${M} WHERE id = '${sale}'`);
  const since = `с ${hm(S)}${ago}`;
  z.check(`смена z18p — ${since}, раньше текущих суток (с ${hm(DF)}); пополнение 6 zł и продажа — в ${hm(M)}`,
    !!dep && !!sale && z.sql(`SELECT ${S} < ${DF}`) === 't', `${dep} ${sale}`);

  // «Журнал» — чип «Деньги», «Ещё 100», пока не дойдём до пополнения
  await z.nav(page, 'grants');
  await page.click('#s-grants .segs button[data-k="money"]');
  await z.sleep(1200);
  const find = async () => {
    for (let i = 0; i < 20; i++) {
      const st = await page.evaluate(id => ({ has: audData.some(x => x.id === 'deposit:' + id), more: audHasMore }), dep);
      if (st.has || !st.more) return st.has;
      await page.evaluate(() => audMore());
      await z.sleep(700);
    }
    return false;
  };
  const found = await find();
  const j = await page.evaluate(id => {
    const n = document.getElementById('audShiftNote'), row = [...document.querySelectorAll('#auditTable tr')].find(r => /6 zł/.test(r.textContent) && /Guest49/.test(r.textContent) && /z18p/.test(r.textContent));
    return { note: n ? n.textContent.replace(/\s+/g, ' ').trim() : null, row: row ? row.textContent.replace(/\s+/g, ' ').trim() : null };
  }, dep);
  z.check(`над журналом — «Журнал — с начала твоей смены, ${since}: она началась раньше текущих суток»; пополнение 6 zł от z18p до начала суток — в журнале`,
    j.note === `Журнал — с начала твоей смены, ${since}: она началась раньше текущих суток` && found && !!j.row, JSON.stringify({ found, ...j }));
  await page.locator('#auditTable').scrollIntoViewIfNeeded();
  await z.shot(page, 'z18_journal_admin');

  // «Товары» → «Продажи смены»
  await z.nav(page, 'goods');
  await page.waitForFunction(() => /zł/.test((document.getElementById('salesTotal') || {}).textContent || ''), null, { timeout: 8000 }).catch(() => {});
  const st = await page.evaluate(id => ({ label: (document.getElementById('salesTotal') || {}).textContent || '',
    has: salesCache.some(x => x.id === id) }), sale);
  z.check(`«Продажи смены · ${since} · …» вместо даты суток; продажа «${GOOD}» его смены — в ленте`,
    st.label.startsWith(`${since} · `) && st.has, JSON.stringify(st));
  await z.shot(page, 'z18_journal_sales');

  // не на смене — как раньше: подписи нет, лента — с даты суток
  z.sql(`UPDATE work_entries SET ended_at = now() WHERE user_id = '${uidN}' AND ended_at IS NULL`);
  await page.evaluate(() => renderGoods());
  await z.sleep(1200);
  const day = z.clubDay(0);
  const st2 = await page.evaluate(() => (document.getElementById('salesTotal') || {}).textContent || '');
  await z.nav(page, 'grants');
  await page.evaluate(() => renderAudit());
  await z.sleep(1000);
  const note2 = await page.evaluate(() => !!document.getElementById('audShiftNote'));
  z.check(`смена закрыта: лента — «${day} · …», подписи над журналом нет`, st2.startsWith(`${day} · `) && !note2, `${st2} · ${note2}`);

  // телефон: снова на смене с вечера — подпись помещается
  z.sql(`UPDATE work_entries SET ended_at = NULL WHERE id = (SELECT id FROM work_entries WHERE user_id = '${uidN}' ORDER BY started_at DESC LIMIT 1)`);
  const ph = await z.openAdmin(NICK, { viewport: { width: 360, height: 800 } });
  await z.nav(ph, 'grants');
  await z.sleep(800);
  const pw = await ph.evaluate(() => { const n = document.getElementById('audShiftNote'), r = n ? n.getBoundingClientRect() : null;
    return { note: !!n, right: r ? Math.round(r.right) : null, sw: document.documentElement.scrollWidth, w: innerWidth }; });
  await z.shot(ph, 'z18_journal_phone');
  z.check('телефон 360: подпись о смене — в ширину экрана, вбок не прокручивается', pw.note && pw.right <= pw.w && pw.sw <= pw.w, JSON.stringify(pw));
  await z.done();
})().catch(z.crash);
