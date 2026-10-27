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
const connection = { ...fixtures.connectionUnknown, id: "visual-first", name: "Documents", provider: "webdav", letter: "S", serverPath: "https://files.example.test/dav/Documents", path: "Documents", mounted: true, busy: false, recovery: false, pending: 0, error: "", operation: "",
  uploads: { active: 0, queued: 0, cacheErrors: 0 }, cache: { usedBytes: 1024, pendingBytes: 0, checkedAt: null, error: "" },
  transportSecurity: { state: "encrypted", protocol: "https", source: "configuration", checkedAt: null } };
const snapshot = { ...fixtures.snapshotEmpty, access, settings: { ...fixtures.snapshotEmpty.settings, theme: "light", language: "en" },
  cacheLocation: "C:\\Synthetic\\Cache", freeBytes: 124 * 1024 ** 3,
  providers: [{ ...fixtures.providerEmpty, id: "webdav", name: "WebDAV", backend: "webdav", transportEncryption: "optional", available: true, oauth: false,
    fields: [field("url", "WebDAV HTTPS URL"), field("user", "Username"), field("pass", "Password / app password", true)] },
    { ...fixtures.providerEmpty, id: "gcs", name: "Google Cloud Storage", backend: "google cloud storage", transportEncryption: "required", available: true, oauth: true,
      fields: [{ ...field("service_account_file", "Service account JSON file (optional)"), file: true }] }],
  connections: [connection, { ...connection, id: "visual-second", name: "Team library", letter: "", mountFolder: "C:\\Synthetic\\Team\\" + "Long local directory\\".repeat(12), cardStyle: "violet", serverPath: "https://files.example.test/dav/Projects", path: "Projects", mounted: false },
    { ...connection, id: "visual-long", name: "Shared documents and administrative projects with a deliberately long name", letter: "U", serverPath: "https://files.example.test/dav/" + "Long remote directory/".repeat(8), path: "Long remote directory/".repeat(8), pending: null, recovery: true }],
  importDrafts: [], letters: ["V", "W"], winFsp: true, engineError: "", settingsError: "", desktopWarning: "", startupWarning: "", maintenanceError: "", recoveryHold: false };
