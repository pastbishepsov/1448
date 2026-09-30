// З8-и6 «Отмена оплаченного заказа кухни возвращает деньги», браузер: в «Кухне» «Отменить» у заказа от стойки,
// оплаченного сразу, — тост говорит, что стало с деньгами: кошелёк — «4,00 zł вернулись на кошелёк гостя», наличные —
// «верни гостю 4,00 zł наличными из ящика». Раньше тост обещал «деньги и товар вернулись», а деньги не возвращались.
// admin1, Guest39 (кошелёк докладывается до 10 zł), «Woda 0.5»; всё, что проверка оплатила, в конце отменено.
const z = require('./_z');
(async () => {
  const g = z.uid('Guest39'), W = z.sql("SELECT id FROM goods WHERE name='Woda 0.5'");
  const need = 10 - Number(z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`)) / 100;
  if (need > 0) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: Math.round(need * 100) / 100, method: 'card' }, 'boss');
  const made = [];
  z.atEnd(async () => { for (const { oid, sid } of made) {
    if (!['cancelled', 'done'].includes(z.sql(`SELECT status FROM kitchen_orders WHERE id='${oid}'`)))
      await z.api('POST', `/admin/kitchen/orders/${oid}/cancel`, {}, 'boss');
    if (sid && z.sql(`SELECT (voided_at IS NULL)::text FROM sales WHERE id='${sid}'`) === 'true')
      await z.api('POST', `/admin/sales/${sid}/void`, {}, 'boss');
  } });
  const page = await z.openAdmin('admin1');
  const cancelInUI = async method => {
    const o = await z.api('POST', '/admin/kitchen/orders', { guest_id: g, good_id: W, qty: 1, method }, 'admin1');
    made.push({ oid: (o.data.order || {}).id, sid: o.data.sale_id });
    await z.nav(page, 'kitchen');
    const card = page.locator('#kitBoard .card', { hasText: 'Guest39' }).filter({ hasText: 'оплачен: ' + { wallet: 'кошелёк', cash: 'наличные' }[method] })
      .filter({ has: page.locator('button:has-text("Отменить")') }).last();
    await card.locator('button:has-text("Отменить")').click(); await z.sleep(200);
    // З11-и2: выбор открыт один — в нём «Вернуть 4,00 zł …» как платили (карточка уже без «Отменить» — ищем выбор на доске)
    await page.locator(`#kitBoard .rfPick button[data-rf="${method}"]`).click(); await z.sleep(1500);
    return { status: o.status, toast: (await z.text(page, '#toast')) || '' };
  };
  const w = await cancelInUI('wallet');
  z.check('кошелёк: «✓ Заказ отменён — 4,00 zł вернулись на кошелёк гостя, товар — на склад»',
    w.status === 201 && w.toast.includes('Заказ отменён — 4,00 zł вернулись на кошелёк гостя, товар — на склад'), w.toast);
  await z.shot(page, 'z8_refund');
  const c = await cancelInUI('cash');
  z.check('наличные: «верни гостю 4,00 zł наличными из ящика — «должно быть» уже меньше на них»',
    c.status === 201 && c.toast.includes('верни гостю 4,00 zł наличными из ящика — «должно быть» уже меньше на них'), c.toast);
  const left = made.filter(({ sid }) => sid && z.sql(`SELECT (voided_at IS NULL)::text FROM sales WHERE id='${sid}'`) === 'true');
  z.check('продажи обоих заказов отменены вместе с заказами', left.length === 0, JSON.stringify(left));
  await z.done();
})().catch(z.crash);
