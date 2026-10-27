import { execFileSync } from "node:child_process";
import { cpSync, mkdirSync, readFileSync, writeFileSync, readdirSync } from "node:fs";
import { prepareNetworkOverlay } from "./engine-overlay/prepare-network.mjs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const output = path.join(root, ".tools/engine-overlay-linux");
const go = process.env.SDRIVE_GO || "go";
const run = (args, cwd = root) => execFileSync(go, args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] });
mkdirSync(output, { recursive: true });
const rclone = JSON.parse(run(["mod", "download", "-json", "github.com/rclone/rclone@v1.75.2"]));
const smb = JSON.parse(run(["mod", "download", "-json", "github.com/cloudsoda/go-smb2@v0.0.0-20260701064823-d8c5600d73b8"]));
if (rclone.Sum !== "h1:0cqa6P3WmhGnu2NeOdZSetJnacpwCRtpsNbHOICqPk8=") throw Error("Invalid rclone source checksum");
const expected = readFileSync(path.join(rclone.Dir, "go.sum"), "utf8").split("\n").find(line => line.startsWith(`github.com/cloudsoda/go-smb2 ${smb.Version} `))?.split(" ")[2];
if (!expected || smb.Sum !== expected) throw Error("Invalid SMB source checksum");
const rcloneDir = path.join(output, "rclone"), smbDir = path.join(output, "go-smb2");
// Copied module trees retain read-only modes, so unlock only our private build workspace.
execFileSync("chmod", ["-R", "u+rwX", output]);
cpSync(rclone.Dir, rcloneDir, { recursive: true, force: true });
cpSync(smb.Dir, smbDir, { recursive: true, force: true });
// Work on private writable module files; verified upstream modules stay unchanged.
const modFile = path.join(output, "build.mod");
writeFileSync(modFile, readFileSync(path.join(rclone.Dir, "go.mod")));
writeFileSync(path.join(output, "build.sum"), readFileSync(path.join(rclone.Dir, "go.sum")));
run(["mod", "edit", "-modfile", modFile, `-replace=github.com/cloudsoda/go-smb2=${smbDir}`]);
const Replace = {}, digest = createHash("sha256");
for (const [dir, prefix, names] of [[smbDir, "", ["smb_transport", "smb_transport_test"]], [rcloneDir, "backend/smb", ["transport_command", "transport_command_test"]]]) {
  for (const name of names) {
    const source = path.join(root, "scripts/engine-overlay", `${name}.go.in`);
    Replace[path.join(dir, prefix, `${name}.go`)] = source;
    digest.update(name).update(readFileSync(source));
  }
}
const overlay = path.join(output, "overlay.json");
Object.assign(Replace, prepareNetworkOverlay(rcloneDir, path.join(output, "network")));
for (const name of readdirSync(path.join(root, "scripts/engine-overlay")).sort()) {
  digest.update(name).update(readFileSync(path.join(root, "scripts/engine-overlay", name)));
}
writeFileSync(overlay, JSON.stringify({ Replace }));
if (process.argv.includes("--test")) {
  run(["test", "-mod=readonly", "-modfile", modFile, "-overlay", overlay, "./fs/accounting", "./fs/fshttp", "-run", "^TestNetwork", "-count=1"], rcloneDir);
  run(["test", "-mod=readonly", "-modfile", modFile, "-overlay", overlay, "github.com/cloudsoda/go-smb2", "-run", "^TestShareTransportEncryption$", "-count=1"], rcloneDir);
  run(["test", "-mod=readonly", "-modfile", modFile, "-overlay", overlay, "./backend/smb", "-run", "^TestTransportCommand", "-count=1"], rcloneDir);
}
const binary = path.join(root, ".tools/rclone/rclone");
mkdirSync(path.dirname(binary), { recursive: true });
run(["build", "-mod=readonly", "-modfile", modFile, "-overlay", overlay, "-trimpath", "-ldflags", "-s -w -X github.com/rclone/rclone/fs.Version=v1.75.2", "-o", binary, "."], rcloneDir);
writeFileSync(path.join(root, ".tools/rclone/build.json"), JSON.stringify({ sourceSum: rclone.Sum, overlayDigest: digest.digest("hex"), compiler: run(["env", "GOVERSION"]).trim(), sha256: createHash("sha256").update(readFileSync(binary)).digest("hex") }, null, 2));
console.log("Verified Linux engine ready.");
