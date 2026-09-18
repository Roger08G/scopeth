from __future__ import annotations

import argparse
import json
import os
import sys
from collections.abc import Sequence
from pathlib import Path
from typing import Any

from rich.console import Console
from rich.markup import escape

from . import __version__
from .api import AnalyzeOptions, Scopeth
from .errors import ErrorCode, ScopethError
from .presentation import (
    render_analysis,
    render_configuration,
    render_doctor,
    render_explain,
    render_flows,
    render_functions,
    render_generic,
    render_graph,
    render_state,
    render_surfaces,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="scopeth",
        description="Enumeración profunda y trazable de código Solidity.",
    )
    parser.add_argument("--version", action="version", version=f"%(prog)s {__version__}")
    parser.add_argument("--engine", type=Path, help="Ruta confiable a scopeth-engine.")
    parser.add_argument("--json", action="store_true", help="Emite solo JSON público por stdout.")
    parser.add_argument(
        "--no-color", action="store_true", help="Desactiva colores en salida humana."
    )
    commands = parser.add_subparsers(dest="command", required=True)

    doctor = commands.add_parser("doctor", help="Comprueba motor y solc sin usar la red.")
    _runtime_common(doctor)
    doctor.add_argument("--solc", type=Path, help="Ruta confiable al ejecutable solc.")

    analyze = commands.add_parser(
        "analyze", help="Analiza un archivo, directorio o JSON de Solidity."
    )
    _runtime_common(analyze)
    source = analyze.add_mutually_exclusive_group(required=True)
    source.add_argument(
        "source", nargs="?", type=Path, help=".sol, directorio, Standard JSON o build-info."
    )
    source.add_argument(
        "--sourceth-manifest", type=Path, help="Manifiesto schema_version 1 de Sourceth."
    )
    analyze.add_argument("--mode", choices=("auto", "semantic", "syntax"), default="auto")
    analyze.add_argument("--profile", choices=("inventory", "deep"), default="deep")
    analyze.add_argument("--output", type=Path, default=Path("analysis"))
    analyze.add_argument("--solc", type=Path, help="Ruta confiable al ejecutable solc.")
    analyze.add_argument("--jobs", type=_positive_int, default=max(1, min(os.cpu_count() or 1, 8)))
    analyze.add_argument("--compiler-jobs", type=_positive_int, default=1)
    analyze.add_argument("--timeout", type=_positive_int, default=120, metavar="SECONDS")
    analyze.add_argument(
        "--remapping", action="append", default=[], help="Remapping de imports; repetible."
    )
    analyze.add_argument("--refresh", action="store_true", help="Ignora la caché de compilación.")
    analyze.add_argument(
        "--cpu-profile", type=Path, help="Escribe un perfil CPU nuevo para go tool pprof."
    )
    analyze.add_argument(
        "--heap-profile", type=Path, help="Escribe un perfil heap nuevo para go tool pprof."
    )

    functions = commands.add_parser(
        "functions", help="Consulta funciones sin cargar todo el resultado."
    )
    _runtime_common(functions)
    _query_common(functions)
    functions.add_argument("--visibility", choices=("external", "public", "internal", "private"))
    functions.add_argument("--payable", action=argparse.BooleanOptionalAction, default=None)
    functions.add_argument("--mutability", choices=("pure", "view", "nonpayable", "payable"))
    functions.add_argument("--modifier")

    flows = commands.add_parser(
        "flows", help="Consulta movimientos, autorizaciones y contabilidad."
    )
    _runtime_common(flows)
    _query_common(flows)
    flows.add_argument("--asset-class")
    flows.add_argument("--category", choices=("movement", "authorization", "accounting"))
    flows.add_argument("--operation")
    flows.add_argument("--reachable-from")

    explain = commands.add_parser("explain", help="Explica una función y sus relaciones directas.")
    _runtime_common(explain)
    explain.add_argument("run_dir", type=Path)
    explain.add_argument("--symbol", required=True)

    state = commands.add_parser("state", help="Consulta lecturas y escrituras de estado.")
    _runtime_common(state)
    _query_common(state)
    state.add_argument("--written-by")
    state.add_argument("--variable")
    state.add_argument("--access", choices=("read", "write", "read_write"))

    surfaces = commands.add_parser(
        "surfaces", help="Consulta superficies de revisión sin asignar severidad."
    )
    _runtime_common(surfaces)
    _query_common(surfaces)
    surfaces.add_argument("--category")
    surfaces.add_argument("--rule")
    surfaces.add_argument("--resolution", choices=("resolved", "partial", "unknown"))

    configuration = commands.add_parser(
        "configuration", help="Consulta cambios de configuración y su actor simbólico."
    )
    _runtime_common(configuration)
    _query_common(configuration)
    configuration.add_argument("--category")
    configuration.add_argument("--reachable-from")

    graph = commands.add_parser("graph", help="Exporta un subgrafo bajo demanda.")
    _runtime_common(graph)
    graph.add_argument("run_dir", type=Path)
    graph.add_argument(
        "--kind", choices=("calls", "imports", "inheritance", "state", "assets"), default="calls"
    )
    graph.add_argument("--root")
    graph.add_argument("--depth", type=_positive_int, default=4)
    graph.add_argument("--format", choices=("mermaid", "json"), default="mermaid")
    graph.add_argument("--output", type=Path)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    _configure_utf8()
    parser = build_parser()
    args = parser.parse_args(argv)
    console = Console(stderr=False, no_color=args.no_color)
    diagnostics = Console(stderr=True, no_color=args.no_color)

    def progress(phase: str, message: str) -> None:
        if not args.json:
            diagnostics.print(f"[cyan]{escape(phase)}[/cyan] {escape(message)}")

    try:
        scopeth = Scopeth(args.engine, timeout=_client_timeout(args), progress=progress)
        if args.command == "doctor":
            doctor_result = scopeth.doctor(solc=args.solc)
            _emit(args.json, console, doctor_result, render_doctor)
            return 0 if doctor_result.get("solc_available") else 4
        if args.command == "analyze":
            source = args.sourceth_manifest or args.source
            assert source is not None
            analysis_result = scopeth.analyze(
                source,
                AnalyzeOptions(
                    mode=args.mode,
                    profile=args.profile,
                    output=args.output,
                    solc=args.solc,
                    jobs=args.jobs,
                    compiler_jobs=args.compiler_jobs,
                    timeout_seconds=args.timeout,
                    remappings=tuple(args.remapping),
                    refresh=args.refresh,
                    cpu_profile=args.cpu_profile,
                    heap_profile=args.heap_profile,
                ),
                sourceth_manifest=args.sourceth_manifest is not None,
            )
            if args.json:
                _print_json(
                    {
                        "run_id": analysis_result.run_id,
                        "run_dir": str(analysis_result.run_dir),
                        "status": analysis_result.status,
                        "report": str(analysis_result.report),
                        "mode_used": analysis_result.mode_used,
                        "solc_version": analysis_result.solc_version,
                        "solc_sha256": analysis_result.solc_sha256,
                        "cache_reused": analysis_result.cache_reused,
                        "engine_version": analysis_result.engine_version,
                        "result_schema_version": analysis_result.result_schema_version,
                        "summary": analysis_result.summary,
                    }
                )
            else:
                render_analysis(console, analysis_result)
            return 3 if analysis_result.status == "partial" else 0
        if args.command == "functions":
            records = list(
                scopeth.functions(
                    args.run_dir,
                    visibility=args.visibility,
                    payable=args.payable,
                    mutability=args.mutability,
                    modifier=args.modifier,
                    offset=args.offset,
                    limit=args.limit,
                )
            )
            _emit_records(args.json, console, records, render_functions)
            return 0
        if args.command == "flows":
            records = list(
                scopeth.flows(
                    args.run_dir,
                    asset_class=args.asset_class,
                    category=args.category,
                    operation=args.operation,
                    reachable_from=args.reachable_from,
                    offset=args.offset,
                    limit=args.limit,
                )
            )
            _emit_records(args.json, console, records, render_flows)
            return 0
        if args.command == "state":
            records = list(
                scopeth.state(
                    args.run_dir,
                    written_by=args.written_by,
                    variable=args.variable,
                    access=args.access,
                    offset=args.offset,
                    limit=args.limit,
                )
            )
            _emit_records(args.json, console, records, render_state)
            return 0
        if args.command == "surfaces":
            records = list(
                scopeth.surfaces(
                    args.run_dir,
                    category=args.category,
                    rule=args.rule,
                    resolution=args.resolution,
                    offset=args.offset,
                    limit=args.limit,
                )
            )
            _emit_records(args.json, console, records, render_surfaces)
            return 0
        if args.command == "configuration":
            records = list(
                scopeth.configuration(
                    args.run_dir,
                    category=args.category,
                    reachable_from=args.reachable_from,
                    offset=args.offset,
                    limit=args.limit,
                )
            )
            _emit_records(args.json, console, records, render_configuration)
            return 0
        if args.command == "explain":
            record = scopeth.explain(args.run_dir, args.symbol)
            if args.json:
                _print_json(record)
            else:
                render_explain(console, record)
            return 0
        if args.command == "graph":
            record = scopeth.graph(
                args.run_dir, kind=args.kind, root=args.root, depth=args.depth, format=args.format
            )
            if args.json:
                _print_json(record)
            elif args.format == "mermaid":
                render_graph(console, record, args.output)
            else:
                render_generic(console, record)
            return 0
        parser.error("comando desconocido")
    except ScopethError as exc:
        if args.json:
            _print_json(
                {"error": {"code": exc.code, "message": exc.message, "details": exc.details}}
            )
        else:
            diagnostics.print(f"[bold red]{exc.code}[/bold red]: {escape(exc.message)}")
        return 2 if exc.code == ErrorCode.INPUT_INVALID else 4
    except KeyboardInterrupt:
        if not args.json:
            diagnostics.print("[yellow]Interrumpido por el usuario.[/yellow]")
        return 130
    return 4


