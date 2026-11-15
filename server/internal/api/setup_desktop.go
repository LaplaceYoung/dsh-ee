package api

import "net/http"

const desktopAppName = "DeepSeek Harness"

func (s *Server) desktopShell(r *http.Request, lang string) string {
	pkg, _ := s.connectSetup(r)
	return `
` + setupDesktopComment(lang) + `
if [ "$(uname -s)" = "Darwin" ]; then
  DESKTOP_APP=""
  for candidate in "${DSH_DESKTOP_APP:-}" ` + shellQuote("/Applications/"+desktopAppName+".app") + ` "$HOME"` + shellQuote("/Applications/"+desktopAppName+".app") + `; do
    if [ -n "$candidate" ] && [ -x "$candidate/Contents/Resources/runtime/cli/bin/dsh" ] && [ -x "$candidate/Contents/Resources/runtime/primary-runtime/dependencies/node/bin/node" ]; then
      DESKTOP_APP="$candidate"
      break
    fi
  done
  if [ -n "$DESKTOP_APP" ]; then
    DSH="$DESKTOP_APP/Contents/Resources/runtime/cli/bin/dsh"
    DSH_NODE="$DESKTOP_APP/Contents/Resources/runtime/primary-runtime/dependencies/node/bin/node"
    if pgrep -f "$DESKTOP_APP/Contents/MacOS/" >/dev/null 2>&1; then
      msg_quit_dsh
      elapsed=0
      while pgrep -f "$DESKTOP_APP/Contents/MacOS/" >/dev/null 2>&1; do
        sleep 2
        elapsed=$((elapsed + 2))
        if [ "$elapsed" -ge 120 ]; then
          msg_dsh_running
          exit 1
        fi
      done
    fi
    DSH_DIR="${DSH_HOME:-$HOME/.dsh}"
    if [ ! -d "$DSH_DIR/profiles/desktop" ]; then
      msg_open_dsh_first
      exit 1
    fi
    PENDING_DIR="$DSH_DIR/dsh-ee"
    PENDING_FILE="$PENDING_DIR/token.pending"
    if [ "$DEVICE_LOGIN" -eq 1 ]; then
      PATH="$(dirname "$DSH"):$PATH"
      export PATH
      DSH_EE_SETUP_BASE="$BASE"
      DSH_EE_SETUP_LANG="$SETUP_LANG"
      export DSH_EE_SETUP_BASE DSH_EE_SETUP_LANG
      "$DSH_NODE" -e ` + shellQuote(setupDeviceJS(lang)) + `
      TOKEN=$(cat "$PENDING_FILE")
    fi
    if ! check_token; then
      msg_bad_token
      exit 1
    fi
    write_profile_npmrc "$DSH_DIR/profiles/desktop"
    hint_global_npmrc
    if [ "$DEVICE_LOGIN" -eq 0 ]; then
      ENV_FILE="$DSH_DIR/.env"
      mkdir -p "$DSH_DIR"
      touch "$ENV_FILE"
      chmod 600 "$ENV_FILE"
      grep -v '^` + connectTokenEnv + `=' "$ENV_FILE" > "$ENV_FILE.tmp" || true
      printf '%s=%s\n' '` + connectTokenEnv + `' "$TOKEN" >> "$ENV_FILE.tmp"
      mv "$ENV_FILE.tmp" "$ENV_FILE"
      chmod 600 "$ENV_FILE"
    else
      chmod 700 "$PENDING_DIR"
      chmod 600 "$PENDING_FILE"
      msg_saved_pending_desktop
    fi
    CONNECT_PKG=` + shellQuote(pkg) + `
    if [ -n "$CONNECT_PKG" ]; then
      "$DSH" plugin --profile desktop add "$CONNECT_PKG@latest"
      msg_installed_desktop "$CONNECT_PKG"
    else
      msg_no_connect_plugin_desktop
    fi
    msg_done_desktop
    exit 0
  fi
fi
`
}
