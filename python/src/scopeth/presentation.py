from __future__ import annotations

from collections.abc import Sequence
from pathlib import Path
from typing import Any

from rich.console import Console
from rich.markup import escape
from rich.panel import Panel
from rich.pretty import Pretty
from rich.table import Table
from rich.text import Text

from .api import AnalysisResult
from .errors import ErrorCode, ScopethError
from .protocol import JSONObject


def render_doctor(console: Console, result: JSONObject) -> None:
    table = Table(title="Diagnóstico de Scopeth", border_style="cyan", show_header=False)
    table.add_column("Componente", style="bold")
    table.add_column("Estado")
    table.add_row("Motor", _cell(result.get("engine_version", "unknown")))
    table.add_row("Protocolo", _cell(result.get("protocol_version", "unknown")))
    table.add_row("Go", _cell(result.get("go_version", "unknown")))
    available = bool(result.get("solc_available"))
    status = (
        f"[green]disponible[/] · {_cell(result.get('solc_version', ''))}"
        if available
        else "[red]no disponible[/]"
    )
    table.add_row("solc", status)
    table.add_row("Red", "no utilizada")
    console.print(table)
    if not available and result.get("solc_error"):
        console.print(
            Panel(Text(str(result["solc_error"])), title="Detalle de solc", border_style="red")
        )


def render_analysis(console: Console, result: AnalysisResult) -> None:
    counts = result.summary.get("counts")
    count_text = ""
    if isinstance(counts, dict):
        selected = [
            "sources",
            "contracts",
            "functions",
            "calls",
            "state_accesses",
            "asset_operations",
            "configuration_changes",
            "observations",
        ]
        count_text = "  ".join(f"{key}={counts.get(key, 0)}" for key in selected)
    color = "green" if result.status == "complete" else "yellow"
    lines = [
        f"Estado: [{color}]{result.status}[/]",
        f"Modo: [bold]{result.mode_used}[/]",
        f"solc: {escape(result.solc_version)}",
        f"Caché de compilación: {'reutilizada' if result.cache_reused else 'nueva'}",
        f"Resultado: {escape(str(result.run_dir))}",
        f"Informe: {escape(str(result.report))}",
    ]
    if count_text:
        lines.append(count_text)
    console.print(Panel("\n".join(lines), title=f"Scopeth · {result.run_id}", border_style=color))


def render_functions(console: Console, records: Sequence[JSONObject]) -> None:
    table = Table(title=f"Funciones ({len(records)})", border_style="cyan")
    table.add_column("Símbolo", overflow="fold")
    table.add_column("Visibilidad")
    table.add_column("Mutabilidad")
    table.add_column("Selector")
    table.add_column("Origen")
    for record in records:
        table.add_row(
            _cell(record.get("symbol", "")),
            _cell(record.get("visibility", "")),
            _cell(record.get("state_mutability", "")),
            _cell(record.get("selector", "—") or "—"),
            "getter" if record.get("generated") else "fuente",
        )
    console.print(table)


def render_flows(console: Console, records: Sequence[JSONObject]) -> None:
    table = Table(title=f"Operaciones de activos ({len(records)})", border_style="magenta")
    table.add_column("Función", overflow="fold")
    table.add_column("Categoría")
    table.add_column("Activo")
    table.add_column("Operación")
    table.add_column("Cantidad")
    table.add_column("Resolución")
    for record in records:
        table.add_row(
            _cell(record.get("function", "")),
            _cell(record.get("category", "")),
            _cell(record.get("asset_class", "")),
            _cell(record.get("operation", "")),
            _cell(record.get("amount_expression", "unknown")),
            _cell(record.get("resolution", "")),
        )
    console.print(table)


