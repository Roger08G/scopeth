#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$ROOT/bin"

cd "$ROOT"
go build -trimpath -o "$ROOT/bin/scopeth-engine" ./cmd/scopeth-engine
uv sync --project "$ROOT/python" --all-groups --locked

printf '%s\n' "Build completado: $ROOT/bin"
