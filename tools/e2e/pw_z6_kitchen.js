// З6-и1 (№14), браузер: отмена продажи, которая рассчитала заказ кухни, — тост «Оплата заказа кухни отменена —
// заказ снова ждёт оплаты, склад не тронут» (а не «товар вернулся на склад»), и в карточке гостя снова «Должен».
// Guest38, «Woda 0.5» картой от стойки. В конце заказ рассчитан наличными, остаток товара — как был.
const z = require('./_z');
(async () => {
  const g = z.uid('Guest38'), w = z.sql("SELECT id FROM goods WHERE name='Woda 0.5'");
  const stock0 = z.sql(`SELECT stock FROM goods WHERE id='${w}'`);
  z.atEnd(() => z.sql(`UPDATE goods SET stock=${stock0} WHERE id='${w}'`));
  const o = await z.api('POST', '/admin/kitchen/orders', { guest_id: g, good_id: w, qty: 1, method: 'card' }, 'admin1');
  const oid = (o.data.order || {}).id, sid = o.data.sale_id;
  await z.api('POST', `/admin/kitchen/orders/${oid}/status`, { status: 'done' }, 'admin1');
  z.atEnd(() => z.api('POST', `/admin/kitchen/orders/${oid}/pay`, { method: 'cash' }, 'admin1'));
  z.check('заказ «Woda 0.5» картой от стойки → 201, выдан', o.status === 201 && sid, String(o.status));

  const page = await z.openAdmin('admin1');
  await page.evaluate(() => { window.__toasts = []; const t = toast; toast = (msg, ...a) => { __toasts.push(String(msg)); return t(msg, ...a); }; });
  await page.evaluate(id => voidSale(id), sid);
  await z.sleep(1200);
  const toasts = await page.evaluate(() => __toasts.join(' | '));
  z.check('тост: «Оплата заказа кухни отменена — заказ снова ждёт оплаты, склад не тронут»',
    /Оплата заказа кухни отменена — заказ снова ждёт оплаты, склад не тронут/.test(toasts) && !/вернулся на склад/.test(toasts), toasts);
  await page.evaluate(id => openUserDrawer(id), g);
  await z.sleep(1500);
  const card = await z.text(page, '#drawerBody');
  z.check('карточка Guest38: «Должен 4,00 zł», кухня', /Должен[^0-9]*4,00 zł/.test(card || '') && /кухня/.test(card || ''),
    (card || '').slice(0, 200));
  await z.shot(page, 'z6_kitchen');
  await z.done();
})().catch(z.crash);
