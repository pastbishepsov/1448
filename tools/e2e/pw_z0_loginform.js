// З0-и5 «Форма входа без чужого пароля» (З-Р36): после входа пароль стёрт, после «Выйти» пусты и ник,
// и пароль; ник подставляется из «недавно входили на этом компьютере» (localStorage, свежие первыми);
// неверный пароль и гостевой аккаунт в список не попадают. Персонал сида, гость Guest31; ничего не меняет.
const z = require('./_z');
(async () => {
  const page = await z.openAdmin('admin1');
  const form = () => page.evaluate(() => ({ nick: document.getElementById('nick').value, pass: document.getElementById('pass').value,
    chips: [...document.querySelectorAll('#nickChips button')].map(b => b.textContent),
    list: [...document.querySelectorAll('#nickRecent option')].map(o => o.value), focus: document.activeElement && document.activeElement.id }));
  const logout = async () => { await page.evaluate(() => logout()); await z.idle(page); }; // З19-и14: после записи — ждём страницу, а не паузу
  const enter = async (nick, pass) => {
    await page.fill('#nick', nick); await page.fill('#pass', pass); await page.keyboard.press('Enter'); await z.idle(page);
  };

  z.check('после входа пароль в форме стёрт', (await form()).pass === '');
  await logout();
  let f = await form();
  z.check('после «Выйти» ник и пароль пусты', f.nick === '' && f.pass === '', JSON.stringify(f));
  z.check('в «недавних» — admin1, и в подсказке ввода тоже', f.chips.join() === 'admin1' && f.list.join() === 'admin1', JSON.stringify(f));
  await page.click('#nickChips button:has-text("admin1")');
  f = await form();
  z.check('чип подставляет ник и ставит курсор в пароль', f.nick === 'admin1' && f.focus === 'pass', JSON.stringify(f));
  await page.fill('#pass', z.STAFF_PASS); await page.keyboard.press('Enter'); await z.idle(page);
  z.check('вход по чипу проходит', await page.evaluate(() => !!me && me.nickname === 'admin1'));
  await logout();
  await enter('admin2', z.STAFF_PASS);
  await logout();
  z.check('свежий вход — первым: admin2, admin1', (await form()).chips.join() === 'admin2,admin1', (await form()).chips.join());
  await enter('boss', 'Wrong-pass-0');
  await enter('Guest31', z.GUEST_PASS);
  f = await form();
  z.check('неверный пароль и гостевой аккаунт в список не попали', f.chips.join() === 'admin2,admin1', JSON.stringify(f));
  await z.shot(page, 'z0_loginform');
  await z.done();
})().catch(z.crash);
