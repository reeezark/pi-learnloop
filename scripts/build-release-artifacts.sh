#!/bin/sh

set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec node - "$script_dir/.." "$@" <<'JS'
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const {spawn} = require('node:child_process');
const [root, version, output, ...extra] = process.argv.slice(2);
const semver = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
let staging;
let child;
let interrupted = false;
let killTimer;
for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(signal, () => {
  interrupted = true;
  if (child) {
    const pid = child.pid;
    try { process.kill(-pid, signal); } catch {}
    clearTimeout(killTimer);
    killTimer = setTimeout(() => { try { process.kill(-pid, 'SIGKILL'); } catch {} }, 2000);
  }
});
function fail(message) { throw new Error(message); }
function command(file, args, options = {}) {
  if (interrupted) fail('build interrupted');
  return new Promise((resolve, reject) => {
    child = spawn(file, args, {cwd: root, detached: true, ...options});
    let output = '';
    child.stdout?.on('data', data => { output += data; });
    child.stderr?.resume();
    child.once('error', () => reject(new Error(`${file} failed`)));
    child.once('close', code => {
      child = undefined;
      clearTimeout(killTimer);
      if (interrupted || code !== 0) reject(new Error(interrupted ? 'build interrupted' : `${file} failed`));
      else resolve(output);
    });
  });
}
async function main() {
try {
  if (!version || !output || extra.length) fail('usage: build-release-artifacts.sh <version> <output-directory>');
  if (!semver.test(version) || version !== JSON.parse(fs.readFileSync(path.join(root, 'package.json'))).version)
    fail('version must be canonical SemVer equal to package.json');
  if (process.platform !== 'darwin') fail('macOS build host required');
  const repository = fs.realpathSync(root);
  const requested = path.resolve(output);
  const destination = path.join(fs.realpathSync(path.dirname(requested)), path.basename(requested));
  if (destination === repository || destination.startsWith(repository + path.sep) || destination === path.parse(destination).root)
    fail('output must be outside the repository and filesystem root');
  function checkOutput() {
    const stat = fs.lstatSync(destination, {throwIfNoEntry: false});
    if (stat && (!stat.isDirectory() || stat.isSymbolicLink() || fs.readdirSync(destination).length))
      fail('output must be absent or an empty real directory');
  }
  checkOutput();
  // No ambient workspace, toolchain switch, user Go flags, or architecture tuning.
  const env = {...process.env, GOENV: 'off', GOTOOLCHAIN: 'local', GOWORK: 'off',
    GOFLAGS: '', GOEXPERIMENT: '', CGO_ENABLED: '0', GOOS: 'darwin',
    GOAMD64: 'v1', GOARM64: 'v8.0'};
  delete env.GOROOT;
  if (!/^go version go1\.27\.1 darwin\/(arm64|amd64)\n$/.test(await command('go', ['version'], {env})))
    fail('exact Go 1.27.1 release toolchain required');
  await command('git', ['rev-parse', '--verify', 'HEAD']);
  staging = fs.mkdtempSync(path.join(path.dirname(destination), '.pi-learnloop-release-'));
  const artifacts = path.join(staging, 'artifacts');
  fs.mkdirSync(artifacts, {mode: 0o755});
  const sums = [];
  for (const arch of ['arm64', 'amd64']) {
    const work = path.join(staging, arch);
    fs.mkdirSync(work, {mode: 0o700});
    const binary = path.join(work, 'pi-learnloop');
    await command('go', ['build', '-mod=readonly', '-trimpath', '-buildvcs=true',
      `-ldflags=-X main.version=${version}`, '-o', binary, './cmd/pi-learnloop'],
      {env: {...env, GOARCH: arch}, stdio: 'inherit'});
    fs.chmodSync(binary, 0o755);
    fs.copyFileSync(path.join(root, 'LICENSE'), path.join(work, 'LICENSE'));
    fs.chmodSync(path.join(work, 'LICENSE'), 0o644);
    fs.writeFileSync(path.join(work, 'README.md'),
      `# Pi LearnLoop ${version} — darwin/${arch}\n\n` +
      'Unsigned local verification candidate; not a stable release.\n' +
      'Requires macOS 13+, Git, Node >=22.19.0, and Pi 0.84.3.\n' +
      'Do not bypass Gatekeeper or quarantine checks.\n' +
      'Stable installation requires separately verified Developer ID signing and notarization.\n' +
      'The npm Pi extension is separate; install the same product version.\n' +
      'Place a verified daemon in a user-selected PATH directory, then run\n' +
      '`pi-learnloop version` and manually start `pi-learnloop daemon` in the foreground.\n' +
      'No automatic startup, update, or history deletion is performed.\n', {mode: 0o644});
    const name = `pi-learnloop_${version}_darwin_${arch}.zip`;
    await command('/usr/bin/zip', ['-X', '-q', path.join(artifacts, name), 'pi-learnloop', 'LICENSE', 'README.md'], {cwd: work});
    const hash = crypto.createHash('sha256').update(fs.readFileSync(path.join(artifacts, name))).digest('hex');
    sums.push(`${hash}  ${name}\n`);
  }
  fs.writeFileSync(path.join(artifacts, 'SHA256SUMS'), sums.join(''), {mode: 0o644});
  await command(path.join(root, 'scripts/verify-release-artifacts.sh'), [version, artifacts], {env, stdio: 'inherit'});
  if (interrupted) fail('build interrupted');
  checkOutput();
  // Rename commits the complete artifact set; a racing nonempty destination fails.
  fs.renameSync(artifacts, destination);
  console.log('Built unsigned macOS candidates; not eligible for publication.');
} catch (error) {
  console.error(`release build: ${error.message}`);
  process.exitCode = 1;
} finally {
  if (staging) fs.rmSync(staging, {recursive: true, force: true});
}
}
main();
JS
