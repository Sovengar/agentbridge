#!/bin/sh
# Exercise mux without a real multiplexer: fake backends record argv.
set -eu

DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
MUX="$DIR/mux"
TMP=$(mktemp -d)
LOG="$TMP/log"
FAKE="$TMP/bin"
mkdir -p "$FAKE"
export LOG

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}

contains() {
	grep -Fq -- "$1" "$LOG" || fail "missing in backend log: $1"
}

reset() {
	: >"$LOG"
}

cat >"$FAKE/tmux" <<'EOF'
#!/bin/sh
printf 'tmux %s\n' "$*" >>"$LOG"
EOF

cat >"$FAKE/herdr" <<'EOF'
#!/bin/sh
printf 'herdr %s\n' "$*" >>"$LOG"
case "$1 $2" in
"pane split") printf '{"result":{"pane":{"pane_id":"p1"}}}\n' ;;
"tab create") printf '{"result":{"root_pane":{"pane_id":"p2"}}}\n' ;;
*) : ;;
esac
EOF

cat >"$FAKE/tuios" <<'EOF'
#!/bin/sh
printf 'tuios %s\n' "$*" >>"$LOG"
case "$1" in
split-window) printf '{"window_id":"w1"}\n' ;;
*) : ;;
esac
EOF

cat >"$FAKE/kitty" <<'EOF'
#!/bin/sh
printf 'kitty %s\n' "$*" >>"$LOG"
EOF

cat >"$FAKE/wezterm" <<'EOF'
#!/bin/sh
printf 'wezterm %s\n' "$*" >>"$LOG"
EOF

chmod +x "$FAKE/tmux" "$FAKE/herdr" "$FAKE/tuios" "$FAKE/kitty" "$FAKE/wezterm"
PATH="$FAKE:$PATH"
export PATH

unset HERDR_ENV TUIOS_ENV KITTY_LISTEN_ON WEZTERM_PANE TMUX

out=$(HERDR_ENV=1 "$MUX" --dry-run -- pi -p x)
printf '%s' "$out" | grep -Fq 'backend=herdr' || fail "HERDR_ENV detection"

out=$(TUIOS_ENV=1 "$MUX" --dry-run -- pi -p x)
printf '%s' "$out" | grep -Fq 'backend=tuios' || fail "TUIOS_ENV detection"

out=$(KITTY_LISTEN_ON=1 "$MUX" --dry-run -- pi -p x)
printf '%s' "$out" | grep -Fq 'backend=kitty' || fail "KITTY detection"

out=$(WEZTERM_PANE=1 "$MUX" --dry-run -- pi -p x)
printf '%s' "$out" | grep -Fq 'backend=wezterm' || fail "WEZTERM detection"

out=$(TMUX=1 "$MUX" --dry-run -- pi -p x)
printf '%s' "$out" | grep -Fq 'backend=tmux' || fail "TMUX detection"

out=$("$MUX" --dry-run -- pi -p x)
printf '%s' "$out" | grep -Fq 'backend=none' || fail "none detection"

out=$("$MUX" --dry-run --layout tab --session-id projects-92 -- pi -p x)
printf '%s' "$out" | grep -Fq 'label=projects-92' || fail "session-id label"

out=$("$MUX" --dry-run --layout tab --label job --session-id projects-92 -- pi -p x)
printf '%s' "$out" | grep -Fq 'label=job' || fail "explicit label wins"

reset
TMUX=1 "$MUX" --backend tmux --layout pane --direction down --working-dir /tmp -- pi -p 'do it'
contains 'tmux split-window -v'
contains "'do it'"

reset
TMUX=1 "$MUX" --backend tmux --layout pane --working-dir /tmp -- pi -p 'do it'
contains 'tmux split-window -h'
contains ' -c /tmp'

reset
TMUX=1 "$MUX" --backend tmux --layout popup --width 70% -- pi -p 'do it'
contains 'tmux display-popup -E'

reset
HERDR_ENV=1 "$MUX" --backend herdr --layout pane -- pi -p 'do it'
contains 'herdr pane split'
contains 'herdr pane run p1'

reset
HERDR_ENV=1 "$MUX" --backend herdr --layout tab --session-id projects-92 -- pi -p 'do it'
contains 'herdr tab create'
contains '--label projects-92'

reset
HERDR_ENV=1 "$MUX" --backend herdr --layout popup -- pi -p 'do it'
contains 'herdr pane split'
contains 'herdr pane zoom p1 --on'
contains 'herdr pane run p1'

reset
TUIOS_ENV=1 TUIOS_SESSION=work "$MUX" --backend tuios --layout pane -- pi -p 'do it'
contains 'tuios split-window vertical -s work --json'
contains 'tuios send-text -s work -w w1'

reset
TUIOS_ENV=1 TUIOS_SESSION=work "$MUX" --backend tuios --layout popup -- pi -p 'do it'
contains 'tuios popup -s work'
contains 'sh -c'

reset
TUIOS_ENV=1 TUIOS_SESSION=work "$MUX" --backend tuios --layout tab -- pi -p 'do it'
contains 'tuios new-window'

reset
KITTY_LISTEN_ON=1 "$MUX" --backend kitty --layout pane --direction down --working-dir /tmp -- pi -p 'do it'
contains 'kitty @ launch --type=window --location=hsplit'
contains "--cwd /tmp"

reset
KITTY_LISTEN_ON=1 "$MUX" --backend kitty --layout tab --label job -- pi -p 'do it'
contains 'kitty @ launch --type=tab --tab-title job'

reset
KITTY_LISTEN_ON=1 "$MUX" --backend kitty --layout popup -- pi -p 'do it'
contains 'kitty @ launch --type=overlay'

reset
WEZTERM_PANE=1 "$MUX" --backend wezterm --layout pane --direction down --working-dir /tmp -- pi -p 'do it'
contains 'wezterm cli split-pane --bottom'
contains "--cwd /tmp"

reset
WEZTERM_PANE=1 "$MUX" --backend wezterm --layout popup -- pi -p 'do it'
contains 'wezterm cli split-pane --right'

out=$("$MUX" --backend none -- sh -c 'printf ok')
[ "$out" = "ok" ] || fail "none backend should exec the command"

printf 'mux tests: ok\n'
