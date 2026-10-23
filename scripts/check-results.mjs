import { appendFile, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const checks = [
  ["sdrive", "TestWindowsMountLifecycle"],
  ["sdrive", "TestWindowsMountOpenFiles"],
  ["sdrive", "TestWindowsMountPendingRecovery"],
  ["sdrive/internal/platform", "TestOfficialPrerequisiteSignatures/winfsp"],
  ["sdrive/internal/platform", "TestOfficialPrerequisiteSignatures/webview2"],
];

export function goCheckResults(events) {
  return checks.map(([pkg, test]) => {
    let status = "not_run", reason = "No test execution recorded; opt-in disabled or an earlier step failed";
    let output = [];
    for (const event of events) {
      if (event.Package !== pkg || event.Test !== test) continue;
      if (event.Action === "run") {
        status = "failed";
        reason = "Test started without a terminal result";
        output = [];
      }
      if (event.Action === "output") output.push(event.Output.trim());
      if (event.Action === "pass") { status = "passed"; reason = "Go test completed"; }
      if (event.Action === "fail") { status = "failed"; reason = "Go test failed; see the test step"; }
      if (event.Action === "skip") {
        status = "skipped";
        reason = output.filter((line) => line && !/^\s*(===|---)/.test(line)).join(" ") || "Go test explicitly skipped";
      }
    }
    return { check: test, status, reason };
  });
}

export function renderResults(rows) {
  const cell = (value) => String(value).replaceAll("|", "\\|").replace(/[\r\n]+/g, " ");
  return "## Conditional checks\n\n| Check | Result | Reason |\n| --- | --- | --- |\n" +
    rows.map((row) => `| ${cell(row.check)} | ${cell(row.status)} | ${cell(row.reason)} |`).join("\n") + "\n";
}

async function readOptional(file) {
  try { return await readFile(file, "utf8"); }
  catch (error) { if (error.code === "ENOENT") return ""; throw error; }
}

async function main() {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const directory = path.join(root, ".tools", "check-results");
  const events = [];
  for (const name of ["go-tests.jsonl", "signatures.jsonl"]) {
    const contents = await readOptional(path.join(directory, name));
    for (const line of contents.split(/\r?\n/).filter(Boolean)) events.push(JSON.parse(line.replace(/^\uFEFF/, "")));
  }
  const rows = goCheckResults(events);
  const smoke = path.join(directory, "ui-smoke");
  let runs = [];
  try { runs = await readdir(smoke); }
  catch (error) { if (error.code !== "ENOENT") throw error; }
  for (const run of runs.sort()) {
    const contents = await readOptional(path.join(smoke, run, "result.json"));
    rows.push(contents ? { ...JSON.parse(contents), check: `native-ui/${run}` } :
      { check: `native-ui/${run}`, status: "failed", reason: "Smoke started without a terminal result" });
  }
  if (!runs.length) rows.push({ check: "native-ui", status: "not_run", reason: "Smoke step did not execute; see preceding workflow failures" });
  const markdown = renderResults(rows);
  console.log(markdown);
  await mkdir(directory, { recursive: true });
  await writeFile(path.join(directory, "summary.md"), markdown);
  if (process.env.GITHUB_STEP_SUMMARY) await appendFile(process.env.GITHUB_STEP_SUMMARY, markdown);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main();
