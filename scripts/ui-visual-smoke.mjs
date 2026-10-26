import { createServer } from "../frontend/node_modules/vite/dist/node/index.js";
import { chromium } from "../frontend/node_modules/playwright/index.mjs";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import assert from "node:assert/strict";

// Fixtures exercise the actual React UI without calling the desktop bridge or personal providers.
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const artifacts = path.join(root, ".tools/check-results/desktop-serene");
const fixtures = JSON.parse(await readFile(path.join(root, "frontend/src/bridge-fixtures.json"), "utf8")).samples;
const access = { ...fixtures.accessStatus, setupPending: false, enabled: false, locked: false, error: "", warning: "", language: "en", theme: "light" };
const field = (key, label, secret = false) => ({ key, label, secret, required: true, advanced: false, default: "", file: false });
const connection = { ...fixtures.connectionUnknown, id: "visual-first", name: "Documents", provider: "webdav", letter: "S", path: "Documents", mounted: true, busy: false, recovery: false, pending: 0, error: "", operation: "",
  uploads: { active: 0, queued: 0, cacheErrors: 0 }, cache: { usedBytes: 1024, pendingBytes: 0, checkedAt: null, error: "" },
  transportSecurity: { state: "encrypted", protocol: "https", source: "configuration", checkedAt: null } };
const snapshot = { ...fixtures.snapshotEmpty, access, settings: { ...fixtures.snapshotEmpty.settings, theme: "light", language: "en" },
  cacheLocation: "C:\\Synthetic\\Cache", freeBytes: 124 * 1024 ** 3,
  providers: [{ ...fixtures.providerEmpty, id: "webdav", name: "WebDAV", backend: "webdav", transportEncryption: "optional", available: true, oauth: false,
    fields: [field("url", "WebDAV HTTPS URL"), field("user", "Username"), field("pass", "Password / app password", true)] }],
  connections: [connection, { ...connection, id: "visual-second", name: "Team library", letter: "T", path: "Projects", mounted: false },
    { ...connection, id: "visual-long", name: "Shared documents and administrative projects with a deliberately long name", letter: "U", path: "Long remote directory/".repeat(8), pending: null, recovery: true }],
  importDrafts: [], letters: ["V", "W"], winFsp: true, engineError: "", settingsError: "", desktopWarning: "", startupWarning: "", maintenanceError: "", recoveryHold: false };
const apiSource = `window.visualSnapshot = ${JSON.stringify(snapshot)};
const current = () => window.visualSnapshot;
export const api = {
  accessStatus: async () => current().access, snapshot: async () => current(), desktopVisible: async () => true,
  onDesktopVisibility: cb => { window.visualVisibility = cb; return () => {}; }, onAccessChanged: cb => {window.visualAccess = cb; return () => {};},
  preferenceDefaults: async () => ({ ...current().settings, cacheGiB: 2, cacheMaxAgeHours: 24, writeBackSeconds: 5 }), restoreDefaultCacheLocation: async () => current().cacheLocation,
  proxy: async () => ({ address: '', username: '', password: '', hasPassword: false }), browsers: async () => [{ id: '', name: 'Default browser' }],
  fields: async () => ({ values: {}, transport: '' }), settings: async settings => { current().settings = settings; },
  begin: async () => 'synthetic-draft', status: async () => ({busy:false,done:true,error:''}), folders: async () => [{name:'Documents',path:'Documents'}], end: async () => {},
  about: async () => ({appVersion:'fixture',rcloneVersion:'fixture',windowsVersion:'fixture',winFspVersion:'fixture',webView2Version:'fixture',architecture:'amd64'}),
  diagnostics: async () => 'Synthetic diagnostic report. No credentials or personal paths.', copyDiagnostics: async () => {},
  inspectFileAccess: async () => ({path:'S:\\\\Synthetic.txt',state:'unknown',checkedAt:new Date().toISOString(),processes:[]}),
  previewImport: async () => ({id:'synthetic-import',document:{format:'sdrive',version:1,connections:[{name:'Imported documents',provider:'webdav',path:'Documents',letter:'V',mountMode:'network',values:{},readOnly:false,autoMount:false}],preferences:current().settings}}),
  lock: async () => {current().access={...current().access,enabled:true,locked:true};window.visualAccess(true);},
  unlockAccess: async () => {current().access.locked=false;},
};
export const mountTarget = c => ({letter:c.letter,folder:c.mountFolder||'',mode:c.mountMode||''});
export const validationError = () => undefined;`;
const server = await createServer({ root: path.join(root, "frontend"), configFile: false, server: { host: "127.0.0.1", port: 0 },
  plugins: [{ name: "visual-fixture", enforce: "pre", transform(code, id) {
    if (id.replaceAll("\\", "/").endsWith("/src/api.ts")) return { code: apiSource, map: null };
  } }] });
