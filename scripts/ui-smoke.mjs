import { chromium } from "../frontend/node_modules/playwright/index.mjs";
import { spawn, execFileSync } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import assert from "node:assert/strict";
import { createServer } from "node:http";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const profile = path.join(root, ".tools", "ui-check");
await mkdir(profile, { recursive: true });
const env = Object.fromEntries(
  Object.entries(process.env).filter(
    ([key]) => key.toUpperCase() !== "LOCALAPPDATA",
  ),
);
const binary = path.join(root, "bin", "SDrive-ui-check.exe");
// A local read-only WebDAV fixture validates the real wizard/rclone handoff without an account.
const dav = createServer((request, response) => {
  const path = new URL(request.url, "http://127.0.0.1").pathname;
  if (request.method === "OPTIONS") {
    response.writeHead(200, { DAV: "1, 2", Allow: "OPTIONS, PROPFIND, HEAD" });
    response.end();
    return;
  }
  if (request.method !== "PROPFIND") {
    response.writeHead(405);
    response.end();
    return;
  }
  const entries = [path.endsWith("/") ? path : path + "/"];
  if ((path === "/dav" || path === "/dav/") && request.headers.depth !== "0")
    entries.push("/dav/Documents/");
  response.writeHead(207, { "Content-Type": "application/xml; charset=utf-8" });
  response.end(
    `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">${entries.map((href) => `<d:response><d:href>${href}</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><d:getlastmodified>Thu, 22 Oct 2026 12:00:00 GMT</d:getlastmodified></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`).join("")}</d:multistatus>`,
  );
});
execFileSync(
  path.join(root, ".tools/go/bin/go.exe"),
  [
    "build",
    "-tags",
    "production",
    "-ldflags",
    "-X main.uiSmoke=true",
    "-o",
    binary,
    ".",
  ],
  { cwd: root, windowsHide: true },
);
const child = spawn(binary, ["--background"], {
  cwd: root,
  windowsHide: true,
  env: { ...env, LOCALAPPDATA: profile },
});
await new Promise((resolve) => dav.listen(0, "127.0.0.1", resolve));
let browser;
let diagnostics = "";
child.stderr.on("data", (data) => {
  diagnostics += data.toString().slice(0, 2000);
});
child.stdout.on("data", (data) => {
  diagnostics += data.toString().slice(0, 2000);
});
try {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) break;
    try {
      browser = await chromium.connectOverCDP("http://127.0.0.1:9337");
      break;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
  }
  assert(
    browser,
    `WebView2 did not expose the local debugger (exit=${child.exitCode}): ${diagnostics.slice(0, 2000)}`,
  );
  const context = browser.contexts()[0];
  let page = context.pages()[0];
  if (!page) page = await context.waitForEvent("page");
  await page.setViewportSize({ width: 1180, height: 800 });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  await page.getByText("One letter. All your files.").waitFor();
  await page
    .getByRole("button", { name: "+ New connection", exact: true })
    .click({ force: true });
  assert.equal(
    await page.getByLabel("Connection type").locator("option").count(),
    18,
  );
  await page.getByLabel("Connection type").selectOption("webdav");
  await page
    .getByRole("button", { name: "Continue", exact: true })
    .click({ force: true });
  await page
    .getByLabel("WebDAV HTTPS URL")
    .fill(`http://127.0.0.1:${dav.address().port}/dav/`);
  await page
    .getByText("Advanced options", { exact: true })
    .click({ force: true });
  await page
    .getByLabel("Allow unencrypted connection (HTTP / FTP)")
    .check({ force: true });
  await page
    .getByRole("button", { name: "Continue", exact: true })
    .click({ force: true });
  await page
    .getByRole("button", { name: "▱ Documents", exact: true })
    .waitFor();
  await page
    .getByRole("button", { name: "▱ Documents", exact: true })
    .click({ force: true });
  await page.getByText("No subfolders here.", { exact: true }).waitFor();
  assert(
    await page
      .getByText("Current folder:")
      .locator("..")
      .textContent()
      .then((value) => value.includes("Documents")),
  );
  const markup = await page.content();
  const palette = await page
    .locator("button.primary")
    .first()
    .evaluate((element) => ({
      background: getComputedStyle(element).backgroundColor,
      color: getComputedStyle(element).color,
      forcedColors: matchMedia("(forced-colors: active)").matches,
    }));
  console.log("Native UI palette:", JSON.stringify(palette));
  const css = await page.evaluate(() =>
    [...document.styleSheets]
      .map((sheet) =>
        [...sheet.cssRules].map((rule) => rule.cssText).join("\n"),
      )
      .join("\n"),
  );
  await page
    .getByRole("button", { name: "Cancel", exact: true })
    .click({ force: true });
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: /Preferences/ })
    .click({ force: true });
  await page
    .getByRole("heading", { name: "Preferences", exact: true })
    .waitFor();
  await page.getByLabel("Cache target per connection (GiB)").fill("3");
  await page
    .getByRole("button", { name: "Save", exact: true })
    .click({ force: true });
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page
    .getByRole("button", { name: /Preferences/ })
    .click({ force: true });
  await page.getByLabel("Language").selectOption("pt-BR");
  await page
    .getByRole("button", { name: "Save", exact: true })
    .click({ force: true });
  await page
    .getByRole("heading", { name: "Minhas unidades", exact: true })
    .waitFor();
  assert.equal(await page.locator("html").getAttribute("lang"), "pt-BR");
  await page
    .getByRole("button", { name: /Preferências/ })
    .click({ force: true });
  assert.equal(await page.getByLabel("Idioma").inputValue(), "pt-BR");
  await page.getByLabel("Idioma").selectOption("en");
  await page
    .getByRole("button", { name: "Salvar", exact: true })
    .click({ force: true });
  await page.getByRole("heading", { name: "My drives", exact: true }).waitFor();
  assert.equal(await page.locator("html").getAttribute("lang"), "en");
  assert.deepEqual(errors, []);
  const exited = new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error("Application did not exit")),
      10000,
    );
    child.once("exit", (code) => {
      clearTimeout(timer);
      code === 0 ? resolve() : reject(new Error(`Exit code: ${code}`));
    });
  });
  await page
    .getByRole("button", { name: "Exit safely", exact: true })
    .click({ force: true });
  await exited;
  console.log(
    "Native Wails/WebView2 smoke: 18 profiles, real WebDAV wizard and folder browsing, cancellation, settings, English/Portuguese switching, safe exit — OK",
  );
  const preview = await chromium.launch({
    executablePath:
      "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
    headless: true,
  });
  try {
    const rendered = await preview.newPage({
      viewport: { width: 1180, height: 800 },
    });
    await rendered.setContent(
      markup
        .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, "")
        .replace(/<link\b[^>]*>/gi, ""),
    );
    await rendered.addStyleTag({ content: css });
    await rendered.screenshot({
      path: path.join(root, ".tools", "ui-smoke.png"),
      fullPage: true,
    });
  } finally {
    await preview.close();
  }
} finally {
  dav.closeAllConnections();
  dav.close();
  if (browser) await browser.close().catch(() => {});
  if (child.exitCode === null) child.kill();
}
