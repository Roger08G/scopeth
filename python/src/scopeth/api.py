from __future__ import annotations

from collections.abc import Iterator, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import cast

from .errors import ErrorCode, ScopethError
from .protocol import EngineClient, JSONObject, ProgressCallback

SUPPORTED_ENGINE_MAJOR = "1"
SUPPORTED_RESULT_SCHEMA = "1.0"


@dataclass(frozen=True, slots=True)
class AnalyzeOptions:
    mode: str = "auto"
    profile: str = "deep"
    output: Path = Path("analysis")
    solc: Path | None = None
    jobs: int = 1
    compiler_jobs: int = 1
    timeout_seconds: int = 120
    remappings: tuple[str, ...] = field(default_factory=tuple)
    refresh: bool = False
    cpu_profile: Path | None = None
    heap_profile: Path | None = None

    def payload(self) -> JSONObject:
        if self.mode not in {"auto", "semantic", "syntax"}:
            raise ScopethError(ErrorCode.INPUT_INVALID, f"Modo no válido: {self.mode}")
        if self.profile not in {"inventory", "deep"}:
            raise ScopethError(ErrorCode.INPUT_INVALID, f"Perfil no válido: {self.profile}")
        result: JSONObject = {
            "mode": self.mode,
            "profile": self.profile,
            "output": str(self.output.expanduser().absolute()),
            "jobs": self.jobs,
            "compiler_jobs": self.compiler_jobs,
            "timeout_seconds": self.timeout_seconds,
            "remappings": list(self.remappings),
            "refresh": self.refresh,
        }
        if self.solc is not None:
            result["solc"] = str(self.solc.expanduser().absolute())
        if self.cpu_profile is not None:
            result["cpu_profile"] = str(self.cpu_profile.expanduser().absolute())
        if self.heap_profile is not None:
            result["heap_profile"] = str(self.heap_profile.expanduser().absolute())
        return result


@dataclass(frozen=True, slots=True)
class AnalysisResult:
    run_id: str
    run_dir: Path
    status: str
    report: Path
    mode_used: str
    solc_version: str
    solc_sha256: str
    cache_reused: bool
    engine_version: str
    result_schema_version: str
    summary: JSONObject

    @classmethod
    def from_payload(cls, payload: JSONObject) -> AnalysisResult:
        _ensure_compatibility(payload)
        summary_value = payload.get("summary")
        summary = cast(JSONObject, summary_value) if isinstance(summary_value, dict) else {}
        return cls(
            run_id=str(payload["run_id"]),
            run_dir=Path(str(payload["run_dir"])),
            status=str(payload["status"]),
            report=Path(str(payload["report"])),
            mode_used=str(payload["mode_used"]),
            solc_version=str(payload["solc_version"]),
            solc_sha256=str(payload.get("solc_sha256", "")),
            cache_reused=bool(payload.get("cache_reused", False)),
            engine_version=str(payload["engine_version"]),
            result_schema_version=str(payload["result_schema_version"]),
            summary=summary,
        )


