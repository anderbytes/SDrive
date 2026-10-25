import { chromium } from "../frontend/node_modules/playwright/index.mjs";
import { spawn, execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { setTimeout as delay } from "node:timers/promises";
import path from "node:path";
import assert from "node:assert/strict";

const exec = promisify(execFile);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

export function smokeEnvironment(profile, source = process.env) {
  return {
    ...Object.fromEntries(Object.entries(source).filter(([key]) =>
      key.toUpperCase() !== "LOCALAPPDATA" && !key.toUpperCase().startsWith("WEBVIEW2_"))),
    LOCALAPPDATA: profile,
  };
}

export function debuggerEndpoint(contents) {
  const [port, endpoint] = contents.trim().split(/\r?\n/);
  assert(/^[0-9]+$/.test(port) && Number(port) > 0 && Number(port) <= 65535,
    "Invalid WebView2 debugger port");
  assert(/^\/devtools\/browser\/[a-zA-Z0-9-]+$/.test(endpoint), "Invalid WebView2 debugger path");
  return `ws://127.0.0.1:${Number(port)}${endpoint}`;
}

async function build(binary, signal) {
  await exec(path.join(root, ".tools/go/bin/go.exe"), [
    "build", "-tags", "production", "-ldflags", "-X main.uiSmoke=true", "-o", binary, ".",
  ], { cwd: root, windowsHide: true, timeout: 120000, signal });
}

// A read-only local fixture exercises the real rclone handoff without an account.
function webdav() {
  return createServer((request, response) => {
    const pathname = new URL(request.url, "http://127.0.0.1").pathname;
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
    const entries = [pathname.endsWith("/") ? pathname : pathname + "/"];
    if ((pathname === "/dav" || pathname === "/dav/") && request.headers.depth !== "0")
      entries.push("/dav/Documents/");
    response.writeHead(207, { "Content-Type": "application/xml; charset=utf-8" });
    response.end(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">${entries.map((href) => `<d:response><d:href>${href}</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><d:getlastmodified>Thu, 22 Oct 2026 12:00:00 GMT</d:getlastmodified></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`).join("")}</d:multistatus>`);
  });
}

async function bounded(promise, milliseconds, message, signal) {
  let timer, aborted;
  try {
    const pending = [promise, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(message)), milliseconds);
    })];
    if (signal) pending.push(new Promise((_, reject) => {
      aborted = () => reject(signal.reason);
      signal.addEventListener("abort", aborted, { once: true });
      if (signal.aborted) aborted();
    }));
    return await Promise.race(pending);
  } finally {
    clearTimeout(timer);
    if (aborted) signal.removeEventListener("abort", aborted);
  }
}

