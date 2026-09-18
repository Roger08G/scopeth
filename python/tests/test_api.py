from __future__ import annotations

from pathlib import Path

import pytest

from scopeth.api import AnalyzeOptions, _ensure_compatibility, _pagination
from scopeth.errors import ErrorCode, ScopethError


def test_options_payload(tmp_path: Path) -> None:
    payload = AnalyzeOptions(
        mode="semantic",
        profile="deep",
        output=tmp_path / "analysis",
        jobs=2,
        remappings=("@openzeppelin/=lib/openzeppelin-contracts/",),
        cpu_profile=tmp_path / "cpu.prof",
        heap_profile=tmp_path / "heap.prof",
    ).payload()
    assert payload["mode"] == "semantic"
    assert payload["jobs"] == 2
    assert payload["remappings"] == ["@openzeppelin/=lib/openzeppelin-contracts/"]
    assert payload["cpu_profile"] == str((tmp_path / "cpu.prof").absolute())
    assert payload["heap_profile"] == str((tmp_path / "heap.prof").absolute())


def test_options_reject_unknown_mode() -> None:
    with pytest.raises(ScopethError) as captured:
        AnalyzeOptions(mode="magic").payload()
    assert captured.value.code == ErrorCode.INPUT_INVALID


@pytest.mark.parametrize(("offset", "limit"), [(-1, 10), (0, 0), (0, 10_001)])
def test_pagination_rejects_invalid_values(offset: int, limit: int) -> None:
    with pytest.raises(ScopethError) as captured:
        _pagination(offset, limit)
    assert captured.value.code == ErrorCode.INPUT_INVALID


def test_version_compatibility() -> None:
    _ensure_compatibility({"engine_version": "1.2.3", "result_schema_version": "1.0"})
    with pytest.raises(ScopethError) as captured:
        _ensure_compatibility({"engine_version": "2.0.0", "result_schema_version": "1.0"})
    assert captured.value.code == ErrorCode.PROTOCOL_MISMATCH
