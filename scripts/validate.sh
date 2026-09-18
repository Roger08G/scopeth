#!/usr/bin/env sh
set -eu

if [ "$#" -ne 1 ]; then
  printf '%s\n' "Uso: ./scripts/validate.sh /ruta/confiable/solc" >&2
  exit 2
fi

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SOLC=$1
TMP_RESULT=$(mktemp -d)
trap 'rm -rf "$TMP_RESULT"' EXIT INT TERM

cd "$ROOT"
./scripts/build.sh
go test ./...
SCOPETH_TEST_SOLC="$SOLC" go test ./internal/ast -run TestRealSolc -count=1
go vet ./...
uv run --project python ruff check python
uv run --project python ruff format --check python
uv run --project python mypy --config-file python/pyproject.toml python/src python/tests
uv run --project python pytest python/tests

printf '%s' "{\"protocol_version\":\"1.0\",\"request_id\":\"validation\",\"operation\":\"analyze\",\"input\":\"$ROOT/fixtures/solidity\",\"options\":{\"mode\":\"semantic\",\"profile\":\"deep\",\"output\":\"$TMP_RESULT\",\"solc\":\"$SOLC\",\"timeout_seconds\":120}}" \
  | "$ROOT/bin/scopeth-engine" \
  | grep '"type":"result"' >/dev/null

response=$(printf '%s' "{\"protocol_version\":\"1.0\",\"request_id\":\"validation-f4\",\"operation\":\"analyze\",\"input\":\"$ROOT/fixtures/f4\",\"options\":{\"mode\":\"semantic\",\"profile\":\"deep\",\"output\":\"$TMP_RESULT\",\"solc\":\"$SOLC\",\"timeout_seconds\":120}}" | "$ROOT/bin/scopeth-engine")
printf '%s\n' "$response" | grep '"type":"result"' >/dev/null
run_dir=$(find "$TMP_RESULT" -mindepth 1 -maxdepth 1 -type d ! -name '.scopeth' | sort | tail -n 1)
test -s "$run_dir/report.md"
test -f "$run_dir/configuration-changes.jsonl"
test -f "$run_dir/assembly-operations.jsonl"

printf '%s\n' "Validación completa."
