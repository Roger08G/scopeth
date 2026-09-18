.PHONY: build test lint validate clean

build:
	go build -trimpath -o bin/scopeth-engine ./cmd/scopeth-engine
	uv sync --project python --all-groups --locked

test:
	go test ./...
	uv run --project python pytest python/tests

lint:
	go vet ./...
	uv run --project python ruff check python
	uv run --project python ruff format --check python
	uv run --project python mypy --config-file python/pyproject.toml python/src python/tests

validate: build lint test

clean:
	go clean
