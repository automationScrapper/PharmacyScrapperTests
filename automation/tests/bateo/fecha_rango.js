// Test: Login then set date range on bateo_ventas
// Run with: node tests/bateo/fecha_rango.js

const assert = require('node:assert');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const path = require('node:path');
const { chromium } = require('playwright');
const { login } = require('../../core/login');
const { computeDateRange, openViaDashboardMenu, setDateRange, consultar, triggerExport } = require('../../runtimes/bateo_ventas');

(async () => {
  // Config via env with safe defaults (replace envs in CI)
  const baseUrl = process.env.ERP_BASE_URL || 'http://erpvm.kurigage.com';
  const username = process.env.ERP_USER
  const password = process.env.ERP_PASS 

  const headless = process.env.HEADLESS ? ['1','true','yes','on'].includes(String(process.env.HEADLESS).toLowerCase()) : false;
  const browser = await chromium.launch({ headless });
  const context = await browser.newContext({ acceptDownloads: true });
  const page = await context.newPage();
  try {
    console.log(`[RUN] Headless=${headless}`);
    console.log('[STEP] Iniciando login...');
    await login(page, { baseUrl, username, password });

    // Open Bateo Ventas by clicking Ventas, then the left-nav "Bateo de ventas"
    console.log('[STEP] Abriendo módulo Ventas > Bateo de ventas...');
    await openViaDashboardMenu(page);
    console.log('[STEP] Módulo Bateo abierto');

    // Allow overriding the base date via env QUERY_DATE (YYYY-MM-DD)
    const queryDateStr = process.env.QUERY_DATE;
    const baseDate = queryDateStr ? new Date(queryDateStr) : new Date();
    const { start, end } = computeDateRange(baseDate);
    const pad = (n) => String(n).padStart(2, '0');
    const fmt = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    console.log(`[STEP] Estableciendo rango: ${fmt(start)} a ${fmt(end)}`);
    await setDateRange(page, { start, end });

    // Click Consultar to load results for the selected range
    console.log('[STEP] Consultando resultados...');
    await consultar(page);

    // Trigger Export and save the downloaded file into automation/downloads
    console.log('[STEP] Exportando reporte...');
    const download = await triggerExport(page);
    const suggested = download.suggestedFilename();
    console.log(`[INFO] Archivo sugerido por el sitio: ${suggested}`);
    const downloadsDir = path.resolve(__dirname, '..', '..', 'downloads');
    if (!fs.existsSync(downloadsDir)) {
      await fsp.mkdir(downloadsDir, { recursive: true });
    }
    // Rename the file to include the selected date range
    const rangeStartStr = fmt(start);
    const rangeEndStr = fmt(end);
    const ext = path.extname(suggested) || '.xlsx';
    const base = path.basename(suggested, ext) || 'export';
    const saveName = `${base}_${rangeStartStr}_a_${rangeEndStr}${ext}`;
    const savePath = path.join(downloadsDir, saveName);
    await download.saveAs(savePath);
    const stat = await fsp.stat(savePath);
    console.log(`[DOWNLOAD] saved to: ${savePath} (${stat.size} bytes)`);
    console.log('[DONE] Export y guardado completado');

    // Basic assertions: values are placed in the inputs
    const startVal = await page.inputValue('#inputBateoVentaFechaInicio');
    const endVal = await page.inputValue('#inputBateoVentasFechaFin');

    const expectStart = `${start.getFullYear()}-${pad(start.getMonth() + 1)}-${pad(start.getDate())}`;
    const expectEnd = `${end.getFullYear()}-${pad(end.getMonth() + 1)}-${pad(end.getDate())}`;

    assert.strictEqual(startVal, expectStart, 'start date not set correctly');
    assert.strictEqual(endVal, expectEnd, 'end date not set correctly');

    console.log('[PASS] bateo/fecha_rango');
    console.log('[DONE] Flujo completo OK');
    await context.close();
    await browser.close();
    process.exit(0);
  } catch (err) {
    console.error('[FAIL] bateo/fecha_rango:', err && err.message ? err.message : err);
    // Capture diagnostics
    try {
      const downloadsDir = path.resolve(__dirname, '..', '..', 'downloads');
      if (!fs.existsSync(downloadsDir)) {
        await fsp.mkdir(downloadsDir, { recursive: true });
      }
      const ts = Date.now();
      const png = path.join(downloadsDir, `error-${ts}.png`);
      const html = path.join(downloadsDir, `error-${ts}.html`);
      await page.screenshot({ path: png, fullPage: true }).catch(() => {});
      const content = await page.content().catch(() => '');
      if (content) await fsp.writeFile(html, content).catch(() => {});
      console.error(`[DEBUG] Saved diagnostics to ${png} and ${html}`);
    } catch {}
    try { await context.close(); } catch {}
    await browser.close();
    process.exit(1);
  }
})();
