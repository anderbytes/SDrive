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
  connections: [{ ...connection, capacity: { total: 100 * 1024 ** 3, used: 25 * 1024 ** 3, free: 75 * 1024 ** 3, checkedAt: null, state: "available" } }, { ...connection, id: "visual-second", name: "Team library", letter: "", mountFolder: "C:\\Synthetic\\Team\\" + "Long local directory\\".repeat(12), cardStyle: "violet", serverPath: "https://files.example.test/dav/Projects", path: "Projects", mounted: false },
    { ...connection, id: "visual-long", name: "Shared documents and administrative projects with a deliberately long name", letter: "U", serverPath: "https://files.example.test/dav/" + "Long remote directory/".repeat(8), path: "Long remote directory/".repeat(8), pending: null, recovery: true }],
  importDrafts: [], letters: ["V", "W"], winFsp: true, engineError: "", settingsError: "", desktopWarning: "", startupWarning: "", maintenanceError: "", recoveryHold: false };
const apiSource = `window.visualSnapshot = ${JSON.stringify(snapshot)};
const current = () => window.visualSnapshot;
const wait = async name => {
  if (window.visualWaits?.[name]) await new Promise(resolve => {
    (window.visualRelease ||= {})[name] = () => { delete window.visualWaits[name]; resolve(); };
  });
};
export const api = {
  accessStatus: async () => { await wait('accessStatus'); return current().access; }, snapshot: async () => { await wait('snapshot'); return structuredClone(current()); }, desktopVisible: async () => true,
  onDesktopVisibility: cb => { window.visualVisibility = cb; return () => {}; }, onAccessChanged: cb => {window.visualAccess = cb; return () => {};},
  preferenceDefaults: async () => ({ ...current().settings, cacheGiB: 2, cacheMaxAgeHours: 24, writeBackSeconds: 5 }), restoreDefaultCacheLocation: async () => current().cacheLocation,
  proxy: async () => ({ address: '', username: '', password: '', hasPassword: false }), browsers: async () => [{ id: '', name: 'Default browser' }],
  updateDriveAppearance: async (id, appearance) => { Object.assign(current().connections.find(c => c.id === id), appearance); },
  test: async () => {}, refreshDirectoryCache: async () => {}, update: async connection => { Object.assign(current().connections.find(c => c.id === connection.id), connection); },
  fields: async () => ({ values: {}, transport: '' }), settings: async settings => { await wait('settings'); current().settings = settings; },
  begin: async () => 'synthetic-draft', status: async () => ({busy:false,authenticated:true,complete:false,error:''}), folders: async () => [{name:'Documents',path:'Documents'}], end: async () => {},
  selectDirectory: async () => 'C:\\\\Synthetic\\\\NewCache', selectFile: async () => ${JSON.stringify("C:\\Synthetic\\Credentials\\" + "long-directory\\".repeat(8) + "account.json")},
  about: async () => ({appVersion:'fixture',rcloneVersion:'fixture',windowsVersion:'fixture',winFspVersion:'fixture',webView2Version:'fixture',architecture:'amd64'}),
  diagnostics: async () => { await wait('diagnostics'); return 'Synthetic diagnostic report. No credentials or personal paths.'; }, copyDiagnostics: async () => {},
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
    window.visualWaits = { accessStatus: true, snapshot: true };
    window.visualMotion = [];
    document.addEventListener("animationstart", event => window.visualMotion.push({ name: event.animationName, target: event.target.className }));
  });
  page.setDefaultTimeout(8000);
  page.on("pageerror", e => errors.push(e.message));
  await page.goto(server.resolvedUrls.local[0]);
  const settle = () => page.evaluate(() => Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {}))));
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
  const largeSpinner = page.locator(".spinner-large");
  await largeSpinner.waitFor();
  assert.equal(await largeSpinner.evaluate(element => getComputedStyle(element).width), "40px");
  assert.equal(await largeSpinner.evaluate(element => getComputedStyle(element).animationName), "spinner-rotate");
  assert(await largeSpinner.evaluate(element => {
    const ring = element.getBoundingClientRect(), main = element.closest("main").getBoundingClientRect();
    return Math.abs((ring.left + ring.right - main.left - main.right) / 2) <= 1;
  }), "Startup spinner must be centered");
  await capture("startup-loading");
  await page.emulateMedia({ reducedMotion: "reduce" });
  assert.equal(await largeSpinner.evaluate(element => getComputedStyle(element).animationName), "none");
  await page.emulateMedia({ reducedMotion: "no-preference", forcedColors: "active" });
  assert(await largeSpinner.evaluate(element => getComputedStyle(element).borderTopColor !== getComputedStyle(element).borderRightColor), "Spinner arc must remain visible in forced colors");
  await capture("startup-forced-colors");
  await page.emulateMedia({ forcedColors: "none" });
  await page.evaluate(() => window.visualRelease.accessStatus());
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  await largeSpinner.waitFor();
  await bounds(); await capture("snapshot-loading");
  await page.evaluate(() => window.visualRelease.snapshot());
  await page.getByRole("heading", { name: "Documents", exact: true }).waitFor();
  assert.equal(await page.locator(".spinner").count(), 0, "Startup spinner must disappear when ready");
  await page.evaluate(() => {
    window.visualSnapshot.connections[0].busy = true;
    window.visualSnapshot.connections[0].operation = "mounting";
    window.visualVisibility(false); window.visualVisibility(true);
  });
  await page.locator(".drive .spinner").waitFor();
  assert.equal(await page.locator(".drive .spinner").evaluate(element => getComputedStyle(element).width), "18px");
  await page.setViewportSize({ width: 420, height: 740 });
  await bounds(); await capture("drive-operation-narrow");
  await page.evaluate(() => {
    window.visualSnapshot.connections[0].busy = false;
    window.visualSnapshot.connections[0].operation = "";
    window.visualVisibility(false); window.visualVisibility(true);
  });
  await page.locator(".drive .spinner").waitFor({ state: "detached" });
  await page.setViewportSize({ width: 1440, height: 900 });
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
    assert.equal(await firstCard.getByRole("progressbar").getAttribute("value"), "25");
    assert(await firstCard.getByText("25% used · 25.0 GiB of 100.0 GiB", { exact: true }).isVisible());
    assert(await firstCard.locator(".drive-capacity").evaluate(element => {
      const box = element.getBoundingClientRect(), card = element.closest(".drive").getBoundingClientRect();
      return box.width > 0 && box.left >= card.left && box.right <= card.right && box.bottom <= card.bottom;
    }), "Remote usage must fit inside the drive card");
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
    for (const topic of ["Remote destination", "Connection options", "Local cache", "Remote capacity", "Connection security"]) {
      const trigger = firstCard.getByRole("button", { name: topic, exact: true });
      await trigger.scrollIntoViewIfNeeded();
      await trigger.hover();
      await page.getByRole("tooltip").waitFor();
      assert.equal(await page.getByRole("tooltip").locator("strong").first().textContent(), topic);
      assert.equal(await page.getByRole("dialog").count(), 0);
      const box = await page.getByRole("tooltip").boundingBox();
      assert(box.x >= 0 && box.y >= 0 && box.x + box.width <= viewport.width + 1 && box.y + box.height <= viewport.height + 1);
      await page.keyboard.press("Escape");
    }
    await firstCard.getByRole("button", { name: "Local cache", exact: true }).click();
    await capture("details-" + viewport.width);
    await page.keyboard.press("Escape");
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  const originalDrive = await page.evaluate(() => structuredClone(window.visualSnapshot.connections[0]));
  const changeDrive = async patch => {
    await page.evaluate(patch => {
      Object.assign(window.visualSnapshot.connections[0], patch);
      window.visualVisibility(false); window.visualVisibility(true);
    }, patch);
  };
  for (const theme of ["light", "dark"]) {
    await page.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
    for (const [state, tone] of [["encrypted", "success"], ["unencrypted", "inactive"], ["unknown", "warning"]]) {
      await changeDrive({ transportSecurity: { ...originalDrive.transportSecurity, state } });
      const shield = firstCard.getByRole("button", { name: "Connection security", exact: true });
      await page.waitForFunction(tone => document.querySelector('[data-topic="security"]')?.dataset.tone === tone, tone);
      assert.equal(await shield.isEnabled(), true, "Inactive security remains inspectable");
      assert(await shield.evaluate(element => {
        const box = element.getBoundingClientRect(), glyph = element.querySelector("svg").getBoundingClientRect();
        return box.width === 32 && box.height === 32 && glyph.width === 16 && glyph.height === 16 && getComputedStyle(element.closest(".drive-details")).gap === "2px";
      }), "Desktop icon and glyph dimensions");
      const info = await firstCard.getByRole("button", { name: "Remote destination", exact: true }).evaluate(element => getComputedStyle(element).color);
      const shieldColor = await shield.evaluate(element => getComputedStyle(element).color);
      assert.notEqual(shieldColor, info, "Security state must have its own color");
      if (state === "unencrypted") {
        assert.equal(await shield.locator(".security-off-mark").count(), 1);
        assert.notEqual(await shield.locator(".security-off-mark").evaluate(element => getComputedStyle(element).stroke), shieldColor);
      }
      await shield.click(); await page.getByRole("tooltip").waitFor();
      await page.keyboard.press("Escape");
      await bounds(); await capture("security-" + state + "-" + theme);
    }
  }
  await changeDrive({ pending: 2, uploads: { active: 1, queued: 1, cacheErrors: 0 } });
  const uploadIcon = firstCard.getByRole("button", { name: "Upload details", exact: true });
  await uploadIcon.focus(); await page.getByRole("tooltip").waitFor();
  await changeDrive({ pending: 0, uploads: { active: 0, queued: 0, cacheErrors: 0 } });
  await uploadIcon.waitFor({ state: "detached" });
  assert.equal(await firstCard.getByRole("button", { name: "Remote destination", exact: true }).evaluate(element => element === document.activeElement), true);
  await page.keyboard.press("Escape");
  await changeDrive({ mounted: false, recovery: false, cache: { ...originalDrive.cache, usedBytes: 0, pendingBytes: 0 } });
  await firstCard.getByRole("button", { name: "Edit drive", exact: true }).waitFor();
  assert.equal(await firstCard.locator(".drive-details button").count(), 2, "Disconnected clean drives only need destination and options");
  await page.setViewportSize({ width: 420, height: 740 });
  await bounds(); await capture("contextual-disconnected-narrow");
  await changeDrive({ ...originalDrive, pending: null, recovery: true, mounted: false });
  await firstCard.getByRole("button", { name: "Alerts and recovery", exact: true }).waitFor();
  assert.equal(await firstCard.getByRole("button", { name: "Alerts and recovery", exact: true }).getAttribute("data-tone"), "danger");
  await bounds(); await capture("contextual-recovery-narrow");
  await changeDrive({ ...originalDrive, capacity: { ...originalDrive.capacity, state: "unsupported" } });
  await firstCard.getByRole("button", { name: "Connection security", exact: true }).waitFor();
  assert.equal(await firstCard.getByRole("button", { name: "Remote capacity", exact: true }).count(), 0);
  await changeDrive(originalDrive);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.evaluate(() => { document.documentElement.dataset.theme = "light"; });
  const touchPage = await browser.newPage({ hasTouch: true, viewport: { width: 420, height: 740 } });
  try {
    await touchPage.goto(server.resolvedUrls.local[0]);
    await touchPage.getByRole("button", { name: "Drive diagnostics", exact: true }).first().waitFor();
    assert(await touchPage.evaluate(() => matchMedia("(pointer: coarse)").matches));
    await touchPage.evaluate(() => Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {}))));
    assert.deepEqual(await touchPage.locator(".drive-detail-icon").first().evaluate(element => {
      const box = element.getBoundingClientRect(); return { width: box.width, height: box.height };
    }), { width: 44, height: 44 }, "Touch retains 44px icon controls");
  } finally { await touchPage.close(); }
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
  const menu = page.locator(".drive-controls > .actions > details > summary").first();
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
  await firstCard.getByRole("button", { name: "Drive diagnostics", exact: true }).click();
  await page.getByRole("tab", { name: "Applications using a file", exact: true }).click();
  await page.getByRole("tab", { name: "Test access", exact: true }).click();
  await page.getByRole("button", { name: "Test access", exact: true }).click();
  await page.getByText("Access test succeeded. Listing folders does not verify write permission.", { exact: true }).waitFor();
  await page.getByRole("tab", { name: "Refresh files in Explorer", exact: true }).click();
  await page.getByRole("button", { name: "Refresh files in Explorer", exact: true }).click();
  await page.getByText("Directory metadata refreshed. Reopen the folder in Explorer to see remote changes.", { exact: true }).waitFor();
  await capture("drive-diagnostics");
  await page.getByRole("tab", { name: "Applications using a file", exact: true }).click();
  await page.getByRole("button", { name: "Select mounted file", exact: true }).click();
  await page.getByText("File access information is unknown.", { exact: false }).waitFor();
  await bounds(); await capture("file-access");
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await page.locator(".drive").nth(1).getByRole("button", { name: "Edit drive", exact: true }).click();
  for (const section of ["General", "Access", "Files", "Network"]) {
    await page.getByRole("tab", { name: section, exact: true }).click();
    await bounds(); await capture("edit-drive-" + section.toLowerCase());
  }
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
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
  await page.evaluate(() => { window.visualWaits.diagnostics = true; });
  await page.getByRole("button", { name: "View diagnostics", exact: true }).click();
  const diagnosticsButton = page.getByRole("button", { name: /Loading diagnostic report/ });
  await diagnosticsButton.locator(".spinner").waitFor();
  assert.equal(await diagnosticsButton.getAttribute("aria-live"), "polite");
  await bounds(); await capture("diagnostics-loading");
  await page.evaluate(() => window.visualRelease.diagnostics());
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
  await page.evaluate(() => { window.visualWaits.settings = true; });
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.locator(".loading-status .spinner").waitFor();
  assert.equal(await page.locator(".loading-status .spinner").evaluate(element => getComputedStyle(element).width), "18px");
  await capture("saving-dark");
  await page.emulateMedia({ reducedMotion: "reduce" });
  assert.equal(await page.locator(".loading-status .spinner").evaluate(element => getComputedStyle(element).animationName), "none");
  await page.evaluate(() => window.visualRelease.settings());
  await page.getByRole("dialog").waitFor({ state: "detached" });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.getByRole("button", { name: /Preferences/ }).click();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.deepEqual(errors, []);
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "passed", type: "browser-fixture", checks: "Centered startup and snapshot spinners, compact card and button spinners, pending saves, reduced-motion and forced-colors spinners, Single-line local/remote paths with full hover text, bold mount prefix, short Open action, theme/custom stripes and fallback, discreet credits, hidden file checkbox, drive appearance save/cancel, local image normalization, folder icon, destination hierarchy, U1, 44px actions, long paths, four viewport sizes, advanced groups and native validation, horizontal field actions, drawer/menu animation and reopening, stable cards on refresh, reduced motion including backdrops, contextual icon visibility, semantic security colors, disappearing-icon focus recovery, 32px desktop and 44px touch controls, preference sections, setup, contextual help and Escape, nested import, access settings, diagnostics, seven languages including narrow field actions, dark and forced colors", errors }, null, 2));
  console.log("Visual UI smoke passed; screenshots: " + artifacts);
} catch (error) {
  await writeFile(path.join(artifacts, "result.json"), JSON.stringify({ status: "failed", type: "browser-fixture", reason: error.message }, null, 2));
  if (page && !page.isClosed()) await page.screenshot({ path: path.join(artifacts, "failure.png"), mask: [page.locator("[data-sensitive]")] }).catch(() => {});
  throw error;
} finally { await browser?.close(); await server.close(); }
