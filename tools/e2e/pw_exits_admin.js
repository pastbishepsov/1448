// Выходы админки (web/admin.html), docs/ВЫХОДЫ-НАЗАД.md: Esc, кнопка «назад» мыши (CDP), клавиша BrowserBack —
// один шаг назад: тур, палитра (и выбор цели в ней), поиск, колокол, шторки и их подформы, формы на экране,
// редактор зала; набранное — только со второго нажатия (тост-предупреждение, 4 с); на экране входа и когда
// ничего не открыто «назад» ничего не делает и со страницы не уводит. Гости ExitAdmA/ExitAdmB (регистрируются
// сами), ПК PC-02 и PC-07; заведённая «Проблема» в конце закрывается.
const L = require('./_exits');
const vis = (p, sel) => p.evaluate(sel => { const e = document.querySelector(sel); return !!e && getComputedStyle(e).display !== 'none' && !e.classList.contains('hidden'); }, sel);
const drawerOpen = p => p.evaluate(() => document.getElementById('drawer').classList.contains('open'));
async function loginAdmin(p, nick, pass){
  await p.goto(L.BASE + '/admin'); await p.waitForTimeout(500);
  await p.fill('#nick', nick); await p.fill('#pass', pass); await p.press('#pass', 'Enter');
  await p.waitForFunction(() => !document.getElementById('appView').classList.contains('hidden'), null, {timeout: 8000});
  await p.waitForTimeout(1200);
}
(async () => {
  const b = await L.browser(); const ctx = await b.newContext({viewport: {width: 1440, height: 900}});
  const p = await ctx.newPage();
  const errs = []; p.on('pageerror', e => errs.push(e.message));
  await p.goto(L.BASE + '/admin'); await p.waitForTimeout(500);
  const url0 = p.url();
  await L.mouseBack(p); await L.esc(p);
  L.check('экран входа: «назад» и Esc — страница на месте', p.url() === url0 && await vis(p, '#loginView'));
  await loginAdmin(p, 'admin1', 'Admin12345');
  const hist0 = await p.evaluate(() => history.length);
  console.log('— тур');
  await p.evaluate(() => tourStart()); await p.waitForTimeout(500);
  L.check('тур на экране', await vis(p, '#tour'));
  await L.mouseBack(p); L.check('тур: «назад» мыши = «Пропустить»', !await vis(p, '#tour'));

  console.log('— шторка ПК');
  const pc = '.hpc:has-text("PC-02")';
  await p.click(pc); await p.waitForTimeout(600);
  L.check('шторка ПК открыта', await drawerOpen(p));
  await L.mouseBack(p); L.check('шторка: «назад» мыши закрыл', !await drawerOpen(p));
  L.check('«назад» не увёл со страницы', p.url() === url0, p.url());
  await p.click(pc); await p.waitForTimeout(600);
  await L.esc(p); L.check('шторка: Esc закрыл', !await drawerOpen(p));
  await p.click(pc); await p.waitForTimeout(600);
  const seat = await p.$('#seatNick');
  if (seat){
    await p.fill('#seatNick', 'Guest4');
    await L.esc(p);
    L.check('набран ник: первый Esc — шторка на месте, тост-предупреждение', await drawerOpen(p) && /несохранённое/.test(await p.evaluate(() => document.getElementById('toast').textContent)));
    await L.esc(p); L.check('второй Esc в течение 4 с — закрыл', !await drawerOpen(p));
    await p.click(pc); await p.waitForTimeout(600);
    await p.fill('#seatNick', 'Guest4'); await L.mouseBack(p); await p.waitForTimeout(4300);
    await L.mouseBack(p); L.check('взвод гаснет через 4 с: «назад» снова только предупреждает', await drawerOpen(p));
    await L.mouseBack(p); L.check('…а следующий сразу — закрывает', !await drawerOpen(p));
  } else console.log('  (поля посадки нет — ПК занят? пропуск)');
  await p.click(pc); await p.waitForTimeout(600);
  await p.click('#drawerBody button:has-text("Проблема")'); await p.waitForTimeout(400);
  L.check('«Проблема» у ПК раскрыта', await vis(p, '#incForm'));
  await L.esc(p); L.check('Esc закрыл форму «Проблема», шторка открыта', !await vis(p, '#incForm') && await drawerOpen(p));
  await L.esc(p); L.check('следующий Esc закрыл шторку', !await drawerOpen(p));

  console.log('— карточка гостя и её подформа');
  const guid = await L.ensureGuest('ExitAdmA');
  await p.evaluate(id => openUserDrawer(id), guid); await p.waitForTimeout(900);
  L.check('карточка гостя открыта', await drawerOpen(p));
  await p.evaluate(() => uFormToggle('deposit')); await p.waitForTimeout(500);
  L.check('форма пополнения раскрыта', await p.evaluate(() => uForm === 'deposit'));
  await L.mouseBack(p); L.check('чистая форма: «назад» свернул форму, карточка открыта', await p.evaluate(() => uForm === null) && await drawerOpen(p));
  await p.evaluate(() => uFormToggle('deposit')); await p.waitForTimeout(500);
  const amt = await p.$('#drawerBody .uform input[type="number"], #drawerBody .uform input');
  await amt.click(); await amt.fill('77');
  await L.mouseBack(p); L.check('набрана сумма: «назад» — форма на месте + предупреждение', await p.evaluate(() => uForm === 'deposit'));
  await L.mouseBack(p); L.check('второй «назад» свернул форму', await p.evaluate(() => uForm === null) && await drawerOpen(p));
  await L.mouseBack(p); L.check('третий «назад» закрыл карточку', !await drawerOpen(p));

  console.log('— палитра, поиск, колокол');
  await p.keyboard.press('Control+KeyK'); await p.waitForTimeout(300);
  L.check('палитра открыта', await vis(p, '#pal'));
  await L.mouseBack(p); L.check('палитра: «назад» закрыл', !await vis(p, '#pal'));
  await p.keyboard.press('Control+KeyK'); await p.waitForTimeout(300);
  await p.keyboard.type('Депозит гостю'); await p.waitForTimeout(400); await p.keyboard.press('Enter'); await p.waitForTimeout(500);
  const pick = await p.evaluate(() => !!palPickAct);
  if (pick){
    await L.mouseBack(p); L.check('палитра: из выбора гостя «назад» → в корень', await vis(p, '#pal') && await p.evaluate(() => !palPickAct));
    await L.mouseBack(p); L.check('…и следующий «назад» закрыл', !await vis(p, '#pal'));
  } else { console.log('  (действие без выбора цели — пропуск)'); await L.esc(p); }
  await p.click('#gq'); await p.keyboard.type('Guest4'); await p.waitForTimeout(700);
  L.check('выпадашка поиска', await vis(p, '#gqDrop'));
  await L.mouseBack(p); L.check('поиск: «назад» закрыл выпадашку и снял фокус', !await vis(p, '#gqDrop') && await p.evaluate(() => document.activeElement.id !== 'gq'));
  await p.click('#bellBtn'); await p.waitForTimeout(300);
  L.check('колокол раскрыт', await vis(p, '#bellDrop'));
  await L.esc(p); L.check('колокол: Esc закрыл (раньше — нет)', !await vis(p, '#bellDrop'));
  await p.click('#bellBtn'); await p.waitForTimeout(300);
  await L.mouseBack(p); L.check('колокол: «назад» закрыл', !await vis(p, '#bellDrop'));
  await p.click('#bellBtn'); await p.waitForTimeout(300);
  await L.browserBackKey(p);
  L.check('BrowserBack закрыл колокол и не увёл', !await vis(p, '#bellDrop') && p.url() === url0);

  console.log('— формы на экране');
  await p.evaluate(() => nav('kitchen')); await p.waitForTimeout(700);
  await p.evaluate(() => kitNewToggle()); await p.waitForTimeout(500);
  L.check('новый заказ кухни раскрыт', await vis(p, '#kitNew'));
  await L.esc(p); L.check('Esc свернул новый заказ', !await vis(p, '#kitNew'));
  // компенсация по проблеме
  const g44 = await L.ensureGuest('ExitAdmB');
  const inc = await L.api('POST', '/admin/incidents', {computer_id: '00000000-0000-0000-0000-000000000007', user_id: g44, kind: 'pc_hang', minutes: 10, note: 'выходы'}, await L.tok('admin1'));
  L.check('проблема заведена', inc.status < 300, inc);
  await p.evaluate(() => nav('grants')); await p.waitForTimeout(600);
  await p.click('button[data-k="incidents"]'); await p.waitForTimeout(900);
  await p.click('#auditTable button:has-text("Компенсировать")'); await p.waitForTimeout(500);
  L.check('форма компенсации раскрыта', await vis(p, '#ic_min'));
  await p.click('#ic_note'); await p.keyboard.type(' — набрал');
  await L.esc(p); L.check('набрана заметка: первый Esc — форма на месте', await vis(p, '#ic_min'));
  await L.esc(p); L.check('второй Esc свернул компенсацию', !await vis(p, '#ic_min'));
  await p.evaluate(() => nav('users')); await p.waitForTimeout(600);
  await p.click('#uq'); await L.esc(p);
  L.check('поле поиска гостей без слоёв: Esc снимает фокус', await p.evaluate(() => document.activeElement.id !== 'uq'));
  await L.mouseBack(p); L.check('ничего не открыто: «назад» ничего не делает и не уводит', p.url() === url0);

  console.log('— моя смена');
  await p.evaluate(() => nav('hall')); await p.waitForTimeout(500);
  await p.click('#shiftChip'); await p.waitForTimeout(900);
  if (await p.$('#drawerBody #ccOpen')){
    await p.click('#drawerBody #ccOpen'); await p.waitForTimeout(400);
    await p.fill('#ccAmount', '5');
    await L.esc(p); L.check('инкассация: Esc свернул сразу (сумма живёт в переменной — не теряется)', !await p.$('#ccAmount') && await drawerOpen(p));
    await p.click('#drawerBody #ccOpen'); await p.waitForTimeout(300);
    L.check('сумма инкассации на месте', await p.inputValue('#ccAmount') === '5');
    await p.evaluate(() => { ccAmount = ''; });
  } else console.log('  (смена не открыта — инкассации нет, пропуск)');
  await L.esc(p); await L.esc(p);
  L.check('«Моя смена» закрыта', !await drawerOpen(p));
  L.check('история не выросла', await p.evaluate(() => history.length) === hist0 && p.url() === url0);
  L.check('ошибок JS нет', errs.length === 0, errs);
  await ctx.close();

  console.log('— редактор зала (владелец)');
  const ctx2 = await b.newContext({viewport: {width: 1440, height: 900}}); const q = await ctx2.newPage();
  q.on('pageerror', e => errs.push(e.message));
  await loginAdmin(q, 'boss', 'Boss12345');
  if (await vis(q, '#tour')) await L.esc(q);
  await q.evaluate(() => nav('hall')); await q.waitForTimeout(500);
  await q.click('#hallEditBtn'); await q.waitForTimeout(500);
  L.check('редактор зала включён', await q.evaluate(() => hallEdit));
  await q.evaluate(() => hallPCClick(hallData[0].id)); await q.waitForTimeout(300);
  await L.esc(q); L.check('Esc снял выбор ПК, редактор открыт', await q.evaluate(() => hallEdit && !selPCId));
  await L.mouseBack(q); L.check('«назад» вышел из редактора', await q.evaluate(() => !hallEdit));
  L.check('ошибок JS нет (владелец)', errs.length === 0, errs);
  if (inc.d && inc.d.id) await L.api('POST', '/admin/incidents/' + inc.d.id + '/resolve', {}, await L.tok('admin1'));
  await b.close(); L.summary();
})().catch(e => { console.error(e); process.exit(1); });
