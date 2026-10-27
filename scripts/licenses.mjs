import { execFile } from "node:child_process";
import { promisify } from "node:util";
import {
  mkdir,
  readFile,
  readdir,
  copyFile,
  writeFile,
  chmod,
  rm,
} from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const exec = promisify(execFile);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const linux = process.platform === "linux";
const go = linux ? process.env.SDRIVE_GO || "go" : path.join(root, ".tools/go/bin/go.exe");
const distribution = process.env.SDRIVE_DIST_DIR || path.join(root, "bin");
const destination = path.join(distribution, "licenses");
if (!destination.startsWith(path.join(root, 'bin') + path.sep)) throw new Error('Invalid output directory');
await rm(destination,{recursive:true,force:true});
await mkdir(destination, { recursive: true });
async function copyWritable(source, target) {
  try {
    await chmod(target, 0o600);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  await copyFile(source, target);
  await chmod(target, 0o600);
}
const records = new Map();
for (const binary of linux ? ["SDrive", "rclone"] : ["SDrive.exe", "rclone.exe"]) {
  const { stdout } = await exec(
    go,
    ["version", "-m", "-json", path.join(distribution, binary)],
    { cwd: root },
  );
  const info = JSON.parse(stdout);
  for (const module of [info, ...info.Deps]) {
    const dep = module.Replace?.Version && module.Replace.Version !== "(devel)" ? module.Replace : module;
    const name = dep.Path;
    const version = dep.Version;
    if (name && version && version !== "(devel)")
      records.set(`${name}@${version}`, { path: name, version });
  }
}
// rclone's main-module source is not a dependency entry in its build info.
records.set("github.com/rclone/rclone@v1.75.2", {
  path: "github.com/rclone/rclone",
  version: "v1.75.2",
});
const queue = [...records.entries()];
let completed = 0;
async function copyNotices(directory, target) {
  let found = 0;
  for (const item of await readdir(directory, { withFileTypes: true })) {
    if (!/^(licen[cs]es?|copying|notice|copyright)(?:[._-].*)?$/i.test(item.name)) continue;
    const source = path.join(directory, item.name);
    if (item.isFile()) {
      await mkdir(target, { recursive: true });
      await copyWritable(source, path.join(target, item.name));
      found++;
    } else if (item.isDirectory()) {
      const walk = async (from, to) => {
        await mkdir(to, { recursive: true });
        for (const entry of await readdir(from, { withFileTypes: true })) {
          if (entry.isDirectory())
            await walk(path.join(from, entry.name), path.join(to, entry.name));
          else if (entry.isFile()) {
            await copyWritable(
              path.join(from, entry.name),
              path.join(to, entry.name),
            );
            found++;
          }
        }
      };
      await walk(source, path.join(target, item.name));
    }
  }
  return found;
}
await Promise.all(
  Array.from({ length: 6 }, async () => {
    while (queue.length) {
      const [id, record] = queue.shift();
      let stdout;
      try {
        ({ stdout } = await exec(go, ["mod", "download", "-json", id], {
          cwd: root,
          maxBuffer: 8 * 1024 * 1024,
          timeout: 180000,
        }));
      } catch (error) {
        throw new Error(
          `Could not retrieve source/license for ${id}: ${error.message}`,
        );
      }
      const module = JSON.parse(stdout);
      if (module.Error || !module.Dir)
        throw new Error(`Source unavailable: ${id}`);
      const slug = id.replaceAll("/", "__");
      const target = path.join(destination, "go", slug);
      if (!(await copyNotices(module.Dir, target)))
        throw new Error(
          `License not found in ${id}; review before distribution`,
        );
      record.sum = module.Sum;
      record.source = `https://proxy.golang.org/${record.path.replace(/[A-Z]/g, (c) => "!" + c.toLowerCase())}/@v/${record.version}.zip`;
      if (id === "github.com/rclone/rclone@v1.75.2") {
        for (const nested of [
          "cmd/bisync/LICENSE.cjnaz",
          "cmd/serve/dlna/LICENSE.anacrolix",
          "fs/rc/js/LICENSE.wasmexec",
        ]) {
          await mkdir(path.dirname(path.join(target, nested)), {
            recursive: true,
          });
          await copyWritable(
            path.join(module.Dir, nested),
            path.join(target, nested),
          );
        }
        await copyWritable(
          module.Zip,
          path.join(distribution, "rclone-source.zip"),
        );
        await mkdir(path.join(destination, "engine-overlay"), { recursive: true });
        for (const entry of await readdir(path.join(root, "scripts/engine-overlay"))) {
          await copyWritable(path.join(root, "scripts/engine-overlay", entry), path.join(destination, "engine-overlay", entry));
        }
        for (const entry of linux ? ["build-linux.sh", "engine-overlay.mjs"] : ["bootstrap.ps1", "engine-overlay.ps1"]) {
          await copyWritable(path.join(root, "scripts", entry), path.join(destination, "engine-overlay", entry));
        }
        await copyWritable(path.join(root, ".tools/rclone/build.json"), path.join(destination, "engine-overlay/build.json"));
      }
      if (id === "github.com/cloudsoda/go-smb2@v0.0.0-20260701064823-d8c5600d73b8") {
        await mkdir(path.join(destination, "engine-overlay"), { recursive: true });
        await copyWritable(module.Zip, path.join(destination, "engine-overlay", "go-smb2-source.zip"));
      }
      completed++;
      if (completed % 30 === 0)
        console.log(`Go licenses: ${completed}/${records.size}`);
    }
  }),
);
const lock = JSON.parse(
  await readFile(path.join(root, "frontend/package-lock.json"), "utf8"),
);
for (const [location, record] of Object.entries(lock.packages)) {
  if (!location || record.dev || !location.startsWith("node_modules/"))
    continue;
  const directory = path.join(root, "frontend", location);
  const name = location.slice("node_modules/".length);
  const target = path.join(destination, "npm", name.replaceAll("/", "__"));
  if (!(await copyNotices(directory, target))) {
    if (name === "@wailsio/runtime") {
      const { stdout } = await exec(
        go,
        [
          "mod",
          "download",
          "-json",
          "github.com/wailsapp/wails/v3@v3.0.0-beta.34",
        ],
        { cwd: root },
      );
      if (!(await copyNotices(JSON.parse(stdout).Dir, target)))
        throw new Error("Wails license missing");
    } else throw new Error(`npm license missing: ${name}`);
  }
}
await copyWritable(
  linux ? path.join((await exec(go, ["env", "GOROOT"], { cwd: root })).stdout.trim(), "LICENSE") : path.join(root, ".tools/go/LICENSE"),
  path.join(destination, "Go-LICENSE"),
);
await writeFile(
  path.join(destination, "modules.json"),
  JSON.stringify([...records.values()], null, 2),
);
console.log(
  `Licenses collected: ${records.size} Go modules and production npm dependencies.`,
);