def _configure_utf8() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            reconfigure(encoding="utf-8", errors="replace")


def _query_common(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("run_dir", type=Path)
    parser.add_argument("--offset", type=_non_negative_int, default=0)
    parser.add_argument("--limit", type=_positive_int, default=100)


def _runtime_common(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--engine", type=Path, default=argparse.SUPPRESS, help=argparse.SUPPRESS)
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    parser.add_argument(
        "--no-color", action="store_true", default=argparse.SUPPRESS, help=argparse.SUPPRESS
    )


def _positive_int(value: str) -> int:
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("debe ser mayor que cero")
    return parsed


def _non_negative_int(value: str) -> int:
    parsed = int(value)
    if parsed < 0:
        raise argparse.ArgumentTypeError("no puede ser negativo")
    return parsed


def _client_timeout(args: argparse.Namespace) -> float:
    return float(getattr(args, "timeout", 120) + 30)


def _print_json(value: Any) -> None:
    sys.stdout.write(json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str))
    sys.stdout.write("\n")


def _emit(
    json_mode: bool,
    console: Console,
    result: dict[str, Any],
    renderer: Any,
) -> None:
    if json_mode:
        _print_json(result)
    else:
        renderer(console, result)


def _emit_records(
    json_mode: bool, console: Console, records: list[dict[str, Any]], renderer: Any
) -> None:
    if json_mode:
        _print_json(records)
    else:
        renderer(console, records)


if __name__ == "__main__":
    raise SystemExit(main())
