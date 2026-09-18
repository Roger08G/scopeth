from __future__ import annotations

import json
import sys
from pathlib import Path

import pytest

from scopeth.errors import ErrorCode, ScopethError
from scopeth.protocol import EngineClient


def _fake_engine(tmp_path: Path, *, wrong_request_id: bool = False) -> list[str]:
    script = tmp_path / "fake_engine.py"
    script.write_text(
        """
import json
import sys

request = json.loads(sys.stdin.readline())
request_id = "wrong" if WRONG else request["request_id"]
base = {"protocol_version": "1.0", "request_id": request_id}
print(json.dumps(base | {"type": "progress", "phase": "test", "message": "ok"}), flush=True)
print(json.dumps(base | {"type": "record", "record": {"name": "f"}}), flush=True)
print(json.dumps(base | {"type": "result", "result": {"status": "complete"}}), flush=True)
""".replace("WRONG", "True" if wrong_request_id else "False"),
        encoding="utf-8",
    )
    return [sys.executable, str(script)]


def test_client_consumes_ndjson(tmp_path: Path) -> None:
    progress: list[tuple[str, str]] = []
    client = EngineClient(
        _fake_engine(tmp_path), progress=lambda phase, text: progress.append((phase, text))
    )
    records, result = client.run("query_functions", run_dir=tmp_path)
    assert records == [{"name": "f"}]
    assert result == {"status": "complete"}
    assert progress == [("test", "ok")]


def test_client_rejects_wrong_request_id(tmp_path: Path) -> None:
    client = EngineClient(_fake_engine(tmp_path, wrong_request_id=True))
    with pytest.raises(ScopethError) as captured:
        client.run("doctor")
    assert captured.value.code == ErrorCode.PROTOCOL_MISMATCH


def test_public_json_is_serializable() -> None:
    payload = {"protocol_version": "1.0", "request_id": "x", "operation": "doctor"}
    assert json.loads(json.dumps(payload)) == payload
