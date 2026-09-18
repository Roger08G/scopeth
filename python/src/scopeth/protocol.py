from __future__ import annotations

import json
import os
import queue
import shutil
import subprocess
import threading
import time
import uuid
from collections.abc import Callable, Iterator, Sequence
from pathlib import Path
from typing import Any, Final

from .errors import ErrorCode, InterruptedScopeth, ScopethError

PROTOCOL_VERSION: Final = "1.0"
MAX_MESSAGE_BYTES: Final = 4 * 1024 * 1024
MAX_DIAGNOSTIC_BYTES: Final = 1024 * 1024

JSONValue = None | bool | int | float | str | list["JSONValue"] | dict[str, "JSONValue"]
JSONObject = dict[str, Any]
ProgressCallback = Callable[[str, str], None]


class EngineClient:
    """Cliente de un proceso `scopeth-engine` por operación."""

    def __init__(
        self,
        engine: str | Path | Sequence[str] | None = None,
        *,
        timeout: float = 180.0,
        progress: ProgressCallback | None = None,
    ) -> None:
        self.command = resolve_engine(engine)
        self.timeout = timeout
        self.progress = progress

    def messages(
        self,
        operation: str,
        *,
        input_path: str | Path | None = None,
        run_dir: str | Path | None = None,
        options: JSONObject | None = None,
    ) -> Iterator[JSONObject]:
        request_id = str(uuid.uuid4())
        request: JSONObject = {
            "protocol_version": PROTOCOL_VERSION,
            "request_id": request_id,
            "operation": operation,
            "options": options or {},
        }
        if input_path is not None:
            request["input"] = str(Path(input_path).expanduser().absolute())
        if run_dir is not None:
            request["run_dir"] = str(Path(run_dir).expanduser().absolute())

        creationflags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
        try:
            process = subprocess.Popen(
                self.command,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                encoding="utf-8",
                errors="replace",
                bufsize=1,
                creationflags=creationflags,
            )
        except OSError as exc:
            raise ScopethError(
                ErrorCode.ENGINE_FAILED,
                f"No se pudo iniciar scopeth-engine: {exc}",
            ) from exc
        assert process.stdin is not None
        assert process.stdout is not None
        assert process.stderr is not None

        events: queue.Queue[tuple[str, str | None]] = queue.Queue()
        threads = [
            threading.Thread(
                target=_read_lines,
                args=(process.stdout, "stdout", events, MAX_MESSAGE_BYTES),
                daemon=True,
            ),
            threading.Thread(
                target=_read_lines,
                args=(process.stderr, "stderr", events, MAX_DIAGNOSTIC_BYTES),
                daemon=True,
            ),
        ]
        for thread in threads:
            thread.start()
        try:
            process.stdin.write(json.dumps(request, ensure_ascii=False, separators=(",", ":")))
            process.stdin.write("\n")
            process.stdin.close()
        except OSError as exc:
            process.kill()
            raise ScopethError(
                ErrorCode.ENGINE_FAILED, "El motor cerró stdin antes de tiempo."
            ) from exc

        deadline = time.monotonic() + self.timeout
        eof: set[str] = set()
        terminal_count = 0
        stderr_parts: list[str] = []
        try:
            while len(eof) < 2:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    process.kill()
                    raise ScopethError(
                        ErrorCode.RESOURCE_LIMIT,
                        f"El motor superó el tiempo permitido ({self.timeout:.0f} s).",
                    )
                try:
                    source, line = events.get(timeout=min(0.2, remaining))
                except queue.Empty:
                    continue
                if line is None:
                    eof.add(source)
                    continue
                if source == "stderr":
                    stderr_parts.append(line)
                    continue
                message = _decode_message(line, request_id)
                message_type = message["type"]
                if message_type == "progress":
                    if self.progress is not None:
                        self.progress(
                            str(message.get("phase", "engine")), str(message.get("message", ""))
                        )
                    continue
                if message_type in {"result", "error"}:
                    terminal_count += 1
                yield message
        except KeyboardInterrupt as exc:
            process.kill()
            raise InterruptedScopeth() from exc
        finally:
            if process.poll() is None:
                process.wait(timeout=2)
            for thread in threads:
                thread.join(timeout=1)

        if terminal_count != 1:
            diagnostic = "".join(stderr_parts).strip()
            if len(diagnostic) > 1000:
                diagnostic = diagnostic[:1000] + "..."
            raise ScopethError(
                ErrorCode.PROTOCOL_MISMATCH,
                f"Se esperaban exactamente un mensaje terminal y llegaron {terminal_count}.",
                {"exit_code": process.returncode, "stderr": diagnostic},
            )

    def run(self, operation: str, **kwargs: Any) -> tuple[list[JSONObject], JSONObject]:
        records: list[JSONObject] = []
        terminal: JSONObject | None = None
        for message in self.messages(operation, **kwargs):
            message_type = message["type"]
            if message_type == "record":
                record = message.get("record")
                if isinstance(record, dict):
                    records.append(record)
                else:
                    records.append({"value": record})
            elif message_type == "error":
                body = message.get("error")
                if not isinstance(body, dict):
                    raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "Error terminal mal formado.")
                raise ScopethError(
                    str(body.get("code", ErrorCode.ENGINE_FAILED)),
                    str(body.get("message", "El motor devolvió un error.")),
                    body.get("details") if isinstance(body.get("details"), dict) else None,
                )
            elif message_type == "result":
                result = message.get("result")
                if not isinstance(result, dict):
                    raise ScopethError(
                        ErrorCode.PROTOCOL_MISMATCH, "Resultado terminal mal formado."
                    )
                terminal = result
        if terminal is None:
            raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "El motor no devolvió un resultado.")
        return records, terminal


