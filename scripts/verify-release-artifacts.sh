#!/bin/sh

set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec node - "$script_dir/.." "$@" <<'JS'
'use strict';
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const {spawnSync} = require('node:child_process');
const [root, version, directory, ...extra] = process.argv.slice(2);
const maxBytes = 128 * 1024 * 1024;
let scratch;
function requireValue(ok, message) { if (!ok) throw new Error(message); }
const env = {...process.env, GOENV: 'off', GOTOOLCHAIN: 'local', GOWORK: 'off', GOFLAGS: ''};
delete env.GOROOT;
function command(file, args, binary = false) {
  const result = spawnSync(file, args, {env, encoding: binary ? null : 'utf8',
    timeout: 30000, maxBuffer: maxBytes});
  requireValue(!result.error && result.status === 0, `${file} inspection failed`);
  return result.stdout;
}
function readRegular(file, limit) {
  const stat = fs.lstatSync(file);
  requireValue(stat.isFile() && !stat.isSymbolicLink() && stat.size <= limit, 'invalid artifact file');
  return fs.readFileSync(file);
}
function diagnostic(binary, expectedArch) {
  const bytes = readRegular(binary, maxBytes);
  requireValue(bytes.length >= 32 && bytes.readUInt32LE(0) === 0xfeedfacf &&
    bytes.readUInt32LE(4) === (expectedArch === 'arm64' ? 0x100000c : 0x1000007) &&
    bytes.readUInt32LE(12) === 2, 'wrong Mach-O architecture or file type');
  const commandsEnd = 32 + bytes.readUInt32LE(20);
  requireValue(commandsEnd <= bytes.length, 'invalid Mach-O commands');
  const segments = [];
  let offset = 32;
  for (let i = 0; i < bytes.readUInt32LE(16); i++) {
    requireValue(offset + 8 <= commandsEnd, 'invalid Mach-O command');
    const kind = bytes.readUInt32LE(offset), size = bytes.readUInt32LE(offset + 4);
    requireValue(size >= 8 && offset + size <= commandsEnd, 'invalid Mach-O command size');
    if (kind === 0x19) {
      requireValue(size >= 72, 'invalid Mach-O segment');
      segments.push({address: bytes.readBigUInt64LE(offset + 24),
        file: bytes.readBigUInt64LE(offset + 40), size: bytes.readBigUInt64LE(offset + 48)});
    }
    offset += size;
  }
  requireValue(offset === commandsEnd, 'invalid Mach-O command count');
  function fileOffset(address, length) {
    const segment = segments.find(s => address >= s.address && address + BigInt(length) <= s.address + s.size);
    requireValue(segment, 'version value is not file-backed');
    const result = segment.file + address - segment.address;
    requireValue(result >= 0 && result + BigInt(length) <= BigInt(bytes.length), 'invalid version offset');
    return Number(result);
  }
  // Inspect the actual Go string header, not an unrelated version-like string.
  // Native nm understands both thin Mach-O architectures; nothing is executed.
  const symbols = [...command('/usr/bin/nm', [binary]).matchAll(/^([0-9a-fA-F]+) [a-zA-Z] _main\.version$/gm)];
  requireValue(symbols.length === 1, 'missing or ambiguous diagnostic version symbol');
  const header = fileOffset(BigInt('0x' + symbols[0][1]), 16);
  const length = bytes.readBigUInt64LE(header + 8);
  requireValue(length === BigInt(Buffer.byteLength(version)), 'wrong diagnostic version length');
  const value = fileOffset(bytes.readBigUInt64LE(header), Number(length));
  requireValue(bytes.subarray(value, value + Number(length)).equals(Buffer.from(version)), 'wrong diagnostic version');
}
try {
  requireValue(version && directory && !extra.length, 'usage: verify-release-artifacts.sh <version> <artifact-directory>');
  requireValue(version === JSON.parse(fs.readFileSync(path.join(root, 'package.json'))).version &&
    /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/.test(version),
    'version must be canonical SemVer equal to package.json');
  requireValue(process.platform === 'darwin', 'macOS verification host required');
  requireValue(/^go version go1\.27\.1 darwin\/(arm64|amd64)\n$/.test(command('go', ['version'])), 'exact Go 1.27.1 required');
  requireValue(fs.lstatSync(directory).isDirectory() && !fs.lstatSync(directory).isSymbolicLink(), 'invalid artifact directory');
  const dir = fs.realpathSync(directory);
  const names = ['arm64', 'amd64'].map(arch => `pi-learnloop_${version}_darwin_${arch}.zip`);
  requireValue(JSON.stringify(fs.readdirSync(dir).sort()) === JSON.stringify(['SHA256SUMS', ...names].sort()), 'wrong artifact file set');
  scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'pi-learnloop-release-verify-'));
  // All later inspection uses the same private bytes that were checksummed.
  const expectedSums = names.map(name => {
    const bytes = readRegular(path.join(dir, name), maxBytes);
    fs.writeFileSync(path.join(scratch, name), bytes, {mode: 0o600});
    return crypto.createHash('sha256').update(bytes).digest('hex') + `  ${name}\n`;
  }).join('');
  requireValue(readRegular(path.join(dir, 'SHA256SUMS'), 4096).equals(Buffer.from(expectedSums)), 'wrong SHA256SUMS');
  const modulePath = 'github.com/reeezark/pi-learnloop';
  const dependencies = new Map([...fs.readFileSync(path.join(root, 'go.mod'), 'utf8')
    .matchAll(/^\s+([^\s]+) (v[^\s]+)(?: \/\/ indirect)?$/gm)].map(m => [m[1], m[2]]));
  const sums = new Set(fs.readFileSync(path.join(root, 'go.sum'), 'utf8').trim().split('\n'));
  let provenance;
  for (const [index, arch] of ['arm64', 'amd64'].entries()) {
    const archive = path.join(scratch, names[index]);
    requireValue(command('/usr/bin/unzip', ['-Z', '-1', archive]) === 'pi-learnloop\nLICENSE\nREADME.md\n', 'wrong ZIP entry set or order');
    const listing = command('/usr/bin/unzip', ['-Z', '-l', archive]).split('\n')
      .filter(line => /^[-dl]/.test(line)).map(line => line.trim().split(/\s+/));
    requireValue(listing.length === 3 && listing.every((row, i) =>
      row[0] === (i === 0 ? '-rwxr-xr-x' : '-rw-r--r--') &&
      /^[0-9]+$/.test(row[3]) && Number(row[3]) <= (i === 0 ? maxBytes : 16384)), 'invalid ZIP permissions or sizes');
    command('/usr/bin/unzip', ['-t', archive]);
    const binary = path.join(scratch, arch);
    // Explicit stream destinations avoid trusting archive paths during extraction.
    fs.writeFileSync(binary, command('/usr/bin/unzip', ['-p', archive, 'pi-learnloop'], true), {mode: 0o600});
    requireValue(command('/usr/bin/unzip', ['-p', archive, 'LICENSE'], true)
      .equals(fs.readFileSync(path.join(root, 'LICENSE'))), 'wrong license');
    const readme = command('/usr/bin/unzip', ['-p', archive, 'README.md']);
    const expectedReadme = `# Pi LearnLoop ${version} — darwin/${arch}\n\n` +
      'Unsigned local verification candidate; not a stable release.\n' +
      'Requires macOS 13+, Git, Node >=22.19.0, and Pi 0.84.3.\n' +
      'Do not bypass Gatekeeper or quarantine checks.\n' +
      'Stable installation requires separately verified Developer ID signing and notarization.\n' +
      'The npm Pi extension is separate; install the same product version.\n' +
      'Place a verified daemon in a user-selected PATH directory, then run\n' +
      '`pi-learnloop version` and manually start `pi-learnloop daemon` in the foreground.\n' +
      'No automatic startup, update, or history deletion is performed.\n';
    requireValue(readme === expectedReadme, 'wrong candidate README');
    diagnostic(binary, arch);
    const info = JSON.parse(command('go', ['version', '-m', '-json', binary]));
    requireValue(info.GoVersion === 'go1.27.1' && info.Path === `${modulePath}/cmd/pi-learnloop` &&
      info.Main.Path === modulePath && !info.Main.Replace && typeof info.Main.Version === 'string', 'invalid Go build identity');
    requireValue(Array.isArray(info.Deps) && info.Deps.length === dependencies.size &&
      new Set(info.Deps.map(dep => dep.Path)).size === dependencies.size &&
      info.Deps.every(dep => dependencies.get(dep.Path) === dep.Version && !dep.Replace &&
        sums.has(`${dep.Path} ${dep.Version} ${dep.Sum}`)), 'dependency build metadata differs from committed manifests');
    const settings = Object.fromEntries(info.Settings.map(s => [s.Key, s.Value]));
    const fixed = {'-buildmode': 'exe', '-compiler': 'gc', '-trimpath': 'true',
      CGO_ENABLED: '0', GOOS: 'darwin', GOARCH: arch, vcs: 'git',
      [arch === 'arm64' ? 'GOARM64' : 'GOAMD64']: arch === 'arm64' ? 'v8.0' : 'v1'};
    requireValue(Object.entries(fixed).every(([key, value]) => settings[key] === value), 'wrong Go build settings');
    const allowed = new Set([...Object.keys(fixed), 'DefaultGODEBUG', 'vcs.revision', 'vcs.time', 'vcs.modified']);
    requireValue(info.Settings.length === Object.keys(settings).length && Object.keys(settings).every(k => allowed.has(k)), 'unexpected Go build settings');
    requireValue(/^[0-9a-f]{40}$/.test(settings['vcs.revision']) &&
      /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/.test(settings['vcs.time']) &&
      Number.isFinite(Date.parse(settings['vcs.time'])) && /^(true|false)$/.test(settings['vcs.modified']), 'missing or malformed VCS metadata');
    const current = JSON.stringify([info.Main.Version, settings['vcs.revision'], settings['vcs.time'], settings['vcs.modified']]);
    requireValue(!provenance || provenance === current, 'mixed source provenance');
    provenance = current;
    console.log(`Verified darwin/${arch} ${version}; revision=${settings['vcs.revision']}; modified=${settings['vcs.modified']}`);
  }
  console.log('Unsigned candidate integrity verified; clean signed-tag and publication gates are separate.');
} catch (error) {
  console.error(`release verification: ${error.message}`);
  process.exitCode = 1;
} finally {
  if (scratch) fs.rmSync(scratch, {recursive: true, force: true});
}
JS
