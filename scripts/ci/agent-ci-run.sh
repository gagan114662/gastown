#!/usr/bin/env bash
# agent-ci-run.sh — Run a single CI job locally in Docker via @redwoodjs/agent-ci
#
# Usage:
#   ./scripts/ci/agent-ci-run.sh lint         # run lint job only
#   ./scripts/ci/agent-ci-run.sh test         # run test + coverage job only
#   ./scripts/ci/agent-ci-run.sh vuln         # run vulnerability scan only
#   ./scripts/ci/agent-ci-run.sh integration  # run integration tests only
#   ./scripts/ci/agent-ci-run.sh              # run full ci.yml (all jobs, heavy)
#
# On failure: container pauses with full state. Copy the runner name from output:
#   npx @redwoodjs/agent-ci@latest retry --name <runner-name>   # resume from failed step
#   npx @redwoodjs/agent-ci@latest abort --name <runner-name>   # kill the container
#
# Requirements: Docker running, Node.js >= 22
# Secrets: cp .env.agent-ci.example .env.agent-ci (one-time)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

export AI_AGENT=1

JOB="${1:-}"
shift 2>/dev/null || true

case "$JOB" in
  lint)        WORKFLOW=".github/workflows/agent-ci-lint.yml" ;;
  test)        WORKFLOW=".github/workflows/agent-ci-test.yml" ;;
  vuln)        WORKFLOW=".github/workflows/agent-ci-vuln.yml" ;;
  integration) WORKFLOW=".github/workflows/ci.yml" ;;  # reuse full workflow, only integration job runs via needs
  "")          WORKFLOW=".github/workflows/ci.yml" ;;
  *)
    echo "Unknown job: $JOB"
    echo "Usage: $0 [lint|test|vuln|integration]"
    exit 1
    ;;
esac

echo "[agent-ci] Running workflow: $WORKFLOW"
exec npx @redwoodjs/agent-ci@latest run \
  --workflow "$WORKFLOW" \
  --pause-on-failure \
  "$@"
