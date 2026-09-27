// З8-и5 «Очередь кухни без потерь», браузер: в «Кухне» только что пришедший заказ — в очереди, даже когда за сутки
// закрытых заказов больше двухсот; значок у «Кухни» считает все заказы в работе и «ждут оплаты»; у «За смену» —
// подпись «последние 200 из N». Очередь на время переполняется: 250 отменённых заказов «z8 заглушка» на boss с начала
// клубных суток (склад, деньги и долг не трогают), в конце удаляются. Guest39 за PC-06 заказывает «Woda 0.5» с
// оплатой при выходе; в конце заказ отменяется (товар — на склад), сессия завершается.
const z = require('./_z');
(async () => {
  const STUB = 'z8 заглушка', g = z.uid('Guest39'), boss = z.uid('boss');
  const W = z.sql("SELECT id FROM goods WHERE name='Woda 0.5'"), pc = z.sql("SELECT id FROM computers WHERE name='PC-06'");
  const rh = Number(z.sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"));
  const from = `(('${z.clubDay(0)}'::date + time '${String(rh).padStart(2, '0')}:00') AT TIME ZONE 'Europe/Warsaw')`;
  z.sql(`DELETE FROM kitchen_orders WHERE name = '${STUB}'`); // хвост упавшего прогона
  z.atEnd(() => z.sql(`DELETE FROM kitchen_orders WHERE name = '${STUB}'`));
  for (const sid of z.sql(`SELECT string_agg(id::text, ' ') FROM sessions WHERE status='active' AND user_id='${g}'`).split(' ').filter(Boolean))
    await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  const need = 20 - Number(z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`)) / 100;
  if (need > 0) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: Math.round(need * 100) / 100, method: 'card' }, 'boss');
  const seat = await z.api('POST', `/admin/computers/${pc}/session`, { guest: 'Guest39' }, 'admin1');
  if (seat.data.session_id) z.atEnd(() => z.api('POST', `/admin/sessions/${seat.data.session_id}/end`, {}, 'admin1'));
  z.sql(`INSERT INTO kitchen_orders (club_id, user_id, name, qty, price_pln, total_pln, paid, status, status_at, created_at)
    SELECT (SELECT id FROM clubs ORDER BY created_at LIMIT 1), '${boss}', '${STUB}', 1, 1, 1, 'postpay', 'cancelled', now(),
    ${from} + g * interval '1 second' FROM generate_series(1, 250) AS g`);
  const o = await z.api('POST', '/admin/kitchen/orders', { guest_id: g, good_id: W, qty: 1, method: 'later' }, 'admin1');
  const oid = (o.data.order || {}).id;
  if (oid) z.atEnd(() => z.api('POST', `/admin/kitchen/orders/${oid}/cancel`, {}, 'admin1'));
  if (!z.check('Guest39 за PC-06 заказал «Woda 0.5» с оплатой при выходе', seat.status === 201 && o.status === 201 && oid,
    `${seat.status} ${o.status} ${JSON.stringify(o.data).slice(0, 200)}`)) return z.done();
  const q = (await z.api('GET', '/admin/kitchen/orders', undefined, 'admin1')).data;

  const page = await z.openAdmin('admin1');
  await z.nav(page, 'kitchen');
  const v = await page.evaluate(() => {
    const cards = [...document.querySelectorAll('#kitBoard .card')].map(c => c.textContent.replace(/\s+/g, ' ').trim());
    const heads = [...document.querySelectorAll('#kitBoard > .muted')].map(h => h.textContent.replace(/\s+/g, ' ').trim());
    const b = document.getElementById('kitBadge');
    return { first: cards[0] || '', heads, badge: b && !b.classList.contains('hidden') ? b.textContent.trim() : '' };
  });
  z.check('первая карточка очереди — новый заказ: «принят», Woda 0.5, Guest39, нести к PC-06',
    /принят/.test(v.first) && /Woda 0\.5/.test(v.first) && /Guest39/.test(v.first) && /нести к PC-06/.test(v.first), v.first.slice(0, 200));
  z.check(`значок «Кухни» — все в работе и «ждут оплаты»: ${q.open + q.unpaid}`, v.badge === String(q.open + q.unpaid) && q.open >= 1,
    `${v.badge} / open ${q.open} · unpaid ${q.unpaid}`);
  z.check(`«За смену · последние 200 из ${q.closed_total}»`, q.closed_total > 200 && v.heads.includes(`За смену · последние 200 из ${q.closed_total}`),
    JSON.stringify(v.heads));
  await z.shot(page, 'z8_kitchen');
  await z.done();
})().catch(z.crash);