const apiSource = `window.visualSnapshot = ${JSON.stringify(snapshot)};
const current = () => window.visualSnapshot;
export const api = {
  accessStatus: async () => current().access, snapshot: async () => structuredClone(current()), desktopVisible: async () => true,
  onDesktopVisibility: cb => { window.visualVisibility = cb; return () => {}; }, onAccessChanged: cb => {window.visualAccess = cb; return () => {};},
  preferenceDefaults: async () => ({ ...current().settings, cacheGiB: 2, cacheMaxAgeHours: 24, writeBackSeconds: 5 }), restoreDefaultCacheLocation: async () => current().cacheLocation,
  proxy: async () => ({ address: '', username: '', password: '', hasPassword: false }), browsers: async () => [{ id: '', name: 'Default browser' }],
  updateDriveAppearance: async (id, appearance) => { Object.assign(current().connections.find(c => c.id === id), appearance); },
  fields: async () => ({ values: {}, transport: '' }), settings: async settings => { current().settings = settings; },
  begin: async () => 'synthetic-draft', status: async () => ({busy:false,authenticated:true,complete:false,error:''}), folders: async () => [{name:'Documents',path:'Documents'}], end: async () => {},
  selectDirectory: async () => 'C:\\\\Synthetic\\\\NewCache', selectFile: async () => ${JSON.stringify("C:\\Synthetic\\Credentials\\" + "long-directory\\".repeat(8) + "account.json")},
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
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.addInitScript(() => {
    window.visualMotion = [];
    document.addEventListener("animationstart", event => window.visualMotion.push({ name: event.animationName, target: event.target.className }));
  });
  page.setDefaultTimeout(8000);
  page.on("pageerror", e => errors.push(e.message));
  await page.goto(server.resolvedUrls.local[0]);
  await page.getByRole("heading", { name: "Documents", exact: true }).waitFor();
  const settle = () => page.evaluate(() => Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {}))));
  const capture = async name => { await settle(); return page.screenshot({ path: path.join(artifacts, name + ".png"), fullPage: true }); };
  const bounds = async () => {
    await settle();
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "Page horizontal overflow");
    const dialog = page.getByRole("dialog").last();
    if (await dialog.count()) {
      assert(await dialog.evaluate(e => e.scrollWidth <= e.clientWidth + 1), "Dialog horizontal overflow");
      assert(await dialog.evaluate(e => { const b = e.getBoundingClientRect(); return Math.abs(b.right - innerWidth) <= 1 && b.width <= (e.classList.contains("with-navigation") ? 800 : 640) && b.top >= (innerWidth < 640 ? 8 : 16) && b.bottom <= innerHeight - (innerWidth < 640 ? 8 : 16) + 1; }), "Drawer must align to the right edge");
      assert(await dialog.locator(".dialog-footer").evaluate(e => { const b = e.getBoundingClientRect(); return b.top >= 0 && b.bottom <= innerHeight + 1; }), "Drawer actions clipped");
    }
    for (const row of await page.locator(".field-action:visible").all()) {
      assert(await row.evaluate(element => {
        const input = element.querySelector("input,select").getBoundingClientRect();
        const button = element.querySelector(":scope > button").getBoundingClientRect();
        return input.width >= 80 && button.height >= 44 && button.left >= input.right && Math.abs(input.bottom - button.bottom) <= 1;
      }), "Field actions must share a horizontal row with usable input space");
    }
  };
  const details = async () => {
    await page.locator(".drive > .actions > details > summary").first().click();
    await page.getByRole("button", { name: "Drive details", exact: true }).click();
  };
  const firstCard = page.locator(".drive").first();
  assert.equal(await firstCard.locator(".status-tag.connected").textContent(), "Connected");
  assert(await firstCard.locator(".status-tag, .type-tag").evaluateAll(tags => tags.every(tag => {
    const style = getComputedStyle(tag);
    return style.borderTopWidth === "1px" && style.borderTopStyle === "solid" && style.borderTopColor !== "rgba(0, 0, 0, 0)";
  })), "Tags need a subtle visible border");
  assert.equal(await page.locator(".drive").nth(1).locator(".letter svg").count(), 1);
  assert.equal(await firstCard.locator(".drive-metadata .type-tag + .server-path").textContent(), "https://files.example.test/dav/Documents");
  const appearance = async () => {
    await firstCard.locator("summary").click();
    await page.getByRole("button", { name: "Customize appearance", exact: true }).click();
  };
  await appearance();
  await page.getByLabel("Teal", { exact: true }).check();
  const image = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAQAAAABACAYAAAD1Xam+AAAA3klEQVR4nO3UoQEAIAzAME7jND4fZ0w0Ir6q574ZoOlsBwB7DADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCDADCPlFxikI+oz1nAAAAAElFTkSuQmCC", "base64");
  await page.getByLabel("Drive image", { exact: true }).setInputFiles({ name: "icon.png", mimeType: "image/png", buffer: image });
  await page.locator(".appearance-preview img").waitFor();
  assert(await page.locator(".appearance-preview img").evaluate(img => img.complete && img.naturalWidth === 128 && img.naturalHeight === 32));
  await bounds(); await capture("appearance-preview");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page.waitForFunction(() => document.querySelector(".drive").dataset.cardStyle === "teal");
  assert(await firstCard.locator(".letter img").evaluate(img => img.complete && img.naturalWidth > 0));
  await appearance();
  await page.getByLabel("Amber", { exact: true }).check();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.equal(await firstCard.getAttribute("data-card-style"), "teal");
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1180, height: 800 }, { width: 850, height: 620 }, { width: 420, height: 740 }]) {
    await page.setViewportSize(viewport);
    await bounds();
    assert(await page.locator(".server-path, .mount-folder > span").evaluateAll(paths => paths.every(element => {
      const style = getComputedStyle(element);
      const box = element.getBoundingClientRect();
      return style.whiteSpace === "nowrap" && style.textOverflow === "ellipsis" && box.height <= parseFloat(style.lineHeight) + 1 && element.title === element.textContent;
    })), "All card paths must stay on one line and preserve the complete hover text");
    assert(await page.locator(".mount-folder strong").evaluate(element => parseInt(getComputedStyle(element).fontWeight) >= 600), "Mount prefix must be bold");
    assert(await page.locator(".drive-credits").evaluate(element => {
      const box = element.getBoundingClientRect(), hint = element.previousElementSibling.getBoundingClientRect();
      return getComputedStyle(element).fontSize === "11px" && box.top >= hint.bottom && getComputedStyle(element).textAlign === "right";
    }), "Credits must be discreet and below content");
    assert.equal(await firstCard.getByRole("button", { name: "Open", exact: true }).getAttribute("title"), "Open in Explorer");
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
  for (const width of [1440, 1180, 850, 420]) {
    await page.setViewportSize({ width, height: width === 420 ? 740 : 800 });
    await page.getByRole("button", { name: "+ New connection", exact: true }).click();
    await page.getByLabel("Mount into").selectOption("folder");
    await page.getByRole("button", { name: "Choose parent directory", exact: true }).click();
    await bounds();
    assert((await page.getByLabel("Local mount folder", { exact: true }).inputValue()).endsWith("\\SDrive"));
    await page.getByLabel("Connection type").selectOption("gcs:serviceAccount");
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await page.getByRole("button", { name: "Select file…", exact: true }).click();
    await bounds();
    assert((await page.getByLabel("Service account JSON file (optional)").inputValue()).endsWith("account.json"));
    const advanced = page.locator(".advanced-options");
    assert.equal(await advanced.locator("details").count(), 0, "Advanced groups must not nest disclosures");
    assert.equal(await advanced.evaluate(element => element.closest("fieldset")), null, "Advanced settings must be outside primary fields");
    await advanced.locator("summary").click();
    for (const group of ["Files and cache", "Network", "Drive behavior"]) await advanced.getByRole("group", { name: group, exact: true }).waitFor();
    assert.equal(await page.getByLabel("Show hidden files", { exact: true }).isChecked(), true);
    await page.getByLabel("Show hidden files", { exact: true }).uncheck();
    await bounds(); await capture("advanced-groups-" + width);
    const metadata = page.getByRole("spinbutton", { name: "Directory metadata lifetime (seconds)", exact: true });
    await metadata.fill("-1");
    await advanced.locator("summary").click();
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    assert.equal(await advanced.getAttribute("open"), "", "Invalid advanced field must reveal its group");
    assert(await metadata.evaluate(element => document.activeElement === element), "Invalid advanced field must receive focus");
    await metadata.fill("300");
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await page.getByRole("button", { name: "▱ Documents", exact: true }).waitFor();
    await page.getByText("Enter destination directly", { exact: true }).click();
    await bounds(); await capture("path-actions-" + width);
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await page.getByRole("button", { name: /Preferences/ }).click();
    const navigationToggle = page.locator(".dialog-navigation .navigation-toggle");
    if (await navigationToggle.isVisible()) await navigationToggle.click();
    await page.getByRole("tab", { name: "Cache", exact: true }).click();
    await bounds(); await capture("cache-actions-" + width);
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  assert(await page.evaluate(() => window.visualMotion.some(event => event.name === "panel-enter")), "Drawers must animate on entry");
  const menu = page.locator(".drive > .actions > details > summary").first();
  for (let opening = 0; opening < 2; opening++) {
    const count = await page.evaluate(() => window.visualMotion.filter(event => event.target === "dropdown").length);
    await menu.click(); await settle();
    assert.equal(await page.evaluate(() => window.visualMotion.filter(event => event.target === "dropdown").length), count + 1, "Menu must animate on each opening");
    await page.keyboard.press("Escape");
    assert.equal(await page.locator(".dropdown").first().evaluate(element => getComputedStyle(element).animationName), "none", "Closing must reset the menu animation");
  }
  const cardAnimations = await page.evaluate(() => window.visualMotion.filter(event => event.target.startsWith("drive ")).length);
  await page.evaluate(() => { window.visualSnapshot.connections[0].path = "Refreshed library"; window.visualSnapshot.connections[0].serverPath = "https://files.example.test/dav/Refreshed library"; window.visualVisibility(false); window.visualVisibility(true); });
  await page.getByText("https://files.example.test/dav/Refreshed library", { exact: true }).waitFor();
  await settle();
  assert.equal(await page.evaluate(() => window.visualMotion.filter(event => event.target.startsWith("drive ")).length), cardAnimations, "Status refreshes must not animate existing cards");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.getByRole("button", { name: "+ New connection", exact: true }).click();
  assert.equal(await page.getByRole("dialog").evaluate(element => getComputedStyle(element).animationName), "none");
  assert.equal(await page.getByRole("dialog").evaluate(element => getComputedStyle(element, "::backdrop").animationName), "none");
  await page.getByRole("button", { name: "Close setup", exact: true }).click();
  await menu.click();
  assert.equal(await page.locator(".dropdown:visible").evaluate(element => getComputedStyle(element).animationName), "none");
  await page.keyboard.press("Escape");
  await page.emulateMedia({ reducedMotion: "no-preference" });
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
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  assert.equal(await page.getByLabel("Drive card stripe").inputValue(), "theme");
  assert(await firstCard.evaluate(element => getComputedStyle(element).borderLeftColor === getComputedStyle(document.querySelector(".primary")).backgroundColor), "Default stripe must follow the theme");
  await page.getByLabel("Drive card stripe").selectOption("drive");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.waitForFunction(() => document.querySelector(".drives").dataset.stripeSource === "drive");
  assert.equal(await firstCard.evaluate(element => getComputedStyle(element).borderLeftColor), "rgb(50, 183, 165)");
  assert(await page.locator(".drive").last().evaluate(element => getComputedStyle(element).borderLeftColor === getComputedStyle(document.querySelector(".primary")).backgroundColor), "Missing custom color must fall back to the theme");
  await page.getByRole("button", { name: /Preferences/ }).click();
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  assert.equal(await page.getByLabel("Drive card stripe").inputValue(), "drive");
  await page.getByRole("tab", { name: "Configuration", exact: true }).click();
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
    const catalog = language === "en" ? {} : JSON.parse(await readFile(path.join(root, "internal/i18n", language + ".json"), "utf8"));
    const t = text => catalog[text] || text;
    await page.setViewportSize({ width: 420, height: 740 });
    await page.getByRole("button", { name: t("+ New connection"), exact: true }).click();
    await page.getByLabel(t("Mount into")).selectOption("folder");
    await page.getByRole("button", { name: t("Choose parent directory"), exact: true }).click();
    await bounds();
    await page.getByLabel(t("Connection type")).selectOption("gcs:serviceAccount");
    await page.getByRole("button", { name: t("Continue"), exact: true }).click();
    await page.getByRole("button", { name: t("Select file…"), exact: true }).click();
    await page.getByText(t("Advanced options"), { exact: true }).click();
    await bounds(); await capture("advanced-groups-" + language);
    await page.getByRole("button", { name: t("Close setup"), exact: true }).click();
    await page.getByRole("button", { name: new RegExp(t("Preferences")) }).click();
    await page.locator(".dialog-navigation .navigation-toggle").click();
    await page.getByRole("tab", { name: t("Cache"), exact: true }).click();
    await bounds(); await capture("cache-actions-" + language);
    await page.getByRole("button", { name: t("Cancel"), exact: true }).click();
  }
  await page.evaluate(() => { window.visualSnapshot.settings.language = "en"; window.visualVisibility(false); window.visualVisibility(true); });
  await page.waitForFunction(() => document.documentElement.lang === "en");
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.getByRole("button", { name: /Preferences/ }).click();
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  await page.getByLabel("Theme", { exact: true }).selectOption("dark");
  await page.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  await bounds(); await capture("dark-drawer");
  await page.emulateMedia({ forcedColors: "active" }); await bounds(); await capture("forced-colors");
  await page.emulateMedia({ forcedColors: "none" });
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.deepEqual(errors, []);
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "passed", type: "browser-fixture", checks: "Single-line local/remote paths with full hover text, bold mount prefix, short Open action, theme/custom stripes and fallback, discreet credits, hidden file checkbox, drive appearance save/cancel, local image normalization, folder icon, destination hierarchy, U1, 44px actions, long paths, four viewport sizes, advanced groups and native validation, horizontal field actions, drawer/menu animation and reopening, stable cards on refresh, reduced motion including backdrops, seven topics, preference sections, setup, contextual help and Escape, nested import, access settings, diagnostics, seven languages including narrow field actions, dark and forced colors", errors }, null, 2));
  console.log("Visual UI smoke passed; screenshots: " + artifacts);
} catch (error) {
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "failed", type: "browser-fixture", reason: error.message }, null, 2));
  if (page && !page.isClosed()) await page.screenshot({ path: path.join(artifacts, "failure.png"), mask: [page.locator("[data-sensitive]")] }).catch(() => {});
  throw error;
} finally { await browser?.close(); await server.close(); }
