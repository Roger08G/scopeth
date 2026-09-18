from __future__ import annotations

from pathlib import Path

import pytest
from rich.console import Console

from scopeth.cli import build_parser
from scopeth.errors import ErrorCode, ScopethError
from scopeth.presentation import render_graph


def test_analyze_parser() -> None:
    args = build_parser().parse_args(["analyze", "contracts", "--mode", "semantic"])
    assert args.command == "analyze"
    assert args.mode == "semantic"
    assert str(args.source) == "contracts"


def test_analyze_requires_input() -> None:
    with pytest.raises(SystemExit) as captured:
        build_parser().parse_args(["analyze"])
    assert captured.value.code == 2


def test_functions_payable_filter() -> None:
    args = build_parser().parse_args(["functions", "analysis/run", "--payable", "--json"])
    assert args.payable is True
    assert args.json is True


def test_f4_f5_query_parsers() -> None:
    flows = build_parser().parse_args(
        ["flows", "analysis/run", "--operation", "transfer", "--reachable-from", "A.f()"]
    )
    assert flows.operation == "transfer"
    assert flows.reachable_from == "A.f()"

    surfaces = build_parser().parse_args(
        ["surfaces", "analysis/run", "--category", "assembly", "--resolution", "partial"]
    )
    assert surfaces.category == "assembly"

    configuration = build_parser().parse_args(
        ["configuration", "analysis/run", "--category", "proxy"]
    )
    assert configuration.category == "proxy"

    graph = build_parser().parse_args(["graph", "analysis/run", "--kind", "assets"])
    assert graph.kind == "assets"


def test_graph_output_is_immutable(tmp_path: Path) -> None:
    target = tmp_path / "graph.mmd"
    console = Console(file=None, force_terminal=False)
    render_graph(console, {"content": "flowchart LR\n"}, target)
    with pytest.raises(ScopethError) as captured:
        render_graph(console, {"content": "changed\n"}, target)
    assert captured.value.code == ErrorCode.UNSAFE_PATH
    assert target.read_text(encoding="utf-8") == "flowchart LR\n"
