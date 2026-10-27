import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const source = path.dirname(fileURLToPath(import.meta.url));

export function prepareNetworkOverlay(rcloneDir, output) {
  const replace = {};
  mkdirSync(output, { recursive: true });
  function patch(relative, changes) {
    let text = readFileSync(path.join(rcloneDir, relative), "utf8").replaceAll("\r\n", "\n");
    for (const [before, after] of changes) {
      if (text.split(before).length !== 2) throw Error(`Engine network hook changed: ${relative}: ${before}`);
      text = text.replace(before, after);
    }
    const target = path.join(output, relative.replaceAll("/", "_"));
    writeFileSync(target, text);
    replace[path.join(rcloneDir, relative)] = target;
  }
  patch("fs/fshttp/dialer.go", [
    ["n, err = c.Conn.Read(b)", "n, err = c.Conn.Read(b)\n\taccounting.RecordNetworkBytes(false, n)"],
    ["n, err = c.Conn.Write(b)", "n, err = c.Conn.Write(b)\n\taccounting.RecordNetworkBytes(true, n)"],
  ]);
  patch("fs/accounting/accounting.go", [
    ["type Account struct {", "type Account struct {\n\tnetworkDirection int"],
    ["func (acc *Account) accountReadN(n int64) {", "func (acc *Account) accountReadN(n int64) {\n\trecordFileBytes(acc.networkDirection, n)"],
  ]);
  patch("fs/accounting/transfer.go", [[
    "tr.acc = newAccountSizeName(ctx, tr.stats, in, tr.size, tr.remote)",
    `tr.acc = newAccountSizeName(ctx, tr.stats, in, tr.size, tr.remote)
        if !tr.checking && tr.srcFs != nil {
            if !tr.srcFs.Features().IsLocal && (tr.dstFs == nil || tr.dstFs.Features().IsLocal) {
                tr.acc.networkDirection = 2
            } else if tr.srcFs.Features().IsLocal && tr.dstFs != nil && !tr.dstFs.Features().IsLocal {
                tr.acc.networkDirection = 1
            }
        }`,
  ]]);
  for (const name of ["network", "network_test"]) {
    replace[path.join(rcloneDir, "fs/accounting", `${name}.go`)] = path.join(source, `${name}.go.in`);
  }
  replace[path.join(rcloneDir, "fs/fshttp/network_test.go")] = path.join(source, "network_transport_test.go.in");
  return replace;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.stdout.write(JSON.stringify(prepareNetworkOverlay(process.argv[2], process.argv[3])));
}
