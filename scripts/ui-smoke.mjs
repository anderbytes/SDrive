import { runSmoke } from "./ui-smoke-runtime.mjs";
import path from "node:path";
import assert from "node:assert/strict";

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
    18,
  );
  await page.getByLabel("Connection type").selectOption("webdav");
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
  await page
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: /Preferences/ })
    .click();
  await page
    .getByRole("heading", { name: "Preferences", exact: true })
    .waitFor();
  await page.getByLabel("Cache target per connection (GiB)").fill("3");
  await page
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: /Preferences/ })
    .click();
  await page.getByLabel("Language").selectOption("pt-BR");
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
    "Native Wails/WebView2 smoke: 18 profiles, real WebDAV wizard and folder browsing, cancellation, settings, English/Portuguese switching, safe exit — OK",
  );
});
