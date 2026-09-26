// З7-и3 (№43 «а», №45), браузер: в шапке владельца — кто из админов сейчас на смене, а не «смена не открыта»
// (у владельца своей смены нет); «Я пришёл» и «Моя смена» у владельца скрыты, клик по чипу ведёт в «Персонал».
// У админа шапка прежняя — его смена. Палитра понимает слова владельца: «сдачи» ведёт к сдачам кассы в «Отчётах»
// (З8-и3: «касс» — в «Бухгалтерию», это pw_z8_book), «зарплата», «табель», «выручка», «экспорт» — каждое находит, куда идти. admin1 входит через API —
// вход админки сам открывает ему смену (З3-и1), так проверка знает, кто должен быть в чипе.
const z = require('./_z');
(async () => {
  await z.tok('admin1', true); // вход как админка: смена admin1 открыта (или уже была открыта)
  const owner = await z.openAdmin('boss');
  const chip = await owner.evaluate(() => { const c = document.getElementById('shiftChip');
    return { txt: document.getElementById('shiftTxt').textContent.trim(), title: c.title, on: c.classList.contains('on'), off: c.classList.contains('off') }; });
  z.check('шапка владельца: «на смене: … admin1 …», не «смена не открыта»', /^на смене: .*admin1/.test(chip.txt) && !/не открыта/.test(chip.txt) && chip.on, JSON.stringify(chip));
  z.check('подсказка чипа: кто и с какого времени отметился', /Отметились: .*admin1 с \d\d:\d\d/.test(chip.title), chip.title);
  await owner.screenshot({ path: `${z.OUT}/z_z7_owner_head.png`, clip: { x: 0, y: 0, width: 1440, height: 120 } });
  // смена по графику идёт, а не отметился никто: состояние подставляется в странице, стенд не трогаем
  const red = await owner.evaluate(() => { if (typeof ownerShift === 'undefined') return null;
    const keep = ownerShift; ownerShift = { open: [], plan: [{ nickname: 'admin2', shift: 'День' }] }; paintShiftChip();
    const c = document.getElementById('shiftChip'), r = { txt: document.getElementById('shiftTxt').textContent, off: c.classList.contains('off'), title: c.title };
    ownerShift = keep; paintShiftChip(); return r; });
  z.check('по графику идёт смена, а не отметился никто — «на смене никого» красным, в подсказке кто не отметился',
    red && red.txt === 'на смене никого' && red.off && /по графику, но не отметились: admin2/.test(red.title), JSON.stringify(red));
  await z.nav(owner, 'staff');
  const box = await owner.evaluate(() => { const b = document.getElementById('myShiftBox');
    return { hidden: !b || b.classList.contains('hidden') || !b.offsetParent, text: b ? b.textContent.replace(/\s+/g, ' ').trim() : '' }; });
  z.check('«Персонал» у владельца: «Я пришёл» и «Моя смена» скрыты', box.hidden && !/Я пришёл/.test(box.text), JSON.stringify(box));
  await owner.evaluate(() => nav('hall'));
  await z.sleep(800);
  await owner.click('#shiftChip');
  await z.sleep(1200);
  const after = await owner.evaluate(() => ({ tab: curTab, drawer: document.getElementById('drawer').classList.contains('open') ? document.getElementById('drawerTitle').textContent : '' }));
  z.check('клик по чипу у владельца → «Персонал», без шторки «Моя смена»', after.tab === 'staff' && !/Моя смена/.test(after.drawer), JSON.stringify(after));

  const pal = async q => { await owner.evaluate(() => palOpen()); await owner.fill('#palInp', q); await z.sleep(500);
    return owner.evaluate(() => [...document.querySelectorAll('#palList .pi')].map(x => ({ name: x.querySelector('b').textContent,
      sub: (x.querySelector('.muted') || {}).textContent || '' }))); };
  const words = {};
  for (const q of ['касс', 'сдачи', 'зарплата', 'табель', 'выручка', 'экспорт']) words[q] = (await pal(q)).map(x => x.name + (x.sub ? ' — ' + x.sub : ''));
  await owner.keyboard.press('Escape');
  z.check('палитра владельца: у каждого слова есть куда идти — касс, сдачи, зарплата, табель, выручка, экспорт',
    Object.values(words).every(a => a.length > 0), JSON.stringify(words));
  await pal('сдачи');
  await owner.screenshot({ path: `${z.OUT}/z_z7_owner_palette.png`, clip: { x: 320, y: 60, width: 800, height: 300 } });
  await owner.keyboard.press('Enter');
  await z.sleep(1500);
  const cash = await owner.evaluate(() => { const k = document.getElementById('shiftKpis'); const r = k && k.getBoundingClientRect();
    return { tab: curTab, pal: document.getElementById('pal').classList.contains('hidden'), inView: !!r && r.height > 0 && r.top >= 0 && r.top < innerHeight }; });
  z.check('«сдачи» + Enter → «Отчёты», сводка смены с кассой на экране', cash.tab === 'stats' && cash.pal && cash.inView, JSON.stringify(cash));

  const adm = await z.openAdmin('admin1');
  const achip = await adm.evaluate(() => document.getElementById('shiftTxt').textContent.trim());
  z.check('шапка админа — его смена: «на смене с ЧЧ:ММ»', /^на смене с \d\d:\d\d$/.test(achip), achip);
  await adm.screenshot({ path: `${z.OUT}/z_z7_admin_head.png`, clip: { x: 0, y: 0, width: 1440, height: 120 } });
  await z.nav(adm, 'staff');
  const abox = await adm.evaluate(() => (document.getElementById('myShiftBox') || {}).textContent || '');
  z.check('«Персонал» у админа: блок своей смены на месте («Отметить уход»)', /Отметить уход/.test(abox), abox.replace(/\s+/g, ' ').trim());
  await z.done();
})().catch(z.crash);