class Scopeth:
    """API reutilizable sin dependencia de argparse."""

    def __init__(
        self,
        engine: str | Path | Sequence[str] | None = None,
        *,
        timeout: float = 180.0,
        progress: ProgressCallback | None = None,
    ) -> None:
        self.client = EngineClient(engine, timeout=timeout, progress=progress)

    def doctor(self, *, solc: Path | None = None) -> JSONObject:
        options: JSONObject = {}
        if solc is not None:
            options["solc"] = str(solc.expanduser().absolute())
        _, result = self.client.run("doctor", options=options)
        _ensure_compatibility(result)
        return result

    def analyze(
        self,
        source: str | Path,
        options: AnalyzeOptions | None = None,
        *,
        sourceth_manifest: bool = False,
    ) -> AnalysisResult:
        selected = options or AnalyzeOptions()
        payload = selected.payload()
        if sourceth_manifest:
            payload["input_kind"] = "sourceth_manifest"
        _, result = self.client.run("analyze", input_path=source, options=payload)
        return AnalysisResult.from_payload(result)

    def functions(
        self,
        run_dir: str | Path,
        *,
        visibility: str | None = None,
        payable: bool | None = None,
        mutability: str | None = None,
        modifier: str | None = None,
        offset: int = 0,
        limit: int = 100,
    ) -> Iterator[JSONObject]:
        options = _pagination(offset, limit)
        if visibility is not None:
            options["visibility"] = visibility
        if payable is not None:
            options["payable"] = payable
        if mutability is not None:
            options["mutability"] = mutability
        if modifier is not None:
            options["modifier"] = modifier
        yield from self._records("query_functions", run_dir, options)

    def flows(
        self,
        run_dir: str | Path,
        *,
        asset_class: str | None = None,
        category: str | None = None,
        operation: str | None = None,
        reachable_from: str | None = None,
        offset: int = 0,
        limit: int = 100,
    ) -> Iterator[JSONObject]:
        options = _pagination(offset, limit)
        if asset_class is not None:
            options["asset_class"] = asset_class
        if category is not None:
            options["category"] = category
        if operation is not None:
            options["operation"] = operation
        if reachable_from is not None:
            options["reachable_from"] = reachable_from
        yield from self._records("query_flows", run_dir, options)

    def state(
        self,
        run_dir: str | Path,
        *,
        written_by: str | None = None,
        variable: str | None = None,
        access: str | None = None,
        offset: int = 0,
        limit: int = 100,
    ) -> Iterator[JSONObject]:
        options = _pagination(offset, limit)
        if written_by is not None:
            options["written_by"] = written_by
        if variable is not None:
            options["variable"] = variable
        if access is not None:
            options["access"] = access
        yield from self._records("query_state", run_dir, options)

    def surfaces(
        self,
        run_dir: str | Path,
        *,
        category: str | None = None,
        rule: str | None = None,
        resolution: str | None = None,
        offset: int = 0,
        limit: int = 100,
    ) -> Iterator[JSONObject]:
        options = _pagination(offset, limit)
        if category is not None:
            options["category"] = category
        if rule is not None:
            options["rule"] = rule
        if resolution is not None:
            options["resolution"] = resolution
        yield from self._records("query_surfaces", run_dir, options)

    def configuration(
        self,
        run_dir: str | Path,
        *,
        category: str | None = None,
        reachable_from: str | None = None,
        offset: int = 0,
        limit: int = 100,
    ) -> Iterator[JSONObject]:
        options = _pagination(offset, limit)
        if category is not None:
            options["category"] = category
        if reachable_from is not None:
            options["reachable_from"] = reachable_from
        yield from self._records("query_configuration", run_dir, options)

    def explain(self, run_dir: str | Path, symbol: str) -> JSONObject:
        records = list(self._records("query_explain", run_dir, {"symbol": symbol}))
        if len(records) != 1:
            raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "explain no devolvió una ficha única.")
        return records[0]

    def graph(
        self,
        run_dir: str | Path,
        *,
        kind: str = "calls",
        root: str | None = None,
        depth: int = 4,
        format: str = "mermaid",
    ) -> JSONObject:
        options: JSONObject = {"kind": kind, "depth": depth, "format": format}
        if root is not None:
            options["root"] = root
        records = list(self._records("query_graph", run_dir, options))
        if len(records) != 1:
            raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "graph no devolvió un resultado único.")
        return records[0]

    def _records(
        self,
        operation: str,
        run_dir: str | Path,
        options: JSONObject,
    ) -> Iterator[JSONObject]:
        terminal_seen = False
        for message in self.client.messages(operation, run_dir=run_dir, options=options):
            if message["type"] == "record":
                record = message.get("record")
                if isinstance(record, dict):
                    yield record
                else:
                    yield {"value": record}
            elif message["type"] == "error":
                body = message.get("error")
                if not isinstance(body, dict):
                    raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "Error terminal mal formado.")
                raise ScopethError(
                    str(body.get("code", "ENGINE_FAILED")),
                    str(body.get("message", "Error del motor.")),
                )
            elif message["type"] == "result":
                terminal_seen = True
        if not terminal_seen:
            raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "Consulta sin resultado terminal.")


def _pagination(offset: int, limit: int) -> JSONObject:
    if offset < 0:
        raise ScopethError(ErrorCode.INPUT_INVALID, "offset no puede ser negativo.")
    if limit < 1 or limit > 10_000:
        raise ScopethError(ErrorCode.INPUT_INVALID, "limit debe estar entre 1 y 10000.")
    return {"offset": offset, "limit": limit}


def _ensure_compatibility(payload: JSONObject) -> None:
    engine_version = str(payload.get("engine_version", ""))
    schema_version = str(payload.get("result_schema_version", ""))
    if engine_version.split(".", maxsplit=1)[0] != SUPPORTED_ENGINE_MAJOR:
        raise ScopethError(
            ErrorCode.PROTOCOL_MISMATCH,
            f"Versión de motor incompatible: {engine_version or 'ausente'}.",
        )
    if schema_version != SUPPORTED_RESULT_SCHEMA:
        raise ScopethError(
            ErrorCode.PROTOCOL_MISMATCH,
            f"Esquema de resultados incompatible: {schema_version or 'ausente'}.",
        )