export async function runSmoke(exercise, { buildBinary = build, workspace = root } = {}) {
  const runs = path.join(workspace, ".tools", "ui-smoke");
  await mkdir(runs, { recursive: true });
  const run = await mkdtemp(path.join(runs, "run-"));
  const artifacts = path.join(workspace, ".tools", "check-results", "ui-smoke", path.basename(run));
  const profile = path.join(run, "profile");
  const binary = path.join(run, "SDrive-ui-check.exe");
  const result = { check: "native-ui", status: "failed", run: path.basename(run), reason: "Smoke did not complete" };
  let child, exited, browser, dav, page, failure;
  const controller = new AbortController();
  const interrupt = () => controller.abort(new Error("Native smoke interrupted"));
  process.once("SIGINT", interrupt);
  process.once("SIGTERM", interrupt);
  const cleanupErrors = [];
  const cleanup = async (label, action) => {
    try { await action(); } catch { cleanupErrors.push(label); }
  };
  try {
    await mkdir(artifacts, { recursive: true });
    assert.equal(process.platform, "win32", "Native smoke requires Windows and WebView2");
    await mkdir(profile);
    await buildBinary(binary, controller.signal);
    controller.signal.throwIfAborted();
    dav = webdav();
    await new Promise((resolve, reject) => {
      dav.once("error", reject);
      dav.listen(0, "127.0.0.1", resolve);
    });
    child = spawn(binary, [], { cwd: workspace, windowsHide: true, env: smokeEnvironment(profile) });
    let spawnError;
    exited = new Promise((resolve) => {
      child.once("error", (error) => { spawnError = error; resolve(null); });
      child.once("exit", (code) => resolve(code));
    });
    // Drain output but never persist raw application diagnostics or profile data.
    child.stdout.resume();
    child.stderr.resume();
    const deadline = Date.now() + 45000;
    while (Date.now() < deadline) {
      controller.signal.throwIfAborted();
      if (spawnError) throw new Error(`Smoke process could not start: ${spawnError.code}`);
      if (child.exitCode !== null || child.signalCode !== null)
        throw new Error(`Smoke process exited before WebView2 was ready (code=${child.exitCode})`);
      try {
        const contents = await readFile(path.join(profile, "SDrive", "webview", "EBWebView", "DevToolsActivePort"), "utf8");
        const endpoint = debuggerEndpoint(contents);
        browser = await chromium.connectOverCDP(endpoint, { timeout: 2000 });
        result.endpoint = endpoint;
        break;
      } catch (error) {
        if (error.code !== "ENOENT" && error.code !== "EBUSY" && error.code !== "EPERM" && error.name !== "TimeoutError" && !error.message.includes("ECONNREFUSED"))
          throw error;
      }
      await delay(250, undefined, { signal: controller.signal });
    }
    assert(browser, "WebView2 debugger unavailable after 45s; check the runtime and Windows graphical session");
    const context = browser.contexts()[0];
    context.setDefaultTimeout(15000);
    page = context.pages()[0] ?? await context.waitForEvent("page", { timeout: 15000 });
    await bounded(exercise({ page, dav, child, exited, artifacts, profile }), 120000, "Native UI scenario exceeded 120s", controller.signal);
    assert.equal(await bounded(exited, 10000, "Application did not exit safely"), 0, "Application exited unsuccessfully");
    result.status = "passed";
    result.reason = "Native WebView2 scenario and safe exit completed";
  } catch (error) {
    failure = error;
    result.reason = error.message.split("\n")[0];
    if (page && !page.isClosed())
      await cleanup("failure screenshot", () => page.screenshot({ path: path.join(artifacts, "failure.png"), mask: [page.locator("[data-sensitive]")], timeout: 5000 }));
  } finally {
    if (dav) await cleanup("WebDAV shutdown", async () => {
      dav.closeAllConnections();
      await new Promise((resolve, reject) => dav.close((error) => error && error.code !== "ERR_SERVER_NOT_RUNNING" ? reject(error) : resolve()));
    });
    if (child?.pid && child.exitCode === null && child.signalCode === null) {
      await cleanup("owned process tree shutdown", async () => {
        await exec("taskkill.exe", ["/PID", String(child.pid), "/T", "/F"], { windowsHide: true, timeout: 10000 });
        await bounded(exited, 10000, "Owned application did not terminate");
      });
    }
    if (browser?.isConnected()) await cleanup("CDP disconnect", () => bounded(browser.close(), 5000, "CDP disconnect timed out"));
    await cleanup("temporary profile removal", async () => {
      assert.equal(path.dirname(path.resolve(run)), path.resolve(runs));
      assert(path.basename(run).startsWith("run-"));
      await rm(run, { recursive: true, force: true, maxRetries: 8, retryDelay: 250 });
    });
    if (cleanupErrors.length) {
      result.status = "failed";
      result.reason += `; cleanup failed: ${cleanupErrors.join(", ")}`;
      failure ??= new Error(result.reason);
    }
    process.removeListener("SIGINT", interrupt);
    process.removeListener("SIGTERM", interrupt);
    await writeFile(path.join(artifacts, "result.json"), JSON.stringify(result, null, 2));
    console.log(`Native UI smoke: ${result.status} — ${result.reason} (${result.run})`);
  }
  if (failure) throw failure;
  return result;
}