def render_state(console: Console, records: Sequence[JSONObject]) -> None:
    table = Table(title=f"Accesos de estado ({len(records)})", border_style="blue")
    table.add_column("Función", overflow="fold")
    table.add_column("Acceso")
    table.add_column("Variable")
    table.add_column("Resolución")
    for record in records:
        table.add_row(
            _cell(record.get("function", "")),
            _cell(record.get("access", "")),
            _cell(record.get("variable", "")),
            _cell(record.get("resolution", "")),
        )
    console.print(table)


def render_surfaces(console: Console, records: Sequence[JSONObject]) -> None:
    table = Table(title=f"Superficies de revisión ({len(records)})", border_style="yellow")
    table.add_column("Función", overflow="fold")
    table.add_column("Categoría")
    table.add_column("Regla")
    table.add_column("Observación", overflow="fold")
    table.add_column("Resolución")
    for record in records:
        table.add_row(
            _cell(record.get("function", "—") or "—"),
            _cell(record.get("category", "")),
            _cell(record.get("rule", "")),
            _cell(record.get("title", "")),
            _cell(record.get("resolution", "")),
        )
    console.print(table)


def render_configuration(console: Console, records: Sequence[JSONObject]) -> None:
    table = Table(title=f"Cambios de configuración ({len(records)})", border_style="red")
    table.add_column("Función", overflow="fold")
    table.add_column("Categoría")
    table.add_column("Ruta")
    table.add_column("Actor simbólico", overflow="fold")
    table.add_column("Resolución")
    for record in records:
        table.add_row(
            _cell(record.get("function", "")),
            _cell(record.get("category", "")),
            _cell(record.get("path", "")),
            _cell(record.get("actor_model", "")),
            _cell(record.get("resolution", "")),
        )
    console.print(table)


def render_explain(console: Console, record: JSONObject) -> None:
    function = record.get("function")
    if not isinstance(function, dict):
        console.print(Pretty(record))
        return
    title = str(function.get("symbol", "Función"))
    summary = Table.grid(padding=(0, 2))
    summary.add_column(style="bold cyan")
    summary.add_column()
    for label, key in (
        ("Firma", "signature"),
        ("Visibilidad", "visibility"),
        ("Mutabilidad", "state_mutability"),
        ("Selector", "selector"),
    ):
        summary.add_row(label, _cell(function.get(key, "—") or "—"))
    console.print(Panel(summary, title=title, border_style="cyan"))
    for label, key in (
        ("Resumen interprocedimental", "summary"),
        ("Comprobaciones", "guards"),
        ("Estado", "state_accesses"),
        ("Aliases storage", "storage_aliases"),
        ("Llamadas", "calls"),
        ("Eventos alcanzables", "events"),
        ("Reverts/errores", "reverts"),
        ("Relaciones no resueltas", "unresolved_relations"),
        ("Activos/autorizaciones", "asset_operations"),
        ("Configuración", "configuration_changes"),
        ("Assembly", "assembly_operations"),
        ("Modelo de actor", "caller_model"),
        ("Observaciones", "observations"),
        ("Evidencias", "evidence"),
    ):
        values = record.get(key)
        if isinstance(values, (dict, list)) and values:
            console.print(Panel(Pretty(values, expand_all=False), title=label, border_style="dim"))


def render_graph(console: Console, record: JSONObject, output: Path | None = None) -> None:
    content = str(record.get("content", ""))
    if output is not None:
        try:
            with output.open("x", encoding="utf-8", newline="\n") as stream:
                stream.write(content)
        except FileExistsError as exc:
            raise ScopethError(
                ErrorCode.UNSAFE_PATH,
                f"El archivo de grafo ya existe y no se sobrescribe: {output}",
            ) from exc
        except OSError as exc:
            raise ScopethError(
                ErrorCode.UNSAFE_PATH, f"No se pudo crear el archivo de grafo: {output}: {exc}"
            ) from exc
        console.print(f"Grafo escrito en [bold]{escape(str(output))}[/bold]")
        return
    console.print(Text(content))


def render_generic(console: Console, value: Any) -> None:
    console.print(Pretty(value, expand_all=False))


def _cell(value: Any) -> str:
    return escape(str(value))
