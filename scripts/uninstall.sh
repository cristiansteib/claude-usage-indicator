#!/usr/bin/env bash
set -euo pipefail
UUID="claude-usage@local"
pkill -x claude-usage-indicator 2>/dev/null || true
command -v gnome-extensions >/dev/null && gnome-extensions disable "$UUID" 2>/dev/null || true
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/gnome-shell/extensions/$UUID"
rm -f "${XDG_BIN_HOME:-$HOME/.local/bin}/claude-usage-indicator" \
      "${XDG_CONFIG_HOME:-$HOME/.config}/autostart/claude-usage-indicator.desktop" \
      "${XDG_RUNTIME_DIR:-/tmp}/claude-usage.json"
rm -rf "${XDG_CACHE_HOME:-$HOME/.cache}/claude-usage-indicator"
if command -v gsettings >/dev/null; then
  cur="$(gsettings get org.gnome.shell enabled-extensions)"
  new="$(printf '%s' "$cur" | sed -E "s/, '$UUID'//; s/'$UUID', //; s/'$UUID'//")"
  [[ "$new" == "[]" ]] && new="@as []"
  gsettings set org.gnome.shell enabled-extensions "$new"
fi
echo "Uninstalled."
