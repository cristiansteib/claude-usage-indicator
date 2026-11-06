#!/usr/bin/env bash
# Installs the daemon and the GNOME Shell extension for the current user.
# Normally run through `mise run install` (which builds first).
set -euo pipefail

UUID="claude-usage@local"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
EXT_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/gnome-shell/extensions/$UUID"
AUTOSTART="${XDG_CONFIG_HOME:-$HOME/.config}/autostart"
RELOAD=0; [[ "${1:-}" == "--reload" ]] && RELOAD=1

say() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }

[[ -x "$ROOT/bin/claude-usage-indicator" ]] || { echo "missing bin/claude-usage-indicator: run 'mise run build' first" >&2; exit 1; }
[[ -f "$HOME/.claude/.credentials.json" ]] || echo "warning: ~/.claude/.credentials.json not found - log in with 'claude' first" >&2

say "Installing daemon to $BIN_DIR"
mkdir -p "$BIN_DIR"
install -m 0755 "$ROOT/bin/claude-usage-indicator" "$BIN_DIR/claude-usage-indicator.new"
mv -f "$BIN_DIR/claude-usage-indicator.new" "$BIN_DIR/claude-usage-indicator"

say "Installing GNOME extension to $EXT_DIR"
mkdir -p "$EXT_DIR"
install -m 0644 "$ROOT"/extension/$UUID/{extension.js,main.js,metadata.json} "$EXT_DIR/"

say "Enabling autostart"
mkdir -p "$AUTOSTART"
sed "s#@BIN@#$BIN_DIR/claude-usage-indicator#" "$ROOT/claude-usage-indicator.desktop.in" > "$AUTOSTART/claude-usage-indicator.desktop"

say "Registering the extension"
if command -v gsettings >/dev/null; then
  cur="$(gsettings get org.gnome.shell enabled-extensions)"
  if [[ "$cur" != *"$UUID"* ]]; then
    if [[ "$cur" == "@as []" || "$cur" == "[]" ]]; then new="['$UUID']"; else new="${cur%]}, '$UUID']"; fi
    gsettings set org.gnome.shell enabled-extensions "$new"
  fi
fi

say "(Re)starting the daemon"
# Match the full command line (process names are truncated to 15 chars, so
# `pkill -x` cannot match), anchored so no other shell command is hit.
pkill -f '^(/[^ ]*/|\./)?claude-usage-indicator( |$)' 2>/dev/null || true
sleep 0.7
setsid nohup "$BIN_DIR/claude-usage-indicator" >/dev/null 2>&1 &
sleep 1
if pgrep -f '^(/[^ ]*/|\./)?claude-usage-indicator( |$)' >/dev/null; then
  echo "daemon running"
else
  echo "warning: daemon did not stay up; run $BIN_DIR/claude-usage-indicator to see why" >&2
fi

if command -v gnome-extensions >/dev/null && gnome-extensions info "$UUID" >/dev/null 2>&1; then
  say "Reloading the extension"
  gnome-extensions disable "$UUID" 2>/dev/null || true
  sleep 0.5
  gnome-extensions enable "$UUID" 2>/dev/null || true
  state="$(gnome-extensions info "$UUID" | awk '/State:/{print $2}')"
  echo "extension state: $state"
  [[ "$state" == "ACTIVE" || "$state" == "ENABLED" ]] && exit 0
fi

cat <<MSG

Done. GNOME only discovers newly installed extensions at login, so
log out and back in once. The bars then appear next to the date.
(After that, 'mise run reload' applies changes without logging out.)
MSG
