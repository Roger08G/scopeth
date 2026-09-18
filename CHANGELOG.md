# Changelog

Todos los cambios relevantes de Scopeth se documentan en este archivo.

## 1.0.0 - 2026-09-17

Primera versión estable para el alcance declarado.

### Análisis

- Inventario semántico basado en AST, ABI y storage layout de `solc`.
- Efectos directos y transitivos, aliases `storage` y punto fijo por SCC.
- Movimientos nativos, ERC-20/721/1155, ERC-4626, wrapped native, autorizaciones y contabilidad interna.
- Cantidades, unidades, dependencias, retorno, callbacks, guards, reachability y estado relacionado.
- Configuración sensible con actores simbólicos.
- Superficies de revisión con evidencia, supuestos y limitaciones, sin severidad.
- Enumeración seleccionada del AST Yul.

### Producto

- Resultados inmutables con hashes, manifiesto, cobertura e informe Markdown.
- Consultas paginadas/streaming y grafos bajo demanda.
- Caché de compilación verificada e invalidación conservadora.
- CLI y API Python 3.12–3.14, motor Go y protocolo NDJSON 1.0.

### Límites declarados

- No incluye CFG completo ni prueba de factibilidad de rutas.
- No consulta estado on-chain ni identifica implementaciones desplegadas.
- La enumeración Yul no equivale a semántica completa de assembly.
