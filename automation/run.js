// Lightweight test orchestrator for pure Playwright tests (Node scripts)
// Usage:
//   node run.js                 -> run all tests under tests/**
//   node run.js tests/group     -> run all tests in a group
//   node run.js tests/group/a.js -> run a single test file

const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

function listAllTests(root) {
  const out = [];
  if (!fs.existsSync(root)) return out;
  const groups = fs.readdirSync(root, { withFileTypes: true });
  for (const g of groups) {
    if (!g.isDirectory()) continue;
    const groupDir = path.join(root, g.name);
    for (const f of fs.readdirSync(groupDir, { withFileTypes: true })) {
      if (f.isFile() && (f.name.endsWith('.js') || f.name.endsWith('.mjs'))) {
        out.push(path.join(groupDir, f.name));
      }
    }
  }
  return out.sort();
}

function listFromArg(arg) {
  const stat = fs.existsSync(arg) ? fs.statSync(arg) : null;
  if (!stat) return [];
  if (stat.isFile()) return [arg];
  if (!stat.isDirectory()) return [];
  return fs
    .readdirSync(arg, { withFileTypes: true })
    .filter((e) => e.isFile() && (e.name.endsWith('.js') || e.name.endsWith('.mjs')))
    .map((e) => path.join(arg, e.name))
    .sort();
}

function runOne(file) {
  console.log(`\n=== RUN ${file}`);
  const res = spawnSync(process.execPath, [file], { encoding: 'utf8' });
  process.stdout.write(res.stdout || '');
  process.stderr.write(res.stderr || '');
  const ok = res.status === 0;
  console.log(`--- ${ok ? 'PASS' : 'FAIL'} ${file}`);
  return ok;
}

const args = process.argv.slice(2);
let files = [];
if (args.length === 0) {
  files = listAllTests(path.join(__dirname, 'tests'));
} else {
  const target = path.resolve(process.cwd(), args[0]);
  files = listFromArg(target);
}

if (!files.length) {
  console.error('No tests found.');
  process.exit(2);
}

let passed = 0;
for (const f of files) {
  if (runOne(f)) passed++;
}

console.log(`\nSummary: ${passed}/${files.length} passed`);
process.exit(passed === files.length ? 0 : 1);

