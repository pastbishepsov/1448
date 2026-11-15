// З16-и1а «Пересадка в другую зону — тариф этой зоны», браузер (решение основателя 28.10). Шторка ПК, «Пересадить» за
// ПК другой зоны: сперва вопрос с тарифом сейчас и после — «Тариф VIP · 30,00 zł/ч» или «Оставить Standard · 20,00 zł/ч…»;
// вопрос переживает перерисовку зала; «прежний» без причины не уходит, с причиной — тариф прежний, в «Деньгах» шторки
// VIP-01 — «тариф Standard»; следом VIP-01 → VIP-02 — снова вопрос (тариф сессии — Standard), «Тариф VIP» — тариф VIP,
// пометки нет. Гость Guest47: кошелёк докладывается до 30 zł (пополнение картой от boss), сессия за PC-04 — с 5 минутами
// игры (SQL), в конце завершается.
const z = require('./_z');
(async () => {
  const G = 'Guest47', WHY = 'z16 браузер: PC-04 шумит — VIP-01 по цене Standard';
  const g = z.uid(G);
  const pc = n => z.sql(`SELECT id FROM computers WHERE name='${n}'`);
  for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND user_id='${g}'`)) // хвосты упавшего прогона
    await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
  z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
    + "(SELECT computer_id FROM sessions WHERE status='active')");
  const w = +z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  if (w < 3000) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: (3000 - w) / 100, method: 'card' }, 'boss');
  const seat = await z.api('POST', `/admin/computers/${pc('PC-04')}/session`, { guest: G }, 'admin1');
  const sid = seat.data.session_id;
  z.atEnd(async () => {
    if (sid && z.sql(`SELECT status FROM sessions WHERE id='${sid}'`) === 'active') await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  });
  z.sql(`UPDATE sessions SET started_at = now() - interval '5 minutes', ready_at = now() - interval '5 minutes' WHERE id='${sid}'`);
  const row = () => z.sql(`SELECT c.name || '|' || s.zone_name || '|' || s.effective_rate_pln FROM sessions s
    JOIN computers c ON c.id = s.computer_id WHERE s.id='${sid}'`);
  const journal = () => z.sql(`SELECT details FROM admin_actions WHERE action='session_move' AND target_user_id='${g}'
    ORDER BY created_at DESC LIMIT 1`);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();

  const p = await z.openAdmin('admin1');
  const openPC = async name => { await p.evaluate(id => openPCDrawer(id), pc(name)); await z.sleep(700); };
  const ask = () => p.evaluate(() => { const b = document.getElementById('moveAsk'); return b ? b.textContent.replace(/\s+/g, ' ').trim() : null; });
  const money = () => p.evaluate(() => { const r = [...document.querySelectorAll('#drawerBody tr')].find(t => /^Деньги/.test(t.textContent.trim()));
    return r ? [...r.cells].map(c => c.textContent.trim()).join(' ').replace(/\s+/g, ' ') : null; });
  // элемента нет (на HEAD вопроса нет) — не ждём его 30 секунд, а идём дальше: проверка упадёт, а не повиснет
  const vis = sel => p.isVisible(sel).catch(() => false);
  const tap = async sel => { if (await vis(sel)) { await p.click(sel); return true; } return false; };
  const moveTo = async name => {
    if (!(await vis('#movePCSel'))) return;
    await p.selectOption('#movePCSel', pc(name)); await p.click('#movePCSel + button'); await z.idle(p); // З19-и14: после записи — ждём страницу, а не паузу
  };

  await openPC('PC-04');
  await moveTo('VIP-01');
  const a1 = await ask();
  z.check('PC-04 → VIP-01: вопрос «VIP-01 — зона VIP: тариф гостя станет 30,00 zł/ч (сейчас Standard, 20,00 zł/ч)» с кнопками '
    + '«Тариф VIP · 30,00 zł/ч» и «Оставить Standard · 20,00 zł/ч…»; гость всё за PC-04', a1 === '⚠ VIP-01 — зона VIP: тариф '
    + 'гостя станет 30,00 zł/ч (сейчас Standard, 20,00 zł/ч) Тариф VIP · 30,00 zł/ч Оставить Standard · 20,00 zł/ч…'
    && row() === 'PC-04|Standard|20.00', `${a1} · ${row()}`);
  await z.shot(p, 'z16_move_ask');
  await p.evaluate(() => renderHall());
  await z.sleep(1500);
  const sel = await p.evaluate(() => { const s = document.getElementById('movePCSel');
    return s && s.selectedIndex >= 0 ? s.options[s.selectedIndex].textContent : null; });
  z.check('перерисовка зала: вопрос на месте, в списке выбран VIP-01', a1 !== null && (await ask()) === a1 && /^VIP-01/.test(sel || ''), sel);
  const k1 = await tap('#moveKeep');
  await z.sleep(500);
  let k2 = false;
  const t1 = norm(await z.toastAfter(p, async () => { k2 = k1 && await tap('#moveKeepGo'); }, 'Тариф остаётся прежним')); // З19-и13: тост этого действия, а не #toast через паузу
  z.check('«Оставить Standard…» без причины — тост «Тариф остаётся прежним — напиши почему: причина уйдёт в журнал», гость всё '
    + 'за PC-04', k2 && t1 === 'Тариф остаётся прежним — напиши почему: причина уйдёт в журнал' && row() === 'PC-04|Standard|20.00',
    `${t1} · ${row()}`);
  if (await vis('#moveWhy')) await p.fill('#moveWhy', WHY);
  let k3 = false;
  const t2 = norm(await z.toastAfter(p, async () => { k3 = await tap('#moveKeepGo'); }, 'Пересажен за VIP-01'));
  z.check('с причиной — тост «Пересажен за VIP-01 · тариф прежний»: сессия за VIP-01 по Standard, 20,00 zł/ч; причина — в журнале',
    k3 && t2 === 'Пересажен за VIP-01 · тариф прежний' && row() === 'VIP-01|Standard|20.00'
    && journal() === `PC-04 → VIP-01 · тариф Standard 20,00 zł/ч оставлен: ${WHY}`, `${t2} · ${row()} · ${journal()}`);
  await openPC('VIP-01');
  const m1 = await money();
  z.check('шторка VIP-01: «Деньги 20,00 zł/ч · тариф Standard · кошелёк …»', /^Деньги 20,00 zł\/ч · тариф Standard · кошелёк /.test(m1 || ''), m1);
  await z.shot(p, 'z16_move_keep');

  await moveTo('VIP-02');
  const a2 = await ask();
  let k4 = false;
  const t3 = norm(await z.toastAfter(p, async () => { k4 = await tap('#moveNew'); }, 'Пересажен за VIP-02'));
  z.check('VIP-01 → VIP-02: снова вопрос (тариф сессии — Standard), «Тариф VIP» — тост «Пересажен за VIP-02 · тариф VIP 30,00 '
    + 'zł/ч», сессия по VIP', k4 && /^⚠ VIP-02 — зона VIP: тариф гостя станет 30,00 zł\/ч \(сейчас Standard, 20,00 zł\/ч\)/.test(a2 || '')
    && t3 === 'Пересажен за VIP-02 · тариф VIP 30,00 zł/ч' && row() === 'VIP-02|VIP|30.00', `${a2} · ${t3} · ${row()}`);
  await openPC('VIP-02');
  const m2 = await money();
  z.check('шторка VIP-02: «Деньги 30,00 zł/ч · кошелёк …» — без пометки', /^Деньги 30,00 zł\/ч · кошелёк /.test(m2 || ''), m2);
  await z.done();
})().catch(z.crash);
