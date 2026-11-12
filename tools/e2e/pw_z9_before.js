// З9-и3в «Взяли до сдачи», браузер. Шторка расхода: у «Банка» вопроса нет; «Из кассы» за сегодня, а сегодня уже
// пересчитывали ящик, — «Взяли из кассы: после сдачи ЧЧ:ММ (кто) — „должно быть“ меньше сейчас / до сдачи ЧЧ:ММ (кто,
// расхождение …) — объяснит её»; сняли «оплачен» — вопрос пропал. «До сдачи» — тост «объясняет сдачу …», «должно быть»
// не меняется. Открыли расход снова — выбрана та сдача; переключили на «после» — тост «„должно быть в ящике“ меньше на
// …», ящик меньше. Временный z9bp; расходы с заметкой «z9bp проверка» в конце сторнируются, z9bp уволен.
const z = require('./_z');
(async () => {
  const MARK = 'z9bp проверка';
  const clean = async () => {
    for (const id of z.sql(`SELECT string_agg(id::text, ' ') FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`).split(' ').filter(Boolean))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
  };
  await clean(); // хвост упавшего прогона
  z.atEnd(clean);
  await z.hireTemp('z9bp');
  const pv = async () => (await z.api('GET', '/admin/work/closeout', undefined, 'z9bp')).data;
  await z.openAdmin('z9bp'); // вход в админку открывает смену
  if ((await pv()).baseline) {
    await z.api('POST', '/admin/work/stop', { counted_pln: 500 }, 'z9bp');
    await z.api('POST', '/admin/work/start', {}, 'z9bp');
  }
  // z9bp пересчитал ящик с недостачей 9 zł
  const pre = (await pv()).expected_pln;
  const st = (await z.api('POST', '/admin/work/stop', { counted_pln: Math.round((pre - 9) * 100) / 100 }, 'z9bp')).data;
  await z.api('POST', '/admin/work/start', {}, 'z9bp');
  const coId = (st.closeout || {}).id, p0 = await pv();

  const boss = await z.openAdmin('boss');
  const after = (act, want) => z.toastAfter(boss, act, want); // З19-и13: тост этого действия, а не #toast через паузу
  const F = v => boss.evaluate(x => fmtPLN(x).replace(/\s+/g, ' '), v);
  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  await boss.click('#expNewBtn'); await z.sleep(1500);
  const noQ = await boss.evaluate(() => !document.getElementById('efBefore'));
  await boss.click('#efMethod button:has-text("Из кассы")'); await z.sleep(400);
  const opts = await boss.evaluate(() => [...document.querySelectorAll('#efBefore option')].map(o => ({ v: o.value, t: o.textContent.replace(/\s+/g, ' ').trim() })));
  await boss.click('#efPaid'); await z.sleep(200);
  const hiddenUnpaid = await boss.evaluate(() => !document.getElementById('efBefore'));
  await boss.click('#efPaid'); await z.sleep(200);
  z.check('шторка расхода: у «Банка» вопроса нет; у «Из кассы» за сегодня — «после сдачи … (z9bp)» и «до сдачи … (z9bp, расхождение −9,00) — объяснит её»; без «оплачен» — вопрос пропал',
    noQ && hiddenUnpaid && opts.length >= 2 && opts[0].v === '' && /^после сдачи \d\d:\d\d \(z9bp\) — «должно быть» меньше сейчас$/.test(opts[0].t)
    && opts.some(o => o.v === coId && /^до сдачи \d\d:\d\d \(z9bp, расхождение −9,00\) — объяснит её$/.test(o.t)), JSON.stringify({ noQ, hiddenUnpaid, opts: opts.slice(0, 3) }));
  await boss.selectOption('#efBefore', coId || '');
  await boss.selectOption('#efCat', { label: 'Прочее' });
  await boss.fill('#efAmount', '9');
  await boss.fill('#efNote', MARK + ': курьер до сдачи');
  await z.shot(boss, 'z9_before_form');
  const t1 = await after(() => boss.click('#efSave'), 'Расход 9,00 zł внесён'), p1 = await pv();
  const coTxt = await boss.evaluate(id => { const c = (expTodayCos.list || []).find(x => x.id === id); return c ? ruDate(c.created_at.slice(0, 10)) + ' ' + hhmm(c.created_at) : '?'; }, coId);
  z.check(`«до сдачи» → тост «… · объясняет сдачу ${coTxt} (z9bp)», «должно быть в ящике» не изменилось`,
    t1.includes(`Расход 9,00 zł внесён — Прочее · объясняет сдачу ${coTxt} (z9bp)`) && Math.abs(p1.expected_pln - p0.expected_pln) < 0.005,
    JSON.stringify({ t1, p0: p0.expected_pln, p1: p1.expected_pln }));

  const id = z.sql(`SELECT id FROM expenses WHERE note = '${MARK}: курьер до сдачи' AND storno_at IS NULL ORDER BY created_at DESC LIMIT 1`);
  await boss.click(`#accBody tr.expRow[data-id="${id}"]`); await z.sleep(1800);
  const sel = await boss.evaluate(() => { const s = document.getElementById('efBefore'); return s ? s.value : null; });
  await boss.selectOption('#efBefore', '');
  const t2 = await after(() => boss.click('#efSave'), 'Расход поправлен'), p2 = await pv();
  z.check(`открыли снова — выбрана та сдача; «после сдачи» → тост «Расход поправлен · „должно быть в ящике“ меньше на 9,00 zł», ящик −9`,
    sel === coId && t2.includes('Расход поправлен · «должно быть в ящике» меньше на 9,00 zł') && Math.abs(p2.expected_pln - (p1.expected_pln - 9)) < 0.005,
    JSON.stringify({ sel, t2, p1: p1.expected_pln, p2: p2.expected_pln }));
  await z.done();
})().catch(z.crash);
