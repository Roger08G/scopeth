# Scopeth Python

Interfaz de terminal y biblioteca Python de **Scopeth**, una herramienta local de enumeración profunda de Solidity.

Este paquete coordina el binario confiable `scopeth-engine`; el análisis semántico, los grafos, los resúmenes y las consultas se ejecutan en el motor Go. Consulta la documentación completa en el `README.md` de la raíz del proyecto.

```python
from pathlib import Path

from scopeth import AnalyzeOptions, Scopeth

client = Scopeth(engine=Path("bin/scopeth-engine"))
result = client.analyze(
    Path("contracts"),
    AnalyzeOptions(solc=Path("tools/solc"), output=Path("analysis")),
)
print(result.report)
```

Scopeth no descarga compiladores, no ejecuta contratos ni reconstruye actividad on-chain.
