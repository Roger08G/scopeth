from __future__ import annotations

from enum import StrEnum
from typing import Any


class ErrorCode(StrEnum):
    INPUT_INVALID = "INPUT_INVALID"
    COMPILER_NOT_FOUND = "COMPILER_NOT_FOUND"
    COMPILER_UNSUPPORTED = "COMPILER_UNSUPPORTED"
    IMPORT_UNRESOLVED = "IMPORT_UNRESOLVED"
    COMPILATION_FAILED = "COMPILATION_FAILED"
    PROTOCOL_MISMATCH = "PROTOCOL_MISMATCH"
    RESOURCE_LIMIT = "RESOURCE_LIMIT"
    ANALYSIS_INCOMPLETE = "ANALYSIS_INCOMPLETE"
    UNSAFE_PATH = "UNSAFE_PATH"
    CACHE_CORRUPT = "CACHE_CORRUPT"
    ENGINE_FAILED = "ENGINE_FAILED"


class ScopethError(RuntimeError):
    def __init__(
        self,
        code: ErrorCode | str,
        message: str,
        details: dict[str, Any] | None = None,
    ) -> None:
        try:
            self.code = ErrorCode(code)
        except ValueError:
            self.code = ErrorCode.ENGINE_FAILED
        self.message = message
        self.details = details or {}
        super().__init__(f"{self.code}: {message}")


class InterruptedScopeth(ScopethError):
    def __init__(self) -> None:
        super().__init__(ErrorCode.ENGINE_FAILED, "Operación interrumpida por el usuario.")
