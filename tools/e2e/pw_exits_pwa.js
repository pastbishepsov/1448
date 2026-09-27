// Выходы гостевого PWA (web/app.html), docs/ВЫХОДЫ-НАЗАД.md: системный «назад» (page.goBack — как кнопка/жест
// Android) закрывает шторку, а не приложение; Esc и кнопка мыши на компьютере — то же. Запись истории — только
// за окно, открытое касанием; закрыли окно сами — запись снята; после «назад» записей не добавляем (иначе
// Chrome помечает их пропускаемыми). Кейс во время ленты — как ✕; анкета с правками сворачивается черновиком.
// Гость ExitPwa (регистрируется сам), телефонный вьюпорт 390×844.
const L = require('./_exits');
const G = process.env.G || 'ExitPwa';
(async () => {
  await L.ensureMoney(G, 150); await L.endSessions(G);
  const uid = await L.ensureGuest(G);
  const b = await L.browser();
  const ctx = await b.newContext({viewport: {width: 390, height: 844}, hasTouch: true, isMobile: true, deviceScaleFactor: 2});
  const p = await ctx.newPage();
  const errs = []; p.on('pageerror', e => errs.push(e.message));
  await p.goto(L.BASE + '/app'); await p.waitForTimeout(600);
  await p.fill('#nick', G); await p.fill('#pass', 'Guest12345'); await p.click('#authBtn');
  await p.waitForFunction(() => !document.getElementById('appView').classList.contains('hidden'), null, {timeout: 8000});
  await p.waitForTimeout(1200);
  const url0 = p.url();
  const st = () => p.evaluate(() => ({g: history.state && history.state.g1448 || 0, depth: backDepth}));
  const open = id => p.evaluate(id => document.getElementById(id).classList.contains('open') || document.getElementById(id).classList.contains('show'), id);

  console.log('— шторка брони');
  await p.click('button[onclick="openSheet()"]'); await p.waitForTimeout(300);
  L.check('бронь открыта касанием → запись истории', await open('sheetWrap') && (await st()).g === 1, await st());
  await p.goBack(); await p.waitForTimeout(400);
  L.check('системный «назад» закрыл шторку, приложение на месте', !await open('sheetWrap') && p.url() === url0, p.url());
  L.check('записей не осталось', (await st()).g === 0 && (await st()).depth === 0, await st());
  await p.click('button[onclick="openSheet()"]'); await p.waitForTimeout(300);
  await p.mouse.click(195, 60); await p.waitForTimeout(500); // тап мимо (по затемнению)
  L.check('тап мимо закрыл шторку и снял запись', !await open('sheetWrap') && (await st()).g === 0 && (await st()).depth === 0, await st());
  await p.click('button[onclick="openSheet()"]'); await p.waitForTimeout(300);
  await L.esc(p); await p.waitForTimeout(300);
  L.check('Esc закрыл шторку и снял запись', !await open('sheetWrap') && (await st()).g === 0, await st());

  console.log('— пакеты и пополнение');
  await p.click('button[data-tab="progress"]'); await p.waitForTimeout(900);
  await p.click('button[onclick="openPackShop()"]'); await p.waitForTimeout(400);
  L.check('витрина пакетов открыта', await open('packWrap'));
  await p.goBack(); await p.waitForTimeout(400);
  L.check('«назад» закрыл пакеты', !await open('packWrap') && p.url() === url0);
  const topBtn = await p.$('button[onclick="openTopup()"]');
  if (topBtn){ await topBtn.click(); await p.waitForTimeout(300);
    L.check('пополнение открыто', await open('topupWrap'));
    await p.goBack(); await p.waitForTimeout(400);
    L.check('«назад» закрыл пополнение', !await open('topupWrap') && p.url() === url0);
  } else console.log('  (кнопки «Пополнить» нет на экране — пропуск)');

  console.log('— кейс: «назад» во время ленты = ✕');
  await L.api('POST', '/admin/users/' + uid + '/grant', {type: 'case', case_tier: 'light', reason: 'проверка выходов'}, await L.tok('boss'));
  await p.click('button[data-tab="cases"]'); await p.waitForTimeout(400);
  await p.evaluate(() => loadCases && loadCases()); await p.waitForTimeout(900);
  await p.click('#casesList [onclick^="openCase"]'); await p.waitForTimeout(700);
  L.check('кейс открыт, лента крутится', await open('caseOv') && await p.evaluate(() => document.getElementById('coPrize').classList.contains('hidden')));
  await p.goBack(); await p.waitForTimeout(400);
  L.check('«назад» во время ленты закрыл окно (как ✕), приложение на месте', !await open('caseOv') && p.url() === url0);
  L.check('записей не осталось', (await st()).g === 0 && (await st()).depth === 0, await st());

  console.log('— анкета: «назад» сворачивает, правки — черновиком');
  await p.click('button[data-tab="progress"]'); await p.waitForTimeout(500);
  await p.click('#profileCard button[onclick="pfToggle()"]'); await p.waitForTimeout(700);
  L.check('анкета развёрнута → запись', await p.evaluate(() => pfOpen) && (await st()).g === 1, await st());
  await p.goBack(); await p.waitForTimeout(400);
  L.check('без правок: «назад» свернул, черновика нет', await p.evaluate(() => !pfOpen && !pfDraft));
  await p.click('#profileCard button[onclick="pfToggle()"]'); await p.waitForTimeout(700);
  const disc0 = await p.inputValue('#pf_discord');
  await p.fill('#pf_discord', 'draft#1448');
  await p.click('#profileCard .pfgames .pfg'); await p.waitForTimeout(300);
  L.check('тап по любимой игре не стёр набранное (баг перерисовки)', await p.inputValue('#pf_discord') === 'draft#1448');
  const fav = await p.evaluate(() => [...pfFavs]);
  await p.evaluate(() => { window.__toasts = []; new MutationObserver(() => window.__toasts.push(document.getElementById('toast').textContent)).observe(document.getElementById('toast'), {childList: true, subtree: true}); });
  await p.goBack(); await p.waitForTimeout(500);
  L.check('с правками: «назад» свернул анкету', await p.evaluate(() => !pfOpen) && p.url() === url0);
  L.check('черновик сохранён, тост показан', await p.evaluate(() => !!pfDraft && pfDraft.discord === 'draft#1448')
    && (await p.evaluate(() => window.__toasts)).some(t => /Анкета свёрнута/.test(t)), await p.evaluate(() => [pfDraft, window.__toasts]));
  await p.click('#profileCard button[onclick="pfToggle()"]'); await p.waitForTimeout(700);
  L.check('«Заполнить» вернул черновик (Discord и игры)', await p.inputValue('#pf_discord') === 'draft#1448' && JSON.stringify(await p.evaluate(() => pfFavs)) === JSON.stringify(fav));
  await p.click('#profileCard button.btn.ghost[onclick="pfToggle()"]'); await p.waitForTimeout(400);
  await p.click('#profileCard button[onclick="pfToggle()"]'); await p.waitForTimeout(700);
  L.check('«Свернуть» кнопкой — как раньше: правки сброшены', await p.inputValue('#pf_discord') === disc0);
  await L.esc(p); await p.waitForTimeout(300);
  L.check('Esc свернул анкету, записи нет', await p.evaluate(() => !pfOpen) && (await st()).g === 0, await st());

  console.log('— окно без касания (QR при запуске) и кнопка мыши');
  // окно открывается само через 6 с после последнего ввода (транзиентная активация Chrome живёт ~5 с;
  // evaluate у Playwright сам даёт активацию — поэтому открываем таймером, а смотрим после)
  await p.evaluate(() => setTimeout(() => { window.__ua = navigator.userActivation.isActive; document.getElementById('qrWrap').classList.add('open'); }, 6000));
  await p.waitForTimeout(6600);
  console.log('  (активация в момент открытия: ' + await p.evaluate(() => window.__ua) + ')');
  L.check('окно без касания: запись не добавлена (Chrome пометил бы её)', (await st()).g === 0, await st());
  await L.mouseBack(p, 195, 300);
  L.check('кнопка «назад» мыши закрыла окно без записи и не увела', !await open('qrWrap') && p.url() === url0, p.url());
  await p.click('#instBtn'); await p.waitForTimeout(300);
  await L.mouseBack(p, 195, 300);
  L.check('кнопка мыши при записи: окно закрыто, запись снята', !await open('installWrap') && p.url() === url0 && (await st()).g === 0, await st());
  await p.evaluate(() => document.getElementById('pwdGate').classList.remove('hidden'));
  await L.esc(p); L.check('экран смены пароля: Esc не закрывает', await p.evaluate(() => !document.getElementById('pwdGate').classList.contains('hidden')));
  await p.evaluate(() => document.getElementById('pwdGate').classList.add('hidden'));
  L.check('ошибок JS нет', errs.length === 0, errs);
  // ничего не открыто — «назад» остаётся браузерным (приложение не держит гостя)
  await L.mouseBack(p, 195, 300);
  L.check('ничего не открыто: «назад» мыши — обычный переход браузера', p.url() !== url0, p.url());
  await b.close(); L.summary();
})().catch(e => { console.error(e); process.exit(1); });
