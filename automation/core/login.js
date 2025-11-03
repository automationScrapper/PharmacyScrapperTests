// Core login flow using pure Playwright
// Usage: await login(page, { baseUrl, username, password })

async function login(page, { baseUrl, username, password }) {
  const loginUrl = new URL('/web/app.php/Login', baseUrl).toString();
  console.log(`[STEP] Navegando a Login: ${loginUrl}`);
  await page.goto(loginUrl, { waitUntil: 'domcontentloaded' });

  // Fill credentials
  await page.waitForSelector('#usuario', { state: 'visible', timeout: 30000 });
  await page.fill('#usuario', String(username ?? ''));
  await page.fill('#password', String(password ?? ''));

  console.log('[STEP] Llenando credenciales');
  // Submit: prefer clicking the button, but also send Enter
  const submitBtn = await page.$('#buttonAuth');
  if (submitBtn) {
    console.log('[STEP] Enviando formulario de login (click)');
    await submitBtn.click().catch(() => {});
  }
  console.log('[STEP] Enviando formulario de login (Enter)');
  await page.press('#password', 'Enter').catch(() => {});

  // Give the app some time to process login and settle
  await page.waitForLoadState('domcontentloaded').catch(() => {});
  await page.waitForLoadState('networkidle').catch(() => {});

  // Helper wrappers that tolerate navigations
  async function safeHasSelector(selector) {
    try {
      const el = await page.$(selector);
      return !!el;
    } catch {
      return false;
    }
  }
  async function safeFirstText(selector) {
    try {
      const el = await page.$(selector);
      if (!el) return '';
      const txt = await el.textContent();
      return (txt || '').trim();
    } catch {
      return '';
    }
  }

  // Wait up to 30s for either:
  //  - URL no longer contains /Login (case-insensitive), OR
  //  - A known post-login element is present (Ventas button/menu)
  const deadline = Date.now() + 30_000;
  let loggedIn = false;
  while (Date.now() < deadline) {
    const urlStr = page.url();
    if (!/\/login\b/i.test(urlStr)) {
      loggedIn = true;
      break;
    }

    if (await safeHasSelector('a#buttonOpenModulo[href="ventas"]') || await safeHasSelector('li#modulo_ventas')) {
      loggedIn = true;
      break;
    }

    // Detect visible error messages early
    const errMsg = await safeFirstText('.alert-danger, .alert[role="alert"], #loginError, .swal2-html-container');
    if (errMsg) {
      throw new Error(`Login failed: ${errMsg}`);
    }

    await page.waitForTimeout(500);
  }

  if (!loggedIn) {
    const current = page.url();
    throw new Error(`Login did not navigate away from /Login (url=${current})`);
  }
  console.log('[STEP] Login OK');
}

module.exports = { login };
