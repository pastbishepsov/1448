// З15-и2 «Канал: «ещё» и честное «прочитано»», браузер (№27, З-Р20). У boss 130 непрочитанных z15c (SQL-ом подряд, по
// миллисекунде): бейдж канала «99+»; открыл — в ленте все 130, над первым — черта «Новые» (она на экране), сверху —
// «Ещё 100 — раньше», бейдж погас. «Ещё» — до ста сообщений старше сверху, то, что было на экране, стоит на месте. Лента
// внизу — новое z15c появляется и сразу прочитано, набранный текст не стирается; пролистал вверх — новое не прочитано
// само (бейдж «1»), долистал вниз — прочитано. «Персонал» — строка «Расхождений касс за 7 дней: N», клик — шторка с
// недостачей z15e 2,00 и кнопкой в «Кассу» за те же 7 дней; у admin1 строки нет. Телефон 360 — лента и строка без
// прокрутки вбок. Сообщения «z15 лента …» удаляются, отметка boss «прочитано» возвращается; z15e сдаёт кассу с недостачей
// 2 zł — сдача остаётся в книге.
const z = require('./_z');
(async () => {
  const MARK = 'z15 лента';
  const drop = () => z.sql(`DELETE FROM staff_messages WHERE text LIKE '${MARK} %'`);
  drop(); // хвост упавшего прогона
  z.atEnd(drop);
  const b0 = z.sql(`SELECT COALESCE(staff_chat_read_at::text, '') FROM users WHERE nickname = 'boss'`);
  z.atEnd(() => z.sql(`UPDATE users SET staff_chat_read_at = ${b0 ? `'${b0}'::timestamptz` : 'NULL'} WHERE nickname = 'boss'`));
  await z.hireTemp('z15c'); await z.hireTemp('z15e');

  // недостача для «Персонала» — до сообщений: служебная строка сдачи уйдёт в канал раньше них и счёт не собьёт
  let pre = (await z.api('GET', '/admin/work/closeout', undefined, 'z15e')).data; // вход в админку открыл смену
  if (pre.baseline) { // ящика на стенде ещё нет — заводит его z15e сдачей на 500 zł
    await z.api('POST', '/admin/work/stop', { counted_pln: 500 }, 'z15e');
    await z.api('POST', '/admin/work/start', {}, 'z15e');
    pre = (await z.api('GET', '/admin/work/closeout', undefined, 'z15e')).data;
  }
  const stop = await z.api('POST', '/admin/work/stop', { counted_pln: +(pre.expected_pln - 2).toFixed(2) }, 'z15e');
  z.check('данные: z15e сдал кассу на 2,00 меньше, чем должно быть', stop.status === 200 && Math.abs(((stop.data || {}).closeout || {}).diff_pln + 2) < 0.001,
    `${stop.status} ${JSON.stringify((stop.data || {}).closeout || stop.data).slice(0, 200)}`);
  await z.sleep(300);
  const club = z.sql(`SELECT id FROM clubs ORDER BY created_at LIMIT 1`);
  z.sql(`INSERT INTO staff_messages (club_id, author_id, role, kind, visible_to, text, created_at) SELECT '${club}', '${z.uid('z15c')}', 'admin', 'text', 'all',
    '${MARK} #' || lpad(i::text, 3, '0'), now() - ((140 - i) || ' milliseconds')::interval FROM generate_series(1, 130) i`);
  const first = z.sql(`SELECT created_at FROM staff_messages WHERE text = '${MARK} #001'`);
  z.sql(`UPDATE users SET staff_chat_read_at = timestamptz '${first}' - interval '1 millisecond' WHERE nickname = 'boss'`);

  const boss = await z.openAdmin('boss');
  const badge0 = await z.text(boss, '#staffBadge');
  z.check('у boss 130 непрочитанных — бейдж канала «99+»', badge0 === '99+', badge0);
  await boss.evaluate(() => openStaffChat()); await z.sleep(1500);
  const st = await boss.evaluate(M => {
    const w = document.getElementById('scWrap'), msgs = [...w.querySelectorAll('.scmsg')];
    const ours = msgs.filter(e => e.textContent.includes(M + ' #')), nw = document.getElementById('scNew'), mb = document.getElementById('scMore');
    const r = nw ? nw.getBoundingClientRect() : null, wr = w.getBoundingClientRect();
    return { n: msgs.length, ours: ours.length, first: ours[0] ? /#001/.test(ours[0].textContent) : false, newBefore: !!nw && nw.nextElementSibling === ours[0],
      newOnScreen: !!r && r.top >= wr.top - 1 && r.bottom <= wr.bottom + 1, more: mb ? mb.textContent.trim() : null,
      badgeHidden: document.getElementById('staffBadge').classList.contains('hidden'), unread: staffUnread };
  }, MARK);
  const srv0 = (await z.api('GET', '/admin/staff-chat', undefined, 'boss')).data.unread;
  z.check('открыл канал: в ленте все 130 непрочитанных, над первым — черта «Новые», и она на экране; сверху — «Ещё 100 — раньше»; '
    + 'бейдж погас, на сервере непрочитанных 0', st.ours === 130 && st.first && st.newBefore && st.newOnScreen && st.more === 'Ещё 100 — раньше'
    && st.badgeHidden && st.unread === 0 && srv0 === 0, JSON.stringify({ st, srv0 }));
  await z.shot(boss, 'z15_chat_new');

  const spot = () => boss.evaluate(M => { const w = document.getElementById('scWrap'), e = [...w.querySelectorAll('.scmsg')].find(x => x.textContent.includes(M + ' #001'));
    return { n: w.querySelectorAll('.scmsg').length, top: e ? Math.round(e.getBoundingClientRect().top) : null }; }, MARK);
  await boss.locator('#scMore').scrollIntoViewIfNeeded(); await z.sleep(300); // кнопку сначала видно — клик сам ленту не двигает
  const s1 = await spot();
  await boss.click('#scMore'); await z.sleep(1500);
  const s2 = await spot();
  z.check('«Ещё 100 — раньше»: сверху добавилось до ста сообщений старше, #001 на экране стоит на месте', s2.n > s1.n && s2.n - s1.n <= 100
    && s1.top !== null && Math.abs(s2.top - s1.top) <= 2, JSON.stringify({ s1, s2 }));

  await boss.evaluate(() => { const w = document.getElementById('scWrap'); w.scrollTop = w.scrollHeight; });
  await z.sleep(300);
  await boss.fill('#scText', 'черновик z15: не стирать');
  await z.api('POST', '/admin/staff-chat', { text: MARK + ' #131 живое' }, 'z15c');
  await boss.waitForFunction(() => /#131 живое/.test(([...document.querySelectorAll('#scWrap .scmsg')].pop() || {}).textContent || ''), null, { timeout: 15000 }).catch(() => {});
  await z.idle(boss); // З19-и14: живое сообщение — по факту, «прочитано» страница пишет сама — ждём её запись, а не паузу
  const live = await boss.evaluate(M => ({ last: /#131 живое/.test([...document.querySelectorAll('#scWrap .scmsg')].pop().textContent),
    draft: document.getElementById('scText').value, badgeHidden: document.getElementById('staffBadge').classList.contains('hidden'), unread: staffUnread }), MARK);
  const srv1 = (await z.api('GET', '/admin/staff-chat', undefined, 'boss')).data.unread;
  z.check('лента внизу — новое сообщение z15c появилось внизу и сразу прочитано (бейдж не горит, на сервере 0); набранный текст на месте',
    live.last && live.draft === 'черновик z15: не стирать' && live.badgeHidden && live.unread === 0 && srv1 === 0, JSON.stringify({ live, srv1 }));

  await boss.evaluate(() => { document.getElementById('scWrap').scrollTop = 0; });
  await z.sleep(300);
  await z.api('POST', '/admin/staff-chat', { text: MARK + ' #132 пока листал' }, 'z15c');
  await z.sleep(2500);
  const up = await boss.evaluate(() => { const b = document.getElementById('staffBadge'); return { badge: b.textContent, hidden: b.classList.contains('hidden'),
    top: Math.round(document.getElementById('scWrap').scrollTop) }; });
  z.check('пролистал вверх — новое само не «прочитано»: бейдж «1», лента стоит, где была', !up.hidden && up.badge === '1' && up.top <= 2, JSON.stringify(up));
  await boss.evaluate(() => { const w = document.getElementById('scWrap'); w.scrollTop = w.scrollHeight; w.dispatchEvent(new Event('scroll')); });
  await z.idle(boss); // «прочитано» — запись
  const down = await boss.evaluate(() => ({ hidden: document.getElementById('staffBadge').classList.contains('hidden'), unread: staffUnread }));
  const srv2 = (await z.api('GET', '/admin/staff-chat', undefined, 'boss')).data.unread;
  z.check('долистал до низа — прочитано: бейдж погас, на сервере 0', down.hidden && down.unread === 0 && srv2 === 0, JSON.stringify({ down, srv2 }));
  await z.shot(boss, 'z15_chat_live');

  await boss.evaluate(() => closeDrawer());
  await z.nav(boss, 'staff');
  const row = await z.text(boss, '#cashDiffsRow');
  z.check('«Персонал»: строка «Расхождений касс за 7 дней: N» — с суммой не объяснённого и «Кто и когда»',
    /^Расхождений касс за 7 дней: \d+ — не объяснено: недостача \d/.test(row || '') && /Кто и когда$/.test(row || ''), row);
  await boss.click('#cashDiffsRow'); await z.sleep(1000);
  const dr = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent,
    rows: [...document.querySelectorAll('#drawerBody tr')].map(r => r.textContent.replace(/\s+/g, ' ').trim()) }));
  z.check('клик — шторка «Расхождения касс за 7 дней»: строка z15e — недостача −2,00 zł', dr.title === 'Расхождения касс за 7 дней'
    && dr.rows.some(r => /z15e/.test(r) && /−2,00 zł/.test(r)), JSON.stringify(dr).slice(0, 400));
  await z.shot(boss, 'z15_cash_diffs');
  await boss.click('#cdToCash'); await z.sleep(1800);
  const acc = await boss.evaluate(() => ({ tab: curTab, acc: accTab, preset: accPeriod.preset, open: document.getElementById('drawer').classList.contains('open') }));
  z.check('«Касса по дням за эти 7 дней» — «Бухгалтерия» → «Касса», период «7 дней», шторка закрыта', acc.tab === 'acc' && acc.acc === 'cash'
    && acc.preset === 'd7' && !acc.open, JSON.stringify(acc));

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'staff');
  const ar = await adm.evaluate(() => ({ row: !!document.getElementById('cashDiffsRow'), box: (document.getElementById('staffCashBox') || {}).className }));
  z.check('admin1: в «Персонале» строки о расхождениях касс нет', !ar.row && /hidden/.test(ar.box || ''), JSON.stringify(ar));

  await adm.goto('about:blank');
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => openStaffChat()); await z.sleep(1500);
  const f1 = await ph.evaluate(() => { const b = document.getElementById('scMore'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, more: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(ph, 'z15_chat_phone');
  z.check('телефон 360: лента канала и «Ещё 100 — раньше» — без прокрутки вбок', f1.sw <= 360 && f1.more, JSON.stringify(f1));
  await ph.evaluate(() => closeDrawer());
  await z.nav(ph, 'staff');
  const f2 = await ph.evaluate(() => { const r = document.getElementById('cashDiffsRow'), q = r ? r.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, row: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(ph, 'z15_cash_phone');
  z.check('телефон 360: строка расхождений в «Персонале» — без прокрутки вбок', f2.sw <= 360 && f2.row, JSON.stringify(f2));
  await z.done();
})().catch(z.crash);
