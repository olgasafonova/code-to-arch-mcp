#!/usr/bin/env bash
# Smoke test: run integration tests against real codebases.
# Usage: bash scripts/smoke-test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass=0
fail=0
skip=0

log_pass() { echo -e "${GREEN}PASS${NC} $1"; ((pass++)); }
log_fail() { echo -e "${RED}FAIL${NC} $1"; ((fail++)); }
log_skip() { echo -e "${YELLOW}SKIP${NC} $1"; ((skip++)); }

echo "=== code-to-arch-mcp smoke test ==="
echo ""

# Ensure unit tests pass first
echo "--- Running unit tests ---"
if ! (cd "$ROOT" && make check); then
    echo -e "${RED}Unit tests failed. Aborting smoke test.${NC}"
    exit 1
fi
echo ""

# Run integration tests (tiers 1-4)
echo "--- Running integration tests ---"
cd "$ROOT"
if go test -tags integration -race -timeout 300s -v ./tests/ 2>&1 | tee /tmp/integration-output.txt; then
    log_pass "Integration tests"
else
    log_fail "Integration tests"
fi

echo ""
echo "=== Results ==="
echo -e "${GREEN}Passed: $pass${NC}  ${RED}Failed: $fail${NC}  ${YELLOW}Skipped: $skip${NC}"
echo ""
echo "Full output: /tmp/integration-output.txt"

if [ "$fail" -gt 0 ]; then
    exit 1
fi
