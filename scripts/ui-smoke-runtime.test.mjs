import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { debuggerEndpoint, runSmoke, smokeEnvironment } from "./ui-smoke-runtime.mjs";
import { goCheckResults, renderResults } from "./check-results.mjs";

test("smoke environment replaces case-insensitive profile and WebView overrides without changing the parent", () => {
  const source = { LocalAppData: "personal", WEBVIEW2_USER_DATA_FOLDER: "personal-webview", webview2_additional_browser_arguments: "--remote-debugging-port=9337", PATH: "tools" };
  assert.deepEqual(smokeEnvironment("isolated", source), { LOCALAPPDATA: "isolated", PATH: "tools" });
  assert.equal(source.LocalAppData, "personal");
});

test("debugger discovery accepts only an explicit loopback endpoint from its profile", () => {
  assert.equal(debuggerEndpoint("12345\r\n/devtools/browser/abc-123\r\n"), "ws://127.0.0.1:12345/devtools/browser/abc-123");
  for (const contents of ["", "0\n/devtools/browser/abc", "65536\n/devtools/browser/abc", "123abc\n/devtools/browser/abc", "80\nws://remote/", "80\n/devtools/browser/../../x"])
    assert.throws(() => debuggerEndpoint(contents));
});

test("conditional results preserve skips, incomplete tests and later signature execution", () => {
  const event = (Action, Test, Output) => ({ Action, Package: "sdrive/internal/platform", Test, Output });
  const testName = "TestOfficialPrerequisiteSignatures/webview2";
  const skipped = [event("run", testName), event("output", testName, "  payload not downloaded\n"), event("skip", testName), event("pass", "TestOfficialPrerequisiteSignatures")];
  let rows = goCheckResults(skipped);
  assert.equal(rows[4].status, "skipped");
  assert.equal(rows[4].reason, "payload not downloaded");
  assert.equal(rows[0].status, "not_run");
  rows = goCheckResults([...skipped, event("run", testName)]);
  assert.equal(rows[4].status, "failed");
  rows = goCheckResults([...skipped, event("run", testName), event("pass", testName)]);
  assert.equal(rows[4].status, "passed");
  assert.equal(goCheckResults([event("fail", testName)])[4].status, "failed");
  assert.match(renderResults([{ check: "a|b", status: "skipped", reason: "a\nb" }]), /a\\\|b.*skipped.*a b/);
});

test("failed build and failed launch remove only their own unique directories", { skip: process.platform !== "win32" }, async (t) => {
  const workspace = await mkdtemp(path.join(tmpdir(), "sdrive-ui-smoke-test-"));
  t.after(async () => {
    assert.equal(path.dirname(workspace), path.resolve(tmpdir()));
    assert(path.basename(workspace).startsWith("sdrive-ui-smoke-test-"));
    await rm(workspace, { recursive: true, force: true });
  });
  const runs = path.join(workspace, ".tools/ui-smoke");
  await mkdir(path.join(runs, "independent-instance"), { recursive: true });
  await writeFile(path.join(runs, "independent-instance", "sentinel"), "keep");
  await Promise.all([
    assert.rejects(runSmoke(() => assert.fail("must not exercise UI"), { workspace, buildBinary: async () => { throw new Error("directed build failure"); } }), /directed build failure/),
    assert.rejects(runSmoke(() => assert.fail("must not exercise UI"), { workspace, buildBinary: async () => {} }), /could not start: ENOENT/),
  ]);
  await assert.rejects(runSmoke(() => assert.fail("must not exercise UI"), {
    workspace, buildBinary: async () => { process.emit("SIGINT"); },
  }), /Native smoke interrupted/);
  assert.deepEqual(await readdir(runs), ["independent-instance"]);
  assert.equal(await readFile(path.join(runs, "independent-instance", "sentinel"), "utf8"), "keep");
  const resultsPath = path.join(workspace, ".tools/check-results/ui-smoke");
  const results = await readdir(resultsPath);
  assert.equal(results.length, 3);
  const records = await Promise.all(results.map(async (run) => JSON.parse(await readFile(path.join(resultsPath, run, "result.json"), "utf8"))));
  assert(records.some((record) => record.status === "failed" && record.reason === "directed build failure"));
  assert(records.some((record) => record.status === "failed" && record.reason.includes("could not start: ENOENT")));
  assert(records.some((record) => record.status === "failed" && record.reason === "Native smoke interrupted"));
});
