# Integración con Sourceth

El adaptador usa el esquema real de Sourceth `schema_version: 1`:

- exige `tool.name == "sourceth"`;
- acepta contratos con `source_status` `downloaded` o `reused`;
- interpreta `files[].relative_path` como la ruta portable desde la raíz de la ejecución (`contracts/<address>/sources/...`), tal como la publica Sourceth;
- comprueba `size_bytes` y `sha256` antes de analizar;
- conserva la dirección como procedencia, sin convertirla en identidad de código ejecutado.

Ejemplo:

```powershell
scopeth analyze --sourceth-manifest C:\download\RUN_ID\manifest.json `
  --output C:\analysis `
  --solc C:\tools\solc.exe
```

Cada espacio de fuentes se separa por dirección mediante un namespace para evitar colisiones de nombres. En 1.0.0 todos los archivos del manifiesto se envían como una unidad Standard JSON; proyectos multi-dirección con imports absolutos que dependan de espacios separados pueden requerir una planificación por unidades futura. Si `solc` no puede resolverlos, Scopeth lo declara y no reescribe imports.

El fixture `fixtures/sourceth-manifest/manifest.json` reproduce esa forma real y se valida con `solc` durante las pruebas de aceptación locales.
