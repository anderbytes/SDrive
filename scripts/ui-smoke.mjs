import { runSmoke } from "./ui-smoke-runtime.mjs";
import path from "node:path";
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";

await runSmoke(async ({ page, dav, exited, artifacts }) => {
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
  assert.equal(await page.getByLabel("Retenção do cache após o último acesso (horas)").inputValue(), "48");
  assert.equal(await page.getByLabel("Atraso do upload após o último uso (segundos)").inputValue(), "0");
  await page.getByLabel("Idioma").selectOption("en");
  await page
    .getByRole("button", { name: "Salvar", exact: true })
    .click();
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  assert.equal(await page.locator("html").getAttribute("lang"), "en");
  assert.deepEqual(errors, []);
  await page
    .getByRole("button", { name: "Exit safely", exact: true })
    .click();
  assert.equal(await exited, 0);
  console.log(
    "Native Wails/WebView2 smoke: setup tabs, local folder destination, real WebDAV browsing, cancellation, cache location and timing, English/Portuguese switching, safe exit — OK",
  );
});