def resolve_engine(engine: str | Path | Sequence[str] | None = None) -> tuple[str, ...]:
    if engine is not None and not isinstance(engine, (str, Path)):
        command = tuple(str(part) for part in engine)
        if not command:
            raise ScopethError(ErrorCode.INPUT_INVALID, "El comando del motor está vacío.")
        return command
    configured = str(engine) if engine is not None else os.environ.get("SCOPETH_ENGINE")
    if configured:
        candidate = Path(configured).expanduser().absolute()
        if not candidate.is_file():
            raise ScopethError(
                ErrorCode.ENGINE_FAILED,
                f"El motor configurado no existe: {candidate}",
            )
        return (str(candidate),)

    executable = "scopeth-engine.exe" if os.name == "nt" else "scopeth-engine"
    package_candidate = Path(__file__).resolve().parent / "bin" / executable
    repository_candidate = Path(__file__).resolve().parents[3] / "bin" / executable
    for candidate in (package_candidate, repository_candidate):
        if candidate.is_file():
            return (str(candidate),)
    discovered = shutil.which("scopeth-engine")
    if discovered:
        return (discovered,)
    raise ScopethError(
        ErrorCode.ENGINE_FAILED,
        "No se encontró scopeth-engine. Compílalo o usa --engine/SCOPETH_ENGINE.",
    )


def _read_lines(
    stream: Any,
    source: str,
    events: queue.Queue[tuple[str, str | None]],
    byte_limit: int,
) -> None:
    consumed = 0
    try:
        for line in stream:
            consumed += len(line.encode("utf-8", errors="replace"))
            if consumed > byte_limit and source == "stderr":
                events.put((source, "[diagnóstico truncado]\n"))
                break
            if len(line.encode("utf-8", errors="replace")) > MAX_MESSAGE_BYTES:
                events.put((source, '{"type":"oversized"}\n'))
                break
            events.put((source, line))
    finally:
        events.put((source, None))


def _decode_message(line: str, request_id: str) -> JSONObject:
    try:
        message = json.loads(line)
    except json.JSONDecodeError as exc:
        raise ScopethError(
            ErrorCode.PROTOCOL_MISMATCH,
            f"El motor emitió una línea que no es JSON: {exc}",
        ) from exc
    if not isinstance(message, dict):
        raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "El mensaje del motor no es un objeto.")
    if message.get("protocol_version") != PROTOCOL_VERSION:
        raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "Versión de protocolo incompatible.")
    if message.get("request_id") != request_id:
        raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "request_id inesperado en la respuesta.")
    if message.get("type") not in {"progress", "record", "result", "error"}:
        raise ScopethError(ErrorCode.PROTOCOL_MISMATCH, "Tipo de mensaje desconocido.")
    return message
