# Protocolo Python ↔ Go

Versión actual: `1.0`.

Python inicia un solo proceso `scopeth-engine` por comando. Escribe una solicitud JSON en `stdin`; el motor emite NDJSON por `stdout`. El AST y los resultados voluminosos nunca cruzan el protocolo.

## Solicitud

```json
{
  "protocol_version": "1.0",
  "request_id": "uuid",
  "operation": "analyze",
  "input": "C:/sources",
  "options": {
    "mode": "auto",
    "profile": "deep",
    "output": "C:/analysis",
    "solc": "C:/tools/solc.exe"
  }
}
```

Límite de solicitud: 1 MiB. Los campos desconocidos del envelope se rechazan.

## Respuestas

- `progress`: cero o más; la CLI lo muestra por `stderr`.
- `record`: cero o más para consultas paginadas.
- `result` o `error`: exactamente uno y siempre terminal.

Todos los mensajes incluyen `protocol_version` y el mismo `request_id`. La CLI consume `stdout` y `stderr` de forma concurrente, limita diagnósticos a 1 MiB y cada mensaje a 4 MiB, controla EOF, timeout, IDs y versión.

Con `--json`, `stdout` contiene exclusivamente el JSON público solicitado. El progreso no se mezcla con él.

## Operaciones 1.0

- `doctor` y `analyze`.
- `query_functions`, `query_flows`, `query_state`, `query_surfaces` y `query_configuration`, con `offset`/`limit`.
- `query_explain`, que agrega las relaciones de una función sin releer el AST.
- `query_graph`, para llamadas, imports, herencia, estado y activos en JSON o Mermaid.

Las consultas leen JSONL de forma incremental en el motor. La API Python expone iteradores; la CLI materializa solo la página solicitada, cuyo límite máximo es 10.000 registros.

## Errores estables

`INPUT_INVALID`, `COMPILER_NOT_FOUND`, `COMPILER_UNSUPPORTED`, `IMPORT_UNRESOLVED`, `COMPILATION_FAILED`, `PROTOCOL_MISMATCH`, `RESOURCE_LIMIT`, `ANALYSIS_INCOMPLETE`, `UNSAFE_PATH`, `CACHE_CORRUPT` y `ENGINE_FAILED`.

## Códigos de salida CLI

| Código | Significado |
|---:|---|
| 0 | Resultado completo para el alcance declarado |
| 2 | Argumentos o entrada inválidos |
| 3 | Resultado parcial y utilizable |
| 4 | Operación fallida |
| 130 | Interrupción del usuario |
