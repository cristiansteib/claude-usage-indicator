// Verifies the release policy: almost everything is a patch; minor/major only
// for contract changes. Run: node --test test/
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { analyzeCommits } from '@semantic-release/commit-analyzer';

const cfg = JSON.parse(readFileSync(new URL('../.releaserc.json', import.meta.url)));
const opts = cfg.plugins.find(p => Array.isArray(p) && p[0] === '@semantic-release/commit-analyzer')[1];
const release = msg => analyzeCommits(opts, { cwd: process.cwd(), commits: [{ hash: 'abc', message: msg }], logger: { log() {} } });

const cases = [
  ['fix: handle 429 backoff',                                   'patch'],
  ['fix(extension): ring overlap color',                        'patch'],
  ['perf: read state async',                                    'patch'],
  ['feat: add a shiny new ring animation',                      'patch'], // new behaviour is still a fix
  ['feat(contract): add `forecast` block to the published JSON','minor'],
  ['fix(contract)!: rename D-Bus method\n\nBREAKING CHANGE: Refresh is now Poll', 'major'],
  ['feat(contract)!: drop -tray flag\n\nBREAKING CHANGE: flag removed',            'major'],
  ['docs: update readme',                                       null],
  ['ci: tweak workflow',                                        null],
  ['chore: bump deps',                                          null],
  ['refactor: split service.go',                                null],
  ['test: more daily cases',                                    null],
  ['build: mise task',                                          null],
];
for (const [msg, want] of cases)
  test(msg.split('\n')[0], async () => assert.equal(await release(msg), want));
