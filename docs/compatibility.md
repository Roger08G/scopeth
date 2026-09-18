# Compatibilidad comprobada

Esta matriz solo incluye ejecuciones reales. No implica soporte universal de Solidity.

| Fecha | Plataforma | Go | Python | solc | Resultado |
|---|---|---:|---:|---:|---|
| 2026-09-17 | Windows amd64 | 1.27.1 | 3.12.14 | 0.8.37 | Tests Python; motor validado por separado |
| 2026-09-17 | Windows amd64 | 1.27.1 | 3.13.15 | 0.8.37 | Tests Python; motor validado por separado |
| 2026-09-17 | Windows amd64 | 1.27.1 | 3.14.6 | 0.8.37 | Tests, wheel y fixture `Vault.sol` completo |

## Capacidades comprobadas con 0.8.37

- Standard JSON Input nativo.
- AST de fuentes y referencias semánticas.
- ABI, `storageLayout` y `transientStorageLayout` solicitados sin bytecode.
- `stopAfter=parsing` para el modo sintáctico.
- Getters públicos, constructor, receive, fallback, funciones libres, interfaces, herencia y modifiers.
- Transferencias nativas, `call{value: ...}`, ERC-20 tipado, autorización y delegatecall observado.
- Propagación de lecturas/escrituras mediante llamadas internas `_credit` y `_debit`.
- Aliases locales, parámetros `storage` encadenados y rutas de campos/mappings preservadas en el fixture avanzado.
- Recursión resuelta por SCC y punto fijo convergente.
- Efectos de modifiers, incluidos bindings `storage`, sin afirmar el orden exacto de sus placeholders.
- Herencia múltiple, overrides, try/catch, arrays de estado, storage transitorio y clasificación tipada ERC-721/ERC-1155 en el fixture avanzado.
- ERC-4626 con unidades assets/shares, wrapping/unwrapping, valor nativo, creación con valor y `selfdestruct` en el fixture F4.
- Retornos ERC-20 comprobados, propagados y no comprobados; la función personalizada `transfer` no se clasifica como ERC-20.
- Configuración sensible, actor simbólico, reachability y superficies de revisión sin severidad.
- AST Yul enumerado para `sload/sstore`, `tload/tstore`, `call/staticcall`, `create2`, `log0` y `switch`.
- Consultas de funciones, flujos, estado, configuración y superficies, más grafos de llamadas/imports/herencia/estado/activos.

## Política de versiones

Scopeth ejecuta el binario indicado por `--solc`, `SCOPETH` no descarga uno. Si la entrada proporciona una identidad de compilador y no coincide, la ejecución queda marcada `analysis_only`. Un pragma compatible no se trata como evidencia de la versión original.

Solidity anterior a `0.8.x`, targets EVM específicos y semántica completa de storage transitorio todavía no tienen una matriz suficiente; el proyecto no los anuncia como soportados.
