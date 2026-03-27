#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

# Check OrbStack is running (it provides the Docker socket Agent CI needs)
if ! orb status 2>/dev/null | grep -q "Running"; then
  echo "[agent-ci] OrbStack not running — falling back to local verify" >&2
  echo "[agent-ci] Start OrbStack to run full CI in Docker next time" >&2
  exec ./scripts/ci/verify.sh pre-merge
fi

echo "[agent-ci] Running CI checks via OrbStack (test, lint, vuln-check)..."
AI_AGENT=1 npx agent-ci run --workflow .github/workflows/ci-local.yml
