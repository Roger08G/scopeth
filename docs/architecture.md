# Arquitectura y pases

Scopeth utiliza una frontera intencionada entre presentación y análisis. Python no procesa AST ni recorre JSONL para resolver consultas; Go no decide cómo presentar la salida humana.

## Componentes

1. `python/src/scopeth`: argumentos, API, proceso del motor, validación del protocolo y salida Rich/JSON.
2. `cmd/scopeth-engine`: una solicitud por proceso y exactamente un mensaje terminal.
3. `internal/input`: snapshot, límites y adaptadores de entrada.
4. `internal/compiler`: identidad de `solc`, Standard JSON seguro, timeout y diagnósticos.
5. `internal/ast`: normalización e inventario; el AST no se expone como API.
6. `internal/model`: tipos concretos para símbolos, llamadas, estado, guards, activos y evidencias.
7. `internal/analysis`: IR, efectos tipados y punto fijo por SCC sobre llamadas internas y modifiers resueltos.
8. `internal/store`: JSON/JSONL determinista, hashes y publicación atómica.
9. `internal/query`: filtros y streaming sobre resultados grandes.

## Pipeline implementado

```text
fuentes
  → snapshot estable y hashes
  → solc --standard-json
  → caché de compilación por contenido e identidad del compilador
  → AST + ABI + storage layout
  → símbolos e IDs normalizados
  → relaciones, aliases y efectos directos
  → bindings de parámetros storage
  → IR + resúmenes transitivos por SCC
  → activos, autorizaciones, configuración y superficies de revisión
  → resultados inmutables
  → consultas paginadas/streaming y grafos bajo demanda
```

El perfil `inventory` termina después del inventario. `deep` añade llamadas, accesos directos de estado, comprobaciones, operaciones de activos y observaciones.

## Identidad

- El source unit lógico, la ruta física y el hash de contenido son campos distintos.
- Los IDs públicos se derivan con SHA-256 de una clave namespaced y no dependen únicamente del nombre, selector o node ID de `solc`.
- Los node IDs solo se conservan como procedencia dentro de la compilación.
- Los offsets son bytes UTF-8. Línea y columna son 1-based; la columna se calcula en bytes.

## Estado actual de los pases

| Pase | Estado 1.0.0 |
|---|---|
| Descubrimiento y compilación | Implementado |
| Inventario, ABI y storage layout | Implementado |
| Imports, herencia y llamadas directas | Implementado |
| Lecturas/escrituras directas | Implementado |
| Guards sintácticos y modifiers aplicados | Implementado; los efectos se unen al resumen de la función |
| Activos/autorizaciones/contabilidad | Implementado con unidades, dependencias, guards, retorno, alcance y procedencia |
| Configuración y actor simbólico | Implementado con guardas y entradas alcanzables |
| Superficies de revisión | Implementadas como observaciones sin severidad |
| CFG completo | Pendiente |
| Alias locales y parámetros `storage` | Implementado con rutas simbólicas y bindings tipados |
| Punto fijo de llamadas internas/modifiers resueltos | Implementado por SCC, incluida recursión |
| Propagación por llamadas externas/dinámicas | No se asume; degrada resolución |
| Enumeración Yul seleccionada | Implementada; semántica completa pendiente |
| Consultas y grafos bajo demanda | Implementados con paginación y salida JSON/Mermaid |
| Resultados, manifiesto, informe y caché | Implementados con hashes e invalidación conservadora |

No se devuelve una lista vacía para fingir una capacidad pendiente. La cobertura aparece en `summary.json`, `manifest.json` y `report.md`.

Los modifiers participan como nodos internos del análisis, pero no se publican como resúmenes de funciones independientes. F3 conserva su unión de efectos; la secuencia exacta alrededor de cada placeholder `_` pertenece al CFG y no se infiere.
