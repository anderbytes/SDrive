import { createHash } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { access, copyFile, mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

async function checksum(file) {
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  return hash.digest('hex');
}

export async function prepareReleaseAssets(root, platform) {
  if (!['windows', 'linux'].includes(platform)) throw new Error('Expected windows or linux');
  const { version } = JSON.parse(await readFile(path.join(root, 'frontend/package.json'), 'utf8'));
  if (!/^\d+\.\d+\.\d+$/.test(version)) throw new Error('Expected a stable three-part version');
  const bin = path.join(root, 'bin');
  const assets = new Map();
  for (const target of ['windows', 'linux']) {
    const directory = target === 'windows' ? bin : path.join(bin, 'linux-amd64');
    const packages = target === 'windows'
      ? [`SDrive-${version}-setup.exe`, `SDrive-${version}-offline-setup.exe`]
      : [`SDrive-${version}-linux-amd64.deb`, `SDrive-${version}-linux-amd64.tar.gz`];
    const available = await Promise.all(packages.map(async name => {
      try { await access(path.join(directory, name)); return true; }
      catch (error) { if (error.code === 'ENOENT') return false; throw error; }
    }));
    if (target !== platform && available.every(value => !value)) continue;
    if (!available.every(Boolean)) throw new Error(`Missing ${target} release packages for ${version}`);
    const manifest = new Map((await readFile(path.join(directory, 'SHA256SUMS'), 'utf8'))
      .trim().split(/\r?\n/).map(line => line.trim().split(/\s+/, 2).reverse()));
    for (const name of [...packages, 'SDrive-source.zip', 'rclone-source.zip']) {
      const source = path.join(directory, name);
      const hash = await checksum(source);
      if (hash !== manifest.get(name)) throw new Error(`Checksum mismatch: ${source}`);
      const destination = name === 'SDrive-source.zip' ? `SDrive-${version}-${target}-source.zip` : name;
      if (assets.has(destination) && assets.get(destination).hash !== hash) {
        throw new Error(`Platform sources differ: ${destination}`);
      }
      assets.set(destination, { source, hash });
    }
  }
  await mkdir(bin, { recursive: true });
  const stage = await mkdtemp(path.join(bin, '.release-assets-'));
  try {
    const lines = [];
    for (const [name, { source, hash }] of assets) {
      const destination = path.join(stage, name);
      await copyFile(source, destination);
      if (await checksum(destination) !== hash) throw new Error(`Copied checksum mismatch: ${name}`);
      lines.push(`${hash}  ${name}`);
    }
    await writeFile(path.join(stage, 'SHA256SUMS'), lines.join('\n') + '\n');
    const releases = path.join(bin, 'Releases');
    await rm(releases, { recursive: true, force: true });
    await rename(stage, releases);
    return releases;
  } finally {
    await rm(stage, { recursive: true, force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  console.log(`Release assets ready at ${await prepareReleaseAssets(root, process.argv[2])}`);
}
