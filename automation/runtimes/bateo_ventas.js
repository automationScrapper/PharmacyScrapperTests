// Runtime helpers for the Bateo Ventas screen

const PATH = '/web/app.php/bateo_ventas';

function ymd(date) {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

function computeDateRange(now = new Date()) {
  const start = new Date(now.getFullYear(), now.getMonth(), 1);
  const end = new Date(now);
  // end should be the day before the consultation day
  end.setDate(end.getDate() - 1);
  // Avoid invalid ranges (e.g., consultation on the 1st)
  if (end < start) end.setTime(start.getTime());
  return { start, end };
}

// Open directly via URL (legacy)
async function openDirect(page, baseUrl) {
  const url = new URL(PATH, baseUrl).toString();
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  // Ensure date inputs exist
  await page.waitForSelector('#inputBateoVentaFechaInicio', { state: 'visible' });
  await page.waitForSelector('#inputBateoVentasFechaFin', { state: 'visible' });
}

// Click the correct Ventas card button on the dashboard
async function clickVentasButton(page) {
  const ventasBtn = 'div.col-6.col-md-4.col-lg-4.col-xl-3.mb-2 a#buttonOpenModulo[href="ventas"]';
  await page.waitForSelector(ventasBtn, { state: 'visible', timeout: 15000 });
  await page.click(ventasBtn);
  await page.waitForLoadState('domcontentloaded').catch(() => {});
  await page.waitForLoadState('networkidle').catch(() => {});
}

// Open by clicking the dashboard button after login
async function openViaButton(page) {
  await clickVentasButton(page);
  // Ensure the Ventas module menu is present
  await page.waitForSelector('li#modulo_ventas', { state: 'attached', timeout: 15000 });
}

async function ensureExpanded(page, liSelector) {
  await page.waitForSelector(liSelector, { state: 'attached', timeout: 15000 });
  const isOpen = await page.$eval(liSelector, (el) => el.classList.contains('menu-open'))
    .catch(() => false);
  if (!isOpen) {
    await page.click(`${liSelector} > a.nav-link`);
    await page.waitForFunction((sel) => {
      const el = document.querySelector(sel);
      return !!el && el.classList.contains('menu-open');
    }, liSelector);
  }
}

// Open by clicking dashboard "Ventas" button, then the left-nav "Bateo de ventas"
async function openViaDashboardMenu(page) {
  await clickVentasButton(page);

  // Expand Ventas module and Reportes category
  await ensureExpanded(page, 'li#modulo_ventas');
  await ensureExpanded(page, 'li#categoria_ventasreportes');

  // Click Bateo de ventas item
  const bateoLink = 'li#menubateo_ventas a.nav-link[href="bateo_ventas"]';
  await page.waitForSelector(bateoLink, { state: 'visible', timeout: 15000 });
  await page.click(bateoLink);

  // Wait for the Bateo de ventas content to be ready
  await page.waitForLoadState('domcontentloaded').catch(() => {});
  await page.waitForLoadState('networkidle').catch(() => {});
  await page.waitForSelector('#inputBateoVentaFechaInicio', { state: 'visible', timeout: 15000 });
  await page.waitForSelector('#inputBateoVentasFechaFin', { state: 'visible', timeout: 15000 });
}

async function setDateRange(page, { start, end }) {
  const startStr = ymd(start);
  const endStr = ymd(end);

  // Helper to set value and fire events to satisfy reactive UIs
  async function applyDateValue(selector, value) {
    await page.waitForSelector(selector, { state: 'visible', timeout: 15000 });
    // Try fill first
    await page.focus(selector).catch(() => {});
    await page.fill(selector, value);
    // Ensure frameworks catch updates (dispatch input/change)
    await page.$eval(
      selector,
      (el, v) => {
        el.value = v;
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
        if (document.activeElement === el) el.blur();
      },
      value
    );
  }

  await applyDateValue('#inputBateoVentaFechaInicio', startStr);
  await applyDateValue('#inputBateoVentasFechaFin', endStr);
}

async function consultar(page) {
  await page.waitForSelector('#buttonBateoVentasList', { state: 'visible', timeout: 15000 });
  // Click and wait for network to settle; if no requests, this returns quickly
  await Promise.all([
    page.waitForLoadState('networkidle').catch(() => {}),
    page.click('#buttonBateoVentasList'),
  ]);
}

async function triggerExport(page) {
  await page.waitForSelector('#buttonBateoVentasExport', { state: 'visible', timeout: 15000 });
  const [download] = await Promise.all([
    page.waitForEvent('download', { timeout: 60000 }),
    page.click('#buttonBateoVentasExport'),
  ]);
  return download;
}

module.exports = { PATH, computeDateRange, openDirect, openViaButton, openViaDashboardMenu, clickVentasButton, setDateRange, consultar, triggerExport };
