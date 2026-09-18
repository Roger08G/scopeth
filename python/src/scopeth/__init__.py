"""API pública de Scopeth."""

from .api import AnalysisResult, AnalyzeOptions, Scopeth
from .errors import ErrorCode, ScopethError

__all__ = ["AnalysisResult", "AnalyzeOptions", "ErrorCode", "Scopeth", "ScopethError"]
__version__ = "1.0.0"
