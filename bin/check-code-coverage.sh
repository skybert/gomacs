#!/usr/bin/env bash
set -euo pipefail

COVERAGE_FILE="build/coverage.out"
THRESHOLD="${1:-90}"
AWK_SCRIPT="$(dirname "$0")/check-coverage-threshold.awk"

coverage_report() {
    go tool cover -func="$COVERAGE_FILE"
}

check_threshold() {
    coverage_report | awk -v min="$THRESHOLD" -f "$AWK_SCRIPT"
}

main() {
    check_threshold
}

main "$@"
