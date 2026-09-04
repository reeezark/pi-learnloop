#!/bin/sh

set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/pi-learnloop-release-test.XXXXXX")
cleanup() {
  case "$test_root" in
    "${TMPDIR:-/tmp}"/pi-learnloop-release-test.*) rm -rf -- "$test_root" ;;
  esac
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

version=$(node -p 'JSON.parse(require("fs").readFileSync(process.argv[1], "utf8")).version' "$script_dir/../package.json")
"$script_dir/build-release-artifacts.sh" "$version" "$test_root/first"
node - "$test_root/first" "$version" <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const [dir, version] = process.argv.slice(2);
assert.deepEqual(fs.readdirSync(dir).sort(), [
  'SHA256SUMS', `pi-learnloop_${version}_darwin_amd64.zip`,
  `pi-learnloop_${version}_darwin_arm64.zip`,
].sort());
JS
echo 'ok - exact artifact set'
"$script_dir/verify-release-artifacts.sh" "$version" "$test_root/first"
echo 'ok - independently verified artifacts'
node - "$script_dir" "$test_root" "$version" <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {spawn} = require('node:child_process');
const [scripts, root, version] = process.argv.slice(2);
(async () => {
  const fake = path.join(root, 'signal-tools');
  fs.mkdirSync(fake);
  const marker = path.join(root, 'building');
  fs.writeFileSync(path.join(fake, 'go'), `#!/bin/sh\nif [ "$1" = version ]; then\n  echo 'go version go1.27.1 darwin/arm64'\n  exit 0\nfi\ntouch '${marker}'\nexec sleep 20\n`, {mode: 0o755});
  const child = spawn(path.join(scripts, 'build-release-artifacts.sh'), [version, path.join(root, 'interrupted')],
    {env: {...process.env, PATH: fake + ':' + process.env.PATH}, stdio: 'ignore'});
  const closed = new Promise(resolve => child.once('close', code => resolve(code)));
  for (let i = 0; i < 100 && !fs.existsSync(marker); i++) await new Promise(resolve => setTimeout(resolve, 50));
  assert.ok(fs.existsSync(marker), 'build did not start');
  child.kill('SIGTERM');
  assert.notEqual(await closed, 0);
  assert.ok(!fs.existsSync(path.join(root, 'interrupted')), 'interrupted build published output');
  assert.ok(!fs.readdirSync(root).some(name => name.startsWith('.pi-learnloop-release-')), 'interrupted staging was retained');
  console.log('ok - interrupted build cleans staging and publishes nothing');
})().catch(error => { console.error(error); process.exitCode = 1; });
JS
node - "$script_dir" "$test_root" "$version" <<'JS'
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const {spawnSync} = require('node:child_process');
const [scripts, root, version] = process.argv.slice(2);
const build = path.join(scripts, 'build-release-artifacts.sh');
const verify = path.join(scripts, 'verify-release-artifacts.sh');
const first = path.join(root, 'first');
const names = ['arm64', 'amd64'].map(arch => `pi-learnloop_${version}_darwin_${arch}.zip`);
function run(file, args, options = {}) {
  return spawnSync(file, args, {encoding: 'utf8', timeout: 600000, maxBuffer: 128 * 1024 * 1024, ...options});
}
function pass(file, args, options) {
  const result = run(file, args, options);
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0, `${file}: ${result.stdout}\n${result.stderr}`);
  return result.stdout;
}
function reject(file, args, message, options) {
  const result = run(file, args, options);
  assert.equal(result.error, undefined);
  assert.notEqual(result.status, 0, `${message} unexpectedly passed`);
  assert.match(result.stderr, /release (build|verification):/);
  console.log(`ok - ${message}`);
}
function hash(bytes) { return crypto.createHash('sha256').update(bytes).digest('hex'); }
function binary(dir, arch) {
  return pass('/usr/bin/unzip', ['-p', path.join(dir, `pi-learnloop_${version}_darwin_${arch}.zip`), 'pi-learnloop'], {encoding: null});
}
function checksums(dir) {
  fs.writeFileSync(path.join(dir, 'SHA256SUMS'), names.map(name => `${hash(fs.readFileSync(path.join(dir, name)))}  ${name}\n`).join(''));
}
let fixtureIndex = 0;
function fixture() {
  const dir = path.join(root, `fixture-${fixtureIndex++}`);
  fs.cpSync(first, dir, {recursive: true});
  return dir;
}
function alteredArchive(label, mutate) {
  const dir = fixture(), work = path.join(root, `contents-${fixtureIndex}`);
  fs.mkdirSync(work);
  pass('/usr/bin/unzip', ['-q', path.join(dir, names[0]), '-d', work]);
  mutate(work);
  fs.unlinkSync(path.join(dir, names[0]));
  pass('/usr/bin/zip', ['-X', '-q', path.join(dir, names[0]), 'pi-learnloop', 'LICENSE', 'README.md'], {cwd: work});
  checksums(dir);
  reject(verify, [version, dir], label);
}
const second = path.join(root, 'second');
fs.mkdirSync(second); // Empty existing directories are allowed.
pass(build, [version, second]);
pass(verify, [version, second]);
for (const arch of ['arm64', 'amd64']) {
  assert.equal(hash(binary(first, arch)), hash(binary(second, arch)), `${arch} unsigned bytes differ`);
  console.log(`ok - repeat-built darwin/${arch} SHA-256 ${hash(binary(first, arch))}`);
}
const nativeArch = process.arch === 'arm64' ? 'arm64' : 'amd64';
const native = path.join(root, 'native');
fs.writeFileSync(native, binary(first, nativeArch), {mode: 0o755});
let result = run(native, ['version']);
assert.equal(result.status, 0);
assert.equal(result.stdout, `pi-learnloop ${version}\n`);
assert.equal(result.stderr, '');
result = run(native, ['version', 'extra']);
assert.equal(result.status, 2);
assert.equal(result.stdout, '');
assert.equal(result.stderr, 'usage: pi-learnloop daemon | version\n');
console.log(`ok - native ${nativeArch} diagnostic and closed invocation; other architecture not executed`);
for (const invalid of ['', 'v' + version, '01.0.0', '0.1', '0.1.0-01', version + '\n', '999.0.0', '-X main.version=x']) {
  reject(build, [invalid, path.join(root, 'invalid-version')], 'invalid or mismatched build version');
  reject(verify, [invalid, first], 'invalid or mismatched verification version');
}
reject(build, [version], 'missing output');
reject(build, [version, second, 'extra'], 'extra build argument');
reject(verify, [version, first, 'extra'], 'extra verification argument');
reject(build, [version, first], 'nonempty output refusal');
assert.ok(fs.existsSync(path.join(first, 'SHA256SUMS')));
const link = path.join(root, 'link');
fs.symlinkSync(first, link);
reject(build, [version, link], 'symlink output refusal');
reject(verify, [version, link], 'symlink artifact-directory refusal');
reject(build, [version, path.join(first, 'SHA256SUMS')], 'file output refusal');
reject(build, [version, path.join(root, 'missing-parent', 'out')], 'missing output parent refusal');
reject(build, [version, path.join(scripts, '..')], 'repository-root output refusal');
const fake = path.join(root, 'fake-tools');
fs.mkdirSync(fake);
const fakeGo = path.join(fake, 'go');
fs.writeFileSync(fakeGo, "#!/bin/sh\necho 'go version go1.26.4 darwin/arm64'\n", {mode: 0o755});
const fakeEnv = {...process.env, PATH: fake + ':' + process.env.PATH};
reject(build, [version, path.join(root, 'wrong-toolchain')], 'different compiler refused', {env: fakeEnv});
fs.writeFileSync(fakeGo, `#!/usr/bin/env node\nconst fs=require('node:fs');const a=process.argv.slice(2);\nif(a[0]==='version'){console.log('go version go1.27.1 darwin/arm64');process.exit(0)}\nif(process.env.GOARCH==='amd64')process.exit(1);\nfs.copyFileSync(${JSON.stringify(native)},a[a.indexOf('-o')+1]);\n`);
reject(build, [version, path.join(root, 'partial')], 'second-architecture failure publishes nothing', {env: fakeEnv});
assert.ok(!fs.existsSync(path.join(root, 'partial')));
assert.ok(!fs.readdirSync(root).some(name => name.startsWith('.pi-learnloop-release-')));
let dir = fixture();
fs.writeFileSync(path.join(dir, 'unexpected'), 'extra');
reject(verify, [version, dir], 'extra artifact');
dir = fixture();
fs.unlinkSync(path.join(dir, names[0]));
reject(verify, [version, dir], 'missing artifact');
dir = fixture();
fs.writeFileSync(path.join(dir, 'SHA256SUMS'), 'bad\n');
reject(verify, [version, dir], 'malformed checksum manifest');
dir = fixture();
fs.appendFileSync(path.join(dir, names[0]), 'corrupt');
reject(verify, [version, dir], 'changed archive checksum');
dir = fixture();
fs.unlinkSync(path.join(dir, names[0]));
fs.symlinkSync(path.join(first, names[0]), path.join(dir, names[0]));
reject(verify, [version, dir], 'symlink archive');
dir = fixture();
const extraWork = path.join(root, 'extra-entry');
fs.mkdirSync(extraWork);
fs.writeFileSync(path.join(extraWork, 'extra'), 'extra');
pass('/usr/bin/zip', ['-q', path.join(dir, names[0]), 'extra'], {cwd: extraWork});
checksums(dir);
reject(verify, [version, dir], 'extra ZIP entry despite matching checksum');
alteredArchive('non-executable ZIP mode', work => fs.chmodSync(path.join(work, 'pi-learnloop'), 0o644));
alteredArchive('wrong architecture despite matching checksum', work => fs.writeFileSync(path.join(work, 'pi-learnloop'), binary(first, 'amd64')));
alteredArchive('incorrect license', work => fs.writeFileSync(path.join(work, 'LICENSE'), 'wrong'));
alteredArchive('incorrect README', work => fs.writeFileSync(path.join(work, 'README.md'), 'wrong'));
alteredArchive('unexpected README instructions', work => fs.appendFileSync(path.join(work, 'README.md'), 'Run an unrelated installer.\n'));
for (const [label, pattern, replace] of [
  ['malformed VCS revision', /vcs\.revision=[0-9a-f]{40}/, value => 'vcs.revision=' + 'z'.repeat(40)],
  ['mixed VCS revisions', /vcs\.revision=[0-9a-f]{40}/, value => 'vcs.revision=' + (value.endsWith('0') ? '1' : '0').repeat(40)],
  ['malformed VCS modified state', /vcs\.modified=(true|false)/, value => 'vcs.modified=' + 'x'.repeat(value.length - 13)],
  ['wrong dependency metadata', /dep\tmodernc\.org\/sqlite\tv1\.35\.0/, value => value.replace('1.35.0', '1.35.1')],
]) alteredArchive(label, work => {
  const file = path.join(work, 'pi-learnloop');
  const bytes = fs.readFileSync(file);
  // Go may keep both runtime.modinfo and the inspectable build-info copy.
  const matches = [...bytes.toString('latin1').matchAll(new RegExp(pattern.source, 'g'))];
  assert.ok(matches.length > 0, `${label} fixture not found`);
  for (const match of matches) {
    const replacement = Buffer.from(replace(match[0]));
    assert.equal(replacement.length, match[0].length);
    replacement.copy(bytes, match.index);
  }
  fs.writeFileSync(file, bytes);
});
const buildEnv = {...process.env, GOENV: 'off', GOTOOLCHAIN: 'local', GOWORK: 'off', GOFLAGS: '',
  GOEXPERIMENT: '', CGO_ENABLED: '0', GOOS: 'darwin', GOARCH: 'arm64', GOARM64: 'v8.0'};
delete buildEnv.GOROOT;
for (const [label, flags] of [
  ['wrong embedded version', ['-trimpath', '-buildvcs=true', '-ldflags=-X main.version=9.9.9']],
  ['absent VCS metadata', ['-trimpath', '-buildvcs=false', `-ldflags=-X main.version=${version}`]],
  ['absent trimpath metadata', ['-buildvcs=true', `-ldflags=-X main.version=${version}`]],
]) alteredArchive(label, work => pass('go', ['build', '-mod=readonly', ...flags, '-o', path.join(work, 'pi-learnloop'), './cmd/pi-learnloop'],
  {cwd: path.join(scripts, '..'), env: buildEnv}));
console.log('Release artifact self-tests passed.');
JS