let browser, page;
const errors = [];
try {
  await mkdir(artifacts, { recursive: true });
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "running", type: "browser-fixture" }));
  await server.listen();
  browser = await chromium.launch(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
    ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : { channel: "msedge" });
  page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.setDefaultTimeout(8000);
  page.on("pageerror", e => errors.push(e.message));
  await page.goto(server.resolvedUrls.local[0]);
  await page.getByRole("heading", { name: "Documents", exact: true }).waitFor();
  const capture = name => page.screenshot({ path: path.join(artifacts, name + ".png"), fullPage: true });
  const bounds = async () => {
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "Page horizontal overflow");
    const dialog = page.getByRole("dialog").last();
    if (await dialog.count()) {
      assert(await dialog.evaluate(e => e.scrollWidth <= e.clientWidth + 1), "Dialog horizontal overflow");
      assert(await dialog.evaluate(e => { const b = e.getBoundingClientRect(); return Math.abs(b.right - innerWidth) <= 1 && b.width <= (e.classList.contains("with-navigation") ? 800 : 640) && b.top >= (innerWidth < 640 ? 8 : 16) && b.bottom <= innerHeight - (innerWidth < 640 ? 8 : 16) + 1; }), "Drawer must align to the right edge");
      assert(await dialog.locator(".dialog-footer").evaluate(e => { const b = e.getBoundingClientRect(); return b.top >= 0 && b.bottom <= innerHeight + 1; }), "Drawer actions clipped");
    }
  };
  const details = async () => {
    await page.locator(".drive > .actions > details > summary").first().click();
    await page.getByRole("button", { name: "Drive details", exact: true }).click();
  };
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1180, height: 800 }, { width: 850, height: 620 }, { width: 420, height: 740 }]) {
    await page.setViewportSize(viewport);
    await bounds();
    const card = page.locator(".drive").first();
    assert(await card.evaluate(e => {
      const letter = e.querySelector(".letter").getBoundingClientRect(), info = e.querySelector(".drive-info").getBoundingClientRect();
      return info.left >= letter.right && info.top >= e.getBoundingClientRect().top;
    }), "U1 information must stay to the right of the letter");
    assert(await card.getByRole("button", { name: "Disconnect", exact: true }).evaluate(e => e.getBoundingClientRect().height >= 44));
    await capture("units-" + viewport.width);
    await details();
    for (const topic of ["Remote destination", "Connection options", "Upload details", "Local cache", "Remote capacity", "Connection security", "Alerts and recovery"]) {
      const tab = page.getByRole("tab", { name: topic, exact: true });
      const menu = page.locator(".dialog-navigation .navigation-toggle");
      if (await menu.isVisible() && !await tab.isVisible()) await menu.click();
      await tab.click(); await bounds();
      assert.equal(await page.locator(".navigation-items button[aria-selected=true]").textContent().then(text => text.replace(/ !$/, "").trim()), topic);
    }
    await capture("details-" + viewport.width);
    await page.getByRole("button", { name: "Close", exact: true }).click();
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: "+ New connection", exact: true }).click();
  await bounds(); await capture("connection-service");
  const help = page.getByRole("button", { name: "Help for Windows drive letter", exact: true });
  await help.hover();
  const tooltip = page.getByRole("tooltip");
  await tooltip.waitFor();
  assert(await tooltip.evaluate(element => {
    const box = element.getBoundingClientRect(), dialog = element.closest("dialog").getBoundingClientRect();
    return box.left >= dialog.left && box.right <= dialog.right && box.top >= dialog.top && box.bottom <= dialog.bottom;
  }), "Help must fit inside the modal top layer");
  assert(await tooltip.evaluate(element => {
    const box = element.getBoundingClientRect();
    return element.contains(document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2));
  }), "Help must remain visible above the scrolling body");
  await capture("connection-help");
  await page.keyboard.press("Escape");
  assert.equal(await tooltip.count(), 0, "Escape dismisses help");
  assert.equal(await page.getByRole("dialog").count(), 1, "Dismissing help preserves setup");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await bounds(); await capture("connection-access");
  await page.getByRole("button", { name: "Close setup", exact: true }).click();
  await page.locator(".drive > .actions > details > summary").first().click();
  await page.getByRole("button", { name: "Applications using a file", exact: true }).click();
  await page.getByRole("button", { name: "Select mounted file", exact: true }).click();
  await page.getByText("File access information is unknown.", { exact: false }).waitFor();
  await bounds(); await capture("file-access");
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByRole("button", { name: /Preferences/ }).click();
  for (const tab of ["General", "Appearance", "Cache", "Network", "Configuration"]) {
    await page.getByRole("tab", { name: tab, exact: true }).click(); await bounds(); await capture("preferences-" + tab.toLowerCase());
  }
  await page.getByRole("button", { name: "Import configuration", exact: true }).click();
  assert.equal(await page.getByRole("dialog").count(), 2, "Import opens above Preferences");
  await bounds(); await capture("import-review");
  await page.getByRole("dialog", { name: "Review imported configuration", exact: true }).press("Escape");
  assert.equal(await page.getByRole("dialog").count(), 1);
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "Application lock", exact: true }).click();
  await bounds(); await capture("access-settings");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "About SDrive", exact: true }).click();
  await page.getByRole("button", { name: "View diagnostics", exact: true }).click();
  await page.getByLabel("Diagnostic report (technical text in English)").waitFor();
  await bounds(); await capture("diagnostics");
  await page.getByRole("button", { name: "Close", exact: true }).click();
  for (const language of ["en", "pt-BR", "es", "fr", "de", "it", "zh-CN"]) {
    await page.evaluate(language => {
      window.visualSnapshot.settings.language = language;
      window.visualVisibility(false); window.visualVisibility(true);
    }, language);
    await page.waitForFunction(language => document.documentElement.lang === language, language);
    await page.setViewportSize({ width: 850, height: 620 }); await bounds();
    await capture("language-" + language);
    if (language === "pt-BR") {
      await page.setViewportSize({ width: 1440, height: 900 });
      await capture("units-pt-BR");
    }
  }
  await page.evaluate(() => { window.visualSnapshot.settings.language = "en"; window.visualVisibility(false); window.visualVisibility(true); });
  await page.waitForFunction(() => document.documentElement.lang === "en");
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: /Preferences/ }).click();
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  await page.getByLabel("Theme").selectOption("dark");
  await page.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  await bounds(); await capture("dark-drawer");
  await page.emulateMedia({ forcedColors: "active" }); await bounds(); await capture("forced-colors");
  await page.emulateMedia({ forcedColors: "none" });
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.deepEqual(errors, []);
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "passed", type: "browser-fixture", checks: "U1, 44px actions, long paths, four viewport sizes, seven topics, vertical preference sections, setup, contextual help and Escape, nested import, access settings, diagnostics, seven languages, dark and forced colors", errors }, null, 2));
  console.log("Visual UI smoke passed; screenshots: " + artifacts);
} catch (error) {
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "failed", type: "browser-fixture", reason: error.message }, null, 2));
  if (page && !page.isClosed()) await page.screenshot({ path: path.join(artifacts, "failure.png"), mask: [page.locator("[data-sensitive]")] }).catch(() => {});
  throw error;
} finally { await browser?.close(); await server.close(); }
