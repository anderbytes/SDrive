import { runSmoke, smokeEnvironment } from "./ui-smoke-runtime.mjs";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
const exec = promisify(execFile);
import assert from "node:assert/strict";
import { mkdir, readFile, writeFile } from "node:fs/promises";

async function verifyAboutActions(page, language) {
  const catalog = language === "en" ? {} : JSON.parse(await readFile(path.join(process.cwd(), "internal/i18n", `${language}.json`), "utf8"));
  const t = (source) => catalog[source] || source;
  await page.getByRole("button", { name: t("About SDrive"), exact: true }).click();
  const dialog = page.getByRole("dialog", { name: t("About SDrive"), exact: true });
  await dialog.getByText("SDrive", { exact: true }).waitFor();
  const actions = await dialog.getByRole("button").evaluateAll((buttons) => buttons.map((button) => button.getAttribute("aria-label") || button.textContent));
  assert(actions.every((name) => [t("Close"), t("Close dialog"), t("View diagnostics")].includes(name)));
  await dialog.getByRole("button", { name: t("View diagnostics"), exact: true }).click();
  assert((await dialog.getByRole("textbox").inputValue()).length > 0);
  await dialog.getByRole("button", { name: t("Copy report"), exact: true }).waitFor();
  await dialog.getByRole("button", { name: t("Close"), exact: true }).click();
}

