#!/usr/bin/env bash
# Enforces this repo's commit policy (see AGENTS.md "Commits and releases").
# Usage: check-commit-msg.sh <file-with-message>      (git commit-msg hook)
#        check-commit-msg.sh --range <from>..<to>      (CI)
set -uo pipefail

TYPES='fix|feat|perf|refactor|docs|test|build|ci|chore|revert'
RE="^(${TYPES})(\\(([a-z0-9._-]+)\\))?(!)?: .+"
fail=0

check() { # $1 = full message, $2 = label
  local msg="$1" label="$2" subject scope bang
  subject="$(printf '%s\n' "$msg" | head -n1)"
  case "$subject" in Merge\ *|Revert\ \"*|fixup!*|squash!*) return 0 ;; esac
  err() { printf 'commit policy: %s\n  > %s\n  %s\n' "$label" "$subject" "$1" >&2; fail=1; }

  if ! [[ "$subject" =~ $RE ]]; then
    err "Use Conventional Commits: <type>(scope)?: <subject>   types: ${TYPES//|/, }"; return
  fi
  scope="${BASH_REMATCH[3]}"; bang="${BASH_REMATCH[4]}"
  (( ${#subject} <= 100 )) || err "Subject is longer than 100 characters."

  local breaking=0
  [[ -n "$bang" ]] && breaking=1
  printf '%s\n' "$msg" | grep -qE '^BREAKING[ -]CHANGE: ' && breaking=1

  if [[ "${BASH_REMATCH[1]}" == feat && "$scope" != contract ]]; then
    err "A plain 'feat' is not allowed: new behaviour that does not change the contract is a 'fix:'. Use 'feat(contract): ...' only for additive contract changes."
  fi
  if (( breaking )) && [[ "$scope" != contract ]]; then
    err "Breaking changes (major releases) are reserved for contract changes: use '<type>(contract)!: ...' with a 'BREAKING CHANGE:' footer."
  fi
  if (( breaking )) && ! printf '%s\n' "$msg" | grep -qE '^BREAKING[ -]CHANGE: .+'; then
    err "Breaking change needs a 'BREAKING CHANGE: <what breaks and how to migrate>' footer."
  fi
}

if [[ "${1:-}" == "--range" ]]; then
  for sha in $(git rev-list --reverse "$2"); do
    check "$(git log -1 --format=%B "$sha")" "$(git log -1 --format='%h' "$sha")"
  done
else
  check "$(grep -v '^#' "$1")" "your commit message"
fi
exit $fail
