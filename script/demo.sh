#!/bin/bash
set -euo pipefail

# Terminal demo for the ettle README.
#
# One ~90s asciinema cast, three beats, on the fully-synthetic Northwind team
# (testdata/northwind/*.jsonl — four Claude Code session transcripts, no real
# data). The whole thing runs on LIVE sessions, not hand-written notes — that's
# the point: ettle reads reasoning-in-progress.
#
#   Beat 1 — the pre-meeting collision catch. Ivo is building a discount engine
#            that calls pricing in-process; Mara is extracting pricing into a
#            network service and deleting the in-process package. Nobody has
#            said this to anyone. ettle surfaces it on Ivo's horizon before the
#            standup that would otherwise have discovered it.
#   Beat 2 — bind-vs-surface. The simple collisions are just FYI'd ("worth a
#            look"); the genuine values choice — the release-freeze date three
#            people are diverging on — is routed to a crux and pre-staged as a
#            clean either/or. Friction in the right spot, not everywhere.
#   Beat 3 — the boundary, and N=1. --show-atoms prints EXACTLY what leaves each
#            machine (typed atoms, never the raw session). Then a single-person
#            run: ettle is useful at N=1 too, catching your own stale assumption.
#
# Usage:
#   ./script/demo.sh            # record the full three-beat cast
#   ./script/demo.sh collision  # just beat 1+2 (Ivo's horizon)
#   ./script/demo.sh boundary   # just beat 3a (--show-atoms)
#   ./script/demo.sh solo       # just beat 3b (N=1 self-assumption)
#
# Requirements: asciinema, Go >= 1.25, and ANTHROPIC_API_KEY in the environment
# (the run makes real model calls — ~2N+3 on Haiku, cheap).
#
# To convert to GIF: agg demo/northwind.cast demo/northwind.gif
# To embed: upload with `asciinema upload demo/northwind.cast` and paste the
# returned badge into the README (see the placeholder there).

# Resolve repo root — works whether run directly or sourced.
if [ -n "${ETTLE_DIR:-}" ]; then
    cd "$ETTLE_DIR"
elif [ -f "$(dirname "${BASH_SOURCE[0]:-$0}")/../go.mod" ]; then
    cd "$(dirname "${BASH_SOURCE[0]:-$0}")/.."
fi
ETTLE_DIR="$(pwd)"
export ETTLE_DIR

DEMO_DIR="demo"
mkdir -p "$DEMO_DIR"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "ANTHROPIC_API_KEY is not set — the demo makes live model calls." >&2
    echo "  export ANTHROPIC_API_KEY=sk-ant-...   then re-run." >&2
    exit 1
fi

# Simulate typing for natural-looking recordings.
type_cmd() {
    local cmd="$1"
    printf '$ '
    for (( i=0; i<${#cmd}; i++ )); do
        printf '%s' "${cmd:$i:1}"
        sleep 0.03
    done
    echo
    sleep 0.3
    eval "$cmd"
    sleep 1.5
}

note() { printf '\n# %s\n' "$1"; sleep 1.2; }

__run_collision() {
    cd "$ETTLE_DIR"
    sleep 0.5
    note "Four teammates, four live Claude Code sessions. No standup yet."
    note "What does Ivo's agent already know he's about to walk into?"
    type_cmd "go run ./cmd/ettle standup --me ivo testdata/northwind/*.jsonl"
    note "The pricing collision and the freeze crux — surfaced before the meeting."
    sleep 1.5
}

__run_boundary() {
    cd "$ETTLE_DIR"
    sleep 0.5
    note "The raw sessions never leave the laptop. THIS is all that crosses:"
    type_cmd "go run ./cmd/ettle standup --show-atoms testdata/northwind/*.jsonl"
    sleep 1.5
}

__run_solo() {
    cd "$ETTLE_DIR"
    sleep 0.5
    note "Useful at N=1: one person, catching their own stale assumption."
    type_cmd "go run ./cmd/ettle standup testdata/solo/dana.md"
    sleep 1.5
}

__run_all() {
    __run_collision
    __run_boundary
    __run_solo
}

# Internal dispatch for asciinema --command.
case "${1:-}" in
    __run_collision) __run_collision; exit 0 ;;
    __run_boundary)  __run_boundary;  exit 0 ;;
    __run_solo)      __run_solo;      exit 0 ;;
    __run_all)       __run_all;       exit 0 ;;
esac

record() {
    local name="$1" rows="$2" title="$3" fn="$4"
    echo "Recording $name ..."
    asciinema rec "$DEMO_DIR/$name.cast" \
        --cols 100 --rows "$rows" \
        --title "$title" \
        --env ETTLE_DIR,ANTHROPIC_API_KEY \
        --command "bash -c 'source $ETTLE_DIR/script/demo.sh $fn'" \
        --overwrite
    echo "Saved: $DEMO_DIR/$name.cast"
}

case "${1:-all}" in
    collision) record collision 40 "ettle: the pre-meeting collision catch" __run_collision ;;
    boundary)  record boundary  44 "ettle: exactly what crosses the boundary" __run_boundary ;;
    solo)      record solo       24 "ettle: useful at N=1"                     __run_solo ;;
    all)
        record northwind 48 "ettle: coordination without the meeting" __run_all
        echo
        echo "Cast saved to $DEMO_DIR/northwind.cast"
        echo "Embed:  asciinema upload $DEMO_DIR/northwind.cast   (paste the badge into README)"
        echo "Or GIF: agg $DEMO_DIR/northwind.cast $DEMO_DIR/northwind.gif"
        ;;
    *)
        echo "Usage: $0 [collision|boundary|solo|all]"
        exit 1
        ;;
esac
