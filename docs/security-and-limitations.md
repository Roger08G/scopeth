# Seguridad local y limitaciones

Las fuentes y los JSON de entrada se consideran datos no confiables.

## Controles implementados

- No se ejecutan contratos, configuraciones, scripts, Makefiles, npm, Hardhat ni Foundry.
- El binario del motor y `solc` proceden de configuración confiable; nunca se busca un ejecutable dentro del proyecto analizado.
- `solc` se invoca sin shell, con argumentos separados, timeout y límites de stdout/stderr.
- Standard JSON se reconstruye con outputs acotados; se ignoran settings arbitrarios de la entrada.
- URLs remotas, symlinks, archivos especiales, traversal y JSON/fuentes sobredimensionados se rechazan.
- Los archivos se leen como snapshot estable; si tamaño o mtime cambian durante la lectura, falla la entrada.
- La salida usa staging y renombrado atómico. Un `RUN_ID` existente no se sobrescribe.
- La caché valida esquema, clave e integridad SHA-256 antes de reutilizar un payload de compilación.
- Markdown y Mermaid escapan contenido no confiable antes de insertarlo.

## Frontera real

Separar `solc` en otro proceso no crea un sandbox. El timeout cancela el proceso directo, pero 1.0.0 no configura Job Objects, namespaces, seccomp, AppContainer ni límites duros del sistema operativo para todo el árbol de procesos. Para analizar código hostil con una frontera fuerte, ejecuta Scopeth dentro de una VM o contenedor endurecido y sin secretos.

## Limitaciones analíticas

- Efectos directos y resúmenes transitivos a través de llamadas internas y modifiers resueltos.
- Sin CFG completo ni prueba de factibilidad de rutas.
- Aliases locales y parámetros `storage` resueltos cuando existen referencias tipadas; los casos incompletos degradan a `unknown`.
- Los modifiers aportan su unión de efectos, pero no se modelan todavía el orden ni la multiplicidad de cada `_`.
- Assembly Yul seleccionado enumerado con argumentos simbólicos; no interpretado semánticamente de forma completa.
- Los destinos externos permanecen simbólicos sin estado on-chain.
- La versión 1.0.0 normaliza una unidad de compilación en un hilo; `--jobs` y `--compiler-jobs` se registran, pero todavía no activan workers paralelos.
- La clasificación de tokens exige evidencia tipada; las firmas ambiguas conservan resolución parcial.
- No hay severidades, explotación, RPC, wallets ni API keys.

`unknown` nunca significa "sin efectos" y una ausencia en los resultados nunca se presenta como prueba de seguridad.
