#!/usr/bin/env bash
# One-shot robustness battery. Synthetic corpora only. Default model = haiku (cheap).
set -uo pipefail
cd "$(dirname "$0")"

LOG="/tmp/ettle-battery-$(date +%s).log"
echo "logfile: $LOG"

EVAL=(
  testdata/eval/standup-rename.json
  testdata/eval/auth-migration.json
  testdata/eval/independent-work.json
  testdata/eval/stale-assumption.json
  testdata/eval/scale-noise.json
  testdata/eval/shared-component-null.json
  testdata/eval/shared-deadline-null.json
  testdata/eval/schema-collision.json
  testdata/eval/duplicate-util.json
  testdata/eval/ownership-dispute.json
  testdata/eval/calendar-divergence.json
)
SUPER=(
  testdata/eval/superposition-userservice-vs-infra.json
  testdata/eval/superposition-frontend-vs-data.json
)
LEAK=(
  testdata/leak/billing-secrets.json
  testdata/leak/auth-secrets.json
  testdata/leak/private-override.json
  testdata/leak/attrition-secrets.json
)

go build -o /tmp/ettle ./cmd/ettle || exit 1

{
  echo "############ BUILD $(date -u +%FT%TZ) ############"
  echo
  echo "############ 1/4  A/B  (single-shot vs voted, pooled McNemar) ############"
  /tmp/ettle eval --ab "${EVAL[@]}"
  echo
  echo "############ 2/4  STABILITY (run-to-run Jaccard, runs=5) ############"
  /tmp/ettle eval --stability --runs 5 "${EVAL[@]}"
  echo
  echo "############ 3/4  SUPERPOSITION (locality f(A∪B)=f(A)∪f(B)) ############"
  /tmp/ettle eval --superposition "${SUPER[@]}"
  echo
  echo "############ 4/4  LEAK (privacy boundary, target leak=0 utility=1.0) ############"
  /tmp/ettle eval --leak "${LEAK[@]}"
  echo
  echo "############ DONE $(date -u +%FT%TZ) ############"
} >>"$LOG" 2>&1

echo "battery finished -> $LOG"