await runSmoke(async ({ page, dav, child, exited, artifacts, profile }) => {
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  await page.getByText("One letter. All your files.").waitFor();
  await page
    .getByRole("button", { name: "+ New connection", exact: true })
    .click();
  assert.equal(
    await page.getByLabel("Connection type").locator("option").count(),
    12,
  );
  assert.equal(await page.getByRole("tab", { name: "Basic", exact: true }).getAttribute("aria-selected"), "true");
  await page.getByRole("tab", { name: "Advanced", exact: true }).click();
  assert.equal(await page.getByLabel("Connection type").locator("option").count(), 7);
  await page.getByText("These connections require additional setup.", { exact: false }).waitFor();
  await page.getByRole("tab", { name: "Advanced", exact: true }).press("Home");
  assert.equal(await page.getByRole("tab", { name: "Basic", exact: true }).getAttribute("aria-selected"), "true");
  await page.getByLabel("Connection type").selectOption("webdav");
  await page.getByLabel("Mount into").selectOption("folder");
  await page.getByLabel("Local mount folder").fill(path.join(artifacts, "new-mount"));
  assert.equal(await page.getByLabel("Drive presentation").inputValue(), "local");
  assert.equal(await page.getByLabel("Drive presentation").isDisabled(), true);
  await page
    .getByRole("button", { name: "Continue", exact: true })
    .click();
  await page
    .getByLabel("WebDAV HTTPS URL")
    .fill(`http://127.0.0.1:${dav.address().port}/dav/`);
  await page
    .getByText("Advanced options", { exact: true })
    .click();
  await page
    .getByLabel("Allow unencrypted connection (HTTP / FTP)")
    .check();
  await page
    .getByRole("button", { name: "Continue", exact: true })
    .click();
  await page
    .getByRole("button", { name: "▱ Documents", exact: true })
    .waitFor();
  await page
    .getByRole("button", { name: "▱ Documents", exact: true })
    .click();
  await page.getByText("No subfolders here.", { exact: true }).waitFor();
  assert(
    await page
      .getByText("Current folder:")
      .locator("..")
      .textContent()
      .then((value) => value.includes("Documents")),
  );
  await page.screenshot({ path: path.join(artifacts, "wizard.png"), timeout: 10000 });
  if (process.env.SDRIVE_TEST_MOUNT === "1") {
    await page.getByRole("button", { name: "Connect drive", exact: true }).click();
    await page.getByRole("heading", { name: "Drive connected", exact: true }).waitFor();
    await page.getByRole("button", { name: "Done", exact: true }).click();
    const drive = page.locator(".drive");
    await drive.getByText("Connected", { exact: true }).waitFor();
    await drive.locator("summary").click();
    await drive.getByRole("button", { name: "Refresh files in Explorer", exact: true }).click();
    await page.getByText("Directory metadata refreshed. Reopen the folder in Explorer to see remote changes.", { exact: true }).waitFor();
    await drive.getByRole("button", { name: "Disconnect", exact: true }).click();
    await drive.getByText("Disconnected", { exact: true }).waitFor();
  } else {
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
  }
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: /Preferences/ })
    .click();
  await page
    .getByRole("heading", { name: "Preferences", exact: true })
    .waitFor();
  assert.equal(await page.getByRole("tab", { name: "General", exact: true }).getAttribute("aria-selected"), "true");
  await page.getByRole("tab", { name: "General", exact: true }).press("ArrowRight");
  assert.equal(await page.getByRole("tab", { name: "Appearance", exact: true }).getAttribute("aria-selected"), "true");
  await page.emulateMedia({ colorScheme: "light" });
  await page.waitForFunction(() => document.documentElement.dataset.theme === "light");
  const lightBackground = await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor);
  await page.getByLabel("Theme").selectOption("dark");
  await page.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  assert.notEqual(await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor), lightBackground);
  await page.screenshot({ path: path.join(artifacts, "preferences-dark.png") });
  await page.getByRole("dialog", { name: "Preferences", exact: true }).press("Escape");
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page.waitForFunction(() => document.documentElement.dataset.theme === "light");
  await page.getByRole("button", { name: /Preferences/ }).click();
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  assert.equal(await page.getByLabel("Theme").inputValue(), "system");
  await page.emulateMedia({ colorScheme: "dark" });
  await page.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  await page.getByLabel("Theme").selectOption("light");
  await page.waitForFunction(() => document.documentElement.dataset.theme === "light");
  await page.getByRole("tab", { name: "Appearance", exact: true }).press("ArrowLeft");
  await page.getByRole("tab", { name: "General", exact: true }).press("ArrowRight");
  await page.emulateMedia({ forcedColors: "active" });
  assert.equal(await page.getByRole("tab", { name: "Appearance", exact: true }).evaluate((element) => getComputedStyle(element).outlineStyle), "solid");
  await page.screenshot({ path: path.join(artifacts, "preferences-high-contrast.png") });
  await page.emulateMedia({ forcedColors: "none" });
  await page.screenshot({ path: path.join(artifacts, "preferences-light.png") });
  await page.getByRole("tab", { name: "Cache", exact: true }).click();
  await page.getByLabel("Cache target per connection (GiB)").fill("3");
  await page.getByLabel("Cache retention after last access (hours)").fill("48");
  await page.getByLabel("Upload delay after last use (seconds)").fill("0");
  const selectedCache = path.join(artifacts, "selected-cache");
  await mkdir(selectedCache);
  await page.getByLabel("New empty cache directory").fill(selectedCache);
  await page.getByRole("button", { name: "Change cache location", exact: true }).click();
  await page.getByText("Cache location changed. The previous cache was preserved.", { exact: true }).waitFor();
  await page
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: /Preferences/ })
    .click();
  await page.getByLabel("Language").selectOption("pt-BR");
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  assert.equal(await page.getByLabel("Theme").inputValue(), "light");
  await page.getByRole("tab", { name: "Cache", exact: true }).click();
  assert.equal(await page.getByLabel("Cache retention after last access (hours)").inputValue(), "48");
  assert.equal(await page.getByLabel("Upload delay after last use (seconds)").inputValue(), "0");
  await page
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await page
    .getByRole("heading", { name: "Minhas unidades", exact: true })
    .waitFor();
  assert.equal(await page.locator("html").getAttribute("lang"), "pt-BR");
  await page
    .getByRole("button", { name: /Preferências/ })
    .click();
  assert.equal(await page.getByLabel("Idioma").inputValue(), "pt-BR");
  assert.equal(await page.getByLabel("Abrir o Explorer após conectar", { exact: true }).isChecked(), false);
  await page.getByRole("tab", { name: "Aparência", exact: true }).click();
  assert.equal(await page.getByLabel("Tema").inputValue(), "light");
  assert.equal(await page.getByLabel("Mostrar ícones de serviço nas letras de unidade", { exact: true }).isChecked(), false);
  await page.getByRole("tab", { name: "Configuração", exact: true }).click();
  assert.equal(await page.getByRole("button", { name: "Exportar configuração", exact: true }).isVisible(), true);
  await page.getByRole("tab", { name: "Cache", exact: true }).click();
  assert.equal(await page.getByLabel("Retenção do cache após o último acesso (horas)").inputValue(), "48");
  assert.equal(await page.getByLabel("Atraso do upload após o último uso (segundos)").inputValue(), "0");
  await page.getByRole("tab", { name: "Geral", exact: true }).click();
  await page.getByLabel("Idioma").selectOption("en");
  await page
    .getByRole("button", { name: "Salvar", exact: true })
    .click();
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  assert.equal(await page.locator("html").getAttribute("lang"), "en");
  await page.getByRole("button", { name: /Preferences/ }).click();
  assert.equal(await page.getByLabel("Open Explorer after connecting", { exact: true }).isChecked(), false);
  await page.getByRole("tab", { name: "Appearance", exact: true }).click();
  assert.equal(await page.getByLabel("Show service icons for drive letters", { exact: true }).isChecked(), false);
  await page.getByRole("tab", { name: "Configuration", exact: true }).click();
  await page.getByRole("button", { name: "Export configuration", exact: true }).click();
  await page.getByRole("button", { name: "Import configuration", exact: true }).click();
  await page.getByRole("heading", { name: "Review imported configuration", exact: true }).waitFor();
  const portablePath = path.join(profile, "portable.json");
  const portable = JSON.parse(await readFile(portablePath, "utf8"));
  assert.equal(portable.format, "sdrive-portable");
  assert.equal(portable.version, 1);
  assert.equal(portable.preferences.theme, "light");
  assert.equal("autoStart" in portable.preferences, false);
  await page.getByRole("dialog", { name: "Review imported configuration" }).getByRole("button", { name: "Cancel", exact: true }).click();
  portable.connections = [{ name: "Portable smoke", provider: "webdav", path: "", letter: "", mountMode: "local", mountFolder: path.join(artifacts, "imported-mount"), readOnly: true, autoMount: false, values: { url: `http://127.0.0.1:${dav.address().port}/dav/` } }];
  await writeFile(portablePath, JSON.stringify(portable));
  await page.getByRole("button", { name: "Import configuration", exact: true }).click();
  await page.getByRole("heading", { name: "Review imported configuration", exact: true }).waitFor();
  assert.equal(await page.getByLabel("Apply imported theme, language, cache timing and desktop preferences").isChecked(), false);
  await page.getByRole("button", { name: "Save imported drafts", exact: true }).click();
  await page.getByText("Authentication required for imported configuration", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Authenticate", exact: true }).click();
  assert.equal(await page.getByLabel("Connection type").inputValue(), "webdav");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  assert.equal(await page.getByLabel("WebDAV HTTPS URL").inputValue(), portable.connections[0].values.url);
  await page.getByText("Advanced options", { exact: true }).click();
  assert.equal(await page.getByLabel("Allow unencrypted connection (HTTP / FTP)").isChecked(), false);
  await page.getByLabel("Allow unencrypted connection (HTTP / FTP)").check();
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByRole("button", { name: "▱ Documents", exact: true }).waitFor();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByText("Authentication required for imported configuration", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Remove draft", exact: true }).click();
  await page.getByRole("button", { name: "Authenticate", exact: true }).waitFor({ state: "hidden" });
  await verifyAboutActions(page, "en");
  await page.getByRole("button", { name: "Application lock", exact: true }).click();
  await page.getByLabel("New PIN", { exact: true }).fill("123456");
  await page.getByLabel("Confirm new PIN", { exact: true }).fill("123456");
  assert.equal(await page.getByLabel("Lock after computer inactivity (minutes; 0 disables)").inputValue(), "15");
  assert.equal(await page.getByLabel("Disconnect safely after inactivity", { exact: true }).isChecked(), false);
  assert.equal(await page.getByLabel("Reconnect idle-disconnected drives after unlock", { exact: true }).isChecked(), false);
  await page.getByLabel("Lock after computer inactivity (minutes; 0 disables)").fill("0");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  const recoveryCode = await page.getByLabel("Recovery code", { exact: true }).textContent();
  assert(recoveryCode && recoveryCode.length === 32);
  await page.getByRole("button", { name: "I saved the recovery code", exact: true }).click();
  await page.getByRole("button", { name: "Lock SDrive", exact: true }).click();
  await page.getByRole("heading", { name: "SDrive is locked", exact: true }).waitFor();
  await page.waitForFunction(() => document.documentElement.dataset.theme === "light");
  assert.equal(await page.getByRole("button", { name: "+ New connection", exact: true }).count(), 0);
  await exec(child.spawnfile, [], { windowsHide: true, timeout: 15000, env: smokeEnvironment(profile) });
  await page.getByRole("heading", { name: "SDrive is locked", exact: true }).waitFor();
  await page.screenshot({ path: path.join(artifacts, "application-locked-en.png") });
  await page.getByLabel("Application PIN", { exact: true }).fill("000000");
  await page.getByRole("button", { name: "Unlock", exact: true }).click();
  await page.getByText("The PIN or recovery code is incorrect.", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Forgot PIN?", exact: true }).click();
  await page.getByLabel("Recovery code", { exact: true }).fill(recoveryCode);
  await page.getByLabel("New PIN", { exact: true }).fill("654321");
  await page.getByRole("button", { name: "Reset PIN", exact: true }).click();
  await page.getByRole("button", { name: "I saved the recovery code", exact: true }).click();
  await page.getByLabel("Application PIN", { exact: true }).fill("654321");
  await page.getByRole("button", { name: "Unlock", exact: true }).click();
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  await page.getByRole("button", { name: "⚙   Preferences", exact: true }).click();
  await page.getByLabel("Language").selectOption("pt-BR");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.getByRole("button", { name: "Bloquear SDrive", exact: true }).click();
  await page.getByRole("heading", { name: "O SDrive está bloqueado", exact: true }).waitFor();
  await page.screenshot({ path: path.join(artifacts, "application-locked-pt-BR.png") });
  await page.getByLabel("PIN do aplicativo", { exact: true }).fill("654321");
  await page.getByRole("button", { name: "Desbloquear", exact: true }).click();
  await page.getByRole("button", { name: "Bloqueio do aplicativo", exact: true }).click();
  await page.getByLabel("PIN atual", { exact: true }).fill("654321");
  await page.getByRole("button", { name: "Desativar bloqueio do aplicativo", exact: true }).click();
  await verifyAboutActions(page, "pt-BR");
  await page.getByRole("button", { name: "⚙   Preferências", exact: true }).click();
  await page.getByRole("tab", { name: "Geral", exact: true }).click();
  await page.getByLabel("Idioma").selectOption("en");
  await page.getByRole("button", { name: "Salvar", exact: true }).click();
  assert.deepEqual(errors, []);
  await page
    .getByRole("button", { name: "Exit safely", exact: true })
    .click();
  assert.equal(await exited, 0);
  console.log(
    "Native Wails/WebView2 smoke: setup, WebDAV browsing, portable export/import and draft reauthentication/cancellation, preference tabs, live theme previews and cancellation, system/light/dark themes and forced colors, desktop preferences, PIN lock/recovery, second-instance activation while locked, English/Portuguese switching, safe exit — OK",
  );
});
