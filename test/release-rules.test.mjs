// Verifies the release policy: almost everything is a patch; minor/major only
// for contract changes. Run: node --test test/
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { analyzeCommits } from '@semantic-release/commit-analyzer';
import { generateNotes } from '@semantic-release/release-notes-generator';

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

// Regression: the preset and the changelog writer must be compatible, otherwise the
// release job dies after analysing commits (this happened on the first release).
test('release notes can be generated', async () => {
  const notesOpts = cfg.plugins.find(p => Array.isArray(p) && p[0] === '@semantic-release/release-notes-generator')[1];
  const notes = await generateNotes(notesOpts, {
    cwd: process.cwd(),
    options: { repositoryUrl: 'https://github.com/cristiansteib/claude-usage-indicator' },
    lastRelease: { gitTag: 'v1.0.0' },
    nextRelease: { version: '1.0.1', gitTag: 'v1.0.1' },
    commits: [{ hash: 'abcdef1234', message: 'fix: handle 429 backoff', committerDate: new Date().toISOString() }],
    logger: { log() {} },
  });
  assert.match(notes, /handle 429 backoff/);
});
