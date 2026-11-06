# AGENTS.md

Guidance for AI coding agents (and humans) working in this repo.
`CLAUDE.md` just imports this file.

## What this is

Claude usage next to the GNOME clock. Two parts that ship together:

- **Go daemon** (repo root, `package main`): fetches usage from
  `https://api.anthropic.com/api/oauth/usage`, caches it, computes the pace ring and
  forecast, writes `$XDG_RUNTIME_DIR/claude-usage.json`, serves D-Bus
  `local.ClaudeUsage` (`Refresh`).
- **GNOME Shell extension** (`extension/claude-usage@local/`): `extension.js` is a tiny
  loader; the real code is `main.js`. It only draws and reads the JSON file.

| File | Role |
|---|---|
| `usage.go` | HTTP call, token from `~/.claude/.credentials.json`, `apiError` |
| `service.go` | Rate-limit policy (poll/cooldown/backoff), publishing, D-Bus |
| `state.go` | Persisted cache (`~/.cache/claude-usage-indicator/state.json`) + published JSON shape |
| `daily.go` | Pace ring (`computeDaily`) and probabilistic forecast (`computeForecast`) |
| `tools/previews` | Renders the README images (renders, **not** screenshots) |
| `scripts/` | `install.sh`, `uninstall.sh`, `check-commit-msg.sh` |

## Commands (everything goes through mise)

```
mise run test          # go vet + go test
mise run build         # ./bin/claude-usage-indicator
mise run install       # build + install daemon/extension/autostart
mise run reload        # re-install + hot-reload the extension
mise run status        # cached usage / ring / forecast, no network
mise run previews      # regenerate docs/*.png (needs ImageMagick)
mise run hooks         # enable the commit-msg hook in this clone
mise run lint:commits  # check commit messages since origin/main
mise run release:dry   # what semantic-release would publish
```

## Commits and releases (important)

Releases are automated by **semantic-release** on every push to `main`
(`.releaserc.json`). The policy is deliberately conservative:

> **Almost everything is a `fix:`.** A new menu line, a new ring color, a refactor
> that changes behaviour, a new forecast tweak: all `fix:` (patch release).
> Minor and major releases are reserved for changes to the **contract**.

The **contract** is what other things depend on:

1. the published JSON file `claude-usage.json` (daemon -> extension),
2. the D-Bus interface `local.ClaudeUsage`,
3. CLI flags of the daemon,
4. the on-disk cache format `state.json` when old files would break,
5. the install layout (extension UUID, paths) and the supported GNOME Shell version.

| Change | Commit | Release |
|---|---|---|
| Bug fix, tweak, new UI/behaviour, perf | `fix: ...` / `perf: ...` | patch |
| Plain `feat: ...` | **rejected by the hook/CI** (use `fix:`) | - |
| Additive contract change (new JSON field, new flag, new D-Bus method) | `feat(contract): ...` | minor |
| Breaking contract change | `<type>(contract)!: ...` + footer `BREAKING CHANGE: <what breaks, how to migrate>` | major |
| docs, tests, CI, build, chore, refactor | `docs:` `test:` `ci:` `build:` `chore:` `refactor:` | none |

Rules enforced by `scripts/check-commit-msg.sh` (local `commit-msg` hook via
`mise run hooks`, and CI on every push/PR): Conventional Commits format, subject
<= 100 chars, no plain `feat`, breaking changes only with scope `contract` and a
`BREAKING CHANGE:` footer. `.releaserc.json` also maps a stray `feat` to a patch,
so even if the check were bypassed the release stays small.
`test/release-rules.test.mjs` pins this policy.

Do not edit `CHANGELOG.md` or tags by hand; the release job does it.
Git identity for this repo: `cristiansteib@gmail.com`.

## Gotchas learned the hard way

- **GNOME caches extension modules until login.** `disable`/`enable` does *not* reload
  `extension.js`; that is why it is a loader that imports `main.js?v=<timestamp>`.
  If the extension ever throws in `enable()` it goes to `ERROR` and stays there until
  the next login, so run `node --check` on both JS files before installing.
- Give every `GObject.registerClass` a unique `GTypeName` (see `TAG` in `main.js`)
  or hot-reload collides.
- **Never hammer the usage API.** It returns 429 with `Retry-After: 0` and has been
  observed to 429 a request only 5 minutes after a success. Keep the cache,
  `pollEvery`, `manualCooldown` and backoff in `service.go`; do not add request paths
  that bypass `Service.Refresh`. Network errors (no response) retry quickly; server
  errors back off exponentially (persisted in `state.json`).
- `pkill -x claude-usage-indicator` does not work (process names are truncated to
  15 chars). Match the full command line, anchored (see `scripts/install.sh`).
- The daemon is a single instance (owns the D-Bus name), so a stale one blocks a new one.
- Stroked SVG paths/lines are dropped by ImageMagick's SVG renderer; `tools/previews`
  draws only filled shapes on purpose.
- Keep login light: autostart delay, async low-priority file reads in the extension.
- Do not print or log the OAuth token. Never commit anything from `~/.claude`.

## Before you finish a change

`mise run test`, `node --check` on `extension/claude-usage@local/*.js`, and if the
published JSON or D-Bus API changed, update both sides plus this file.
