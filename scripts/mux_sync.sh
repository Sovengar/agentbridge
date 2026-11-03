#!/bin/sh
# mux_sync.sh — the duplicated `mux` must not drift.
#
# The script lives twice: canonically in chezmoi
# (`home/dot_local/bin/executable_mux`, which installs ~/.local/bin/mux) and as a
# reference copy in this repo, next to the tests that exercise it. Two copies are
# fine; two DIFFERENT copies are not, because the handoff templates call whichever
# one is on PATH.
#
#   sh scripts/mux_sync.sh          # compare with ~/.local/bin/mux
#   MUX_PATH=/usr/local/bin/mux sh scripts/mux_sync.sh
#
# Exit 0: identical (or no installed mux to compare). Exit 1: they differ, with a
# diff and the side to copy from.
set -eu

DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO="$DIR/mux"
INSTALLED=${MUX_PATH:-$HOME/.local/bin/mux}

if [ ! -f "$REPO" ]; then
	printf 'FAIL: %s is missing\n' "$REPO" >&2
	exit 1
fi

if [ ! -f "$INSTALLED" ]; then
	printf 'skip: no installed mux at %s (set MUX_PATH to compare another copy)\n' "$INSTALLED"
	exit 0
fi

if cmp -s "$REPO" "$INSTALLED"; then
	printf 'mux sync: ok (%s == %s)\n' "$REPO" "$INSTALLED"
	exit 0
fi

printf 'FAIL: the two mux copies differ\n\n' >&2
diff -u "$INSTALLED" "$REPO" >&2 || true
printf '\nchezmoi owns %s; copy the winner over the loser, then re-run this check.\n' "$INSTALLED" >&2
exit 1
