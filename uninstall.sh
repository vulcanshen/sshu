#!/bin/sh
# sshu uninstaller for macOS / Linux.
# Usage: curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/uninstall.sh | sh
#
# terminu family template (tdp project/uninstall.sh). Replace sshu; the
# blocks marked "app's own" are the app's, the rest stays as it is.

set -e

APP="sshu"
# The prefix of the app's environment variables (tdp E2): <APP>__CONFIG, …
APP_UPPER=$(echo "$APP" | tr '[:lower:]' '[:upper:]')

CANDIDATES="$HOME/.local/bin/$APP /usr/local/bin/$APP"

FOUND=""
for path in $CANDIDATES; do
  if [ -f "$path" ]; then
    FOUND="$path"
    break
  fi
done

if [ -z "$FOUND" ]; then
  echo "$APP not found in expected locations."
  echo "Checked: $CANDIDATES"
  exit 1
fi

# --- app's own: what to undo while the binary is still here (none by default) ---
# --- end of the app's own ---

rm "$FOUND"
echo "removed $FOUND"

# ask offers to remove one directory: the answer is read from the terminal,
# not stdin — under `curl ... | sh` stdin is the script itself, so a plain
# `read` would consume script text (or hit EOF) instead of the keypress. No
# controlling terminal (cron, nohup) -> keep it, the safe default.
ask() {
  dir="$1"; what="$2"
  [ -d "$dir" ] || return 0
  printf "Remove %s in %s? [y/N]: " "$what" "$dir"
  if { read -r answer < /dev/tty; } 2>/dev/null; then
    case "$answer" in
      y|Y|yes|YES) rm -rf "$dir"; echo "removed $dir" ;;
      *) echo "kept $dir" ;;
    esac
  else
    echo ""
    echo "kept $dir (no terminal to confirm on)"
  fi
}

# env_of prints the value of the environment variable named $1, empty when unset.
env_of() {
  eval "printf '%s' \"\${$1:-}\""
}

# Where tdp E3 puts the settings, the data and the cache; each is asked about
# only when it is there.
CONFIG_DIR=$(env_of "${APP_UPPER}__CONFIG")
[ -n "$CONFIG_DIR" ] || CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/$APP"
DATA_DIR=$(env_of "${APP_UPPER}__DATA")
[ -n "$DATA_DIR" ] || DATA_DIR="$HOME/.$APP"
CACHE_DIR=$(env_of "${APP_UPPER}__CACHE")
if [ -z "$CACHE_DIR" ]; then
  case "$(uname -s)" in
    Darwin) CACHE_DIR="$HOME/Library/Caches/$APP" ;;
    *)      CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/$APP" ;;
  esac
fi
ask "$CONFIG_DIR" "$APP settings"
ask "$DATA_DIR" "$APP data"
ask "$CACHE_DIR" "$APP cache"

echo ""
echo "$APP uninstalled."

# --- app's own: anything left for the user to do (none by default) ---
# Before it followed tdp E3, sshu kept its settings where Go's
# os.UserConfigDir puts them; on macOS that is Library/Application Support.
# sshu still reads them there while ~/.config/sshu does not exist.
if [ "$(uname -s)" = "Darwin" ]; then
  ask "$HOME/Library/Application Support/$APP" "$APP settings in the old location"
fi
# --- end of the app's own ---
