import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { prepareReleaseAssets } from './release-assets.mjs';

const hash = value => createHash('sha256').update(value).digest('hex');

async function fixture(t, platforms) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'sdrive-release-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(path.join(root, 'frontend'));
  await writeFile(path.join(root, 'frontend/package.json'), JSON.stringify({ version: '1.2.3' }));
  for (const platform of platforms) {
    const directory = path.join(root, 'bin', platform === 'linux' ? 'linux-amd64' : '');
    await mkdir(directory, { recursive: true });
    const packages = platform === 'linux'
      ? ['SDrive-1.2.3-linux-amd64.deb', 'SDrive-1.2.3-linux-amd64.tar.gz']
      : ['SDrive-1.2.3-setup.exe', 'SDrive-1.2.3-offline-setup.exe'];
    const lines = [];
    for (const name of [...packages, 'SDrive-source.zip', 'rclone-source.zip']) {
      const content = name === 'rclone-source.zip' ? 'shared engine' : `${platform}/${name}`;
      await writeFile(path.join(directory, name), content);
      lines.push(`${hash(content)}  ${name}`);
    }
    await writeFile(path.join(directory, 'SHA256SUMS'), '\ufeff' + lines.join('\r\n'));
  }
  return root;
}

test('combines platforms, renames sources, removes stale files and hashes final names', async t => {
  const root = await fixture(t, ['windows', 'linux']);
  const releases = path.join(root, 'bin/Releases');
  await mkdir(releases);
  await writeFile(path.join(releases, 'old-setup.exe'), 'old');
  await prepareReleaseAssets(root, 'windows');
  assert.deepEqual((await readdir(releases)).sort(), [
    'SDrive-1.2.3-setup.exe', 'SDrive-1.2.3-offline-setup.exe',
    'SDrive-1.2.3-linux-amd64.deb', 'SDrive-1.2.3-linux-amd64.tar.gz',
    'SDrive-1.2.3-windows-source.zip', 'SDrive-1.2.3-linux-source.zip',
    'rclone-source.zip', 'SHA256SUMS',
  ].sort());
  for (const line of (await readFile(path.join(releases, 'SHA256SUMS'), 'utf8')).trim().split('\n')) {
    const [expected, name] = line.split('  ');
    assert.equal(hash(await readFile(path.join(releases, name))), expected);
  }
  await prepareReleaseAssets(root, 'linux');
  assert.equal((await readdir(releases)).length, 8);
});

for (const platform of ['windows', 'linux']) {
  test(`supports a standalone ${platform} build`, async t => {
    const root = await fixture(t, [platform]);
    const releases = await prepareReleaseAssets(root, platform);
    assert.equal((await readdir(releases)).length, 5);
  });
}

test('rejects corrupt inputs, partial packages and differing shared sources without replacing the release', async t => {
  const root = await fixture(t, ['windows', 'linux']);
  const releases = await prepareReleaseAssets(root, 'windows');
  const original = await readFile(path.join(releases, 'SHA256SUMS'), 'utf8');
  const source = path.join(root, 'bin/linux-amd64/rclone-source.zip');
  await writeFile(source, 'different engine');
  await assert.rejects(prepareReleaseAssets(root, 'windows'), /Checksum mismatch/);
  const manifest = path.join(root, 'bin/linux-amd64/SHA256SUMS');
  await writeFile(manifest, (await readFile(manifest, 'utf8')).replace(hash('shared engine'), hash('different engine')));
  await assert.rejects(prepareReleaseAssets(root, 'windows'), /Platform sources differ/);
  await rm(path.join(root, 'bin/linux-amd64/SDrive-1.2.3-linux-amd64.deb'));
  await assert.rejects(prepareReleaseAssets(root, 'windows'), /Missing linux release packages/);
  assert.equal(await readFile(path.join(releases, 'SHA256SUMS'), 'utf8'), original);
});
