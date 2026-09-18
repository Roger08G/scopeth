# Contribuir a Scopeth

Gracias por ayudar a mejorar Scopeth. El proyecto acepta correcciones, mejoras de
rendimiento y ampliaciones acotadas de la enumeración estática de Solidity.

## Antes de empezar

- Abre una incidencia para cambios de comportamiento o de protocolo.
- Mantén el análisis local, determinista y sin acceso implícito a la red.
- No conviertas las observaciones en afirmaciones de vulnerabilidad o severidad.
- Añade evidencia y una limitación explícita a cada nueva conclusión del motor.

## Entorno

Necesitas Go 1.24 o posterior, Python 3.12–3.14, `uv` y, para la integración
semántica, un binario `solc` confiable.

```powershell
uv sync --project .\python --all-groups --locked
.\scripts\build.ps1
.\scripts\validate.ps1 -Solc "C:\ruta\confiable\solc.exe"
```

En Linux o macOS utiliza `scripts/build.sh` y `scripts/validate.sh`.

## Pull requests

1. Limita cada cambio a una responsabilidad clara.
2. Incluye tests para los casos nuevos y para los límites conocidos.
3. Ejecuta la validación completa antes de abrir el pull request.
4. Actualiza el protocolo, los esquemas y la documentación cuando corresponda.
5. No incluyas contratos privados, secretos, artefactos de análisis ni binarios.

Los commits deben explicar el propósito del cambio y mantener el historial fácil de
auditar.
