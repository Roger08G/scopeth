# Benchmarks reproducibles

Scopeth no publica objetivos de rendimiento inventados. Este directorio define cómo producir mediciones observadas.

## Corpus incluido

- `small`: `fixtures/standard-json/simple.json`.
- `medium`: `fixtures/solidity/Vault.sol`.
- `advanced`: herencia, recursion, try/catch, aliases y varios estándares de activos.
- `large`: no incluido todavía; debe ser un proyecto público fijado a una revisión y con licencia compatible.

## Ejecución

```powershell
.\benchmarks\run.ps1 `
  -Solc "C:\tools\solc.exe" `
  -Iterations 5 `
  -Output .\benchmarks\observed.json
```

El script registra hardware/runtime, corpus, iteración, tiempo total y tamaño de resultados. La memoria de procesos hijo se marca `unavailable` en lugar de confundir el heap Go con el consumo total. Los resultados observados no se versionan por defecto porque dependen del equipo.

Para inspeccionar CPU y heap sin abrir un servidor `pprof`, añade rutas nuevas a un análisis:

```powershell
uv run --project .\python scopeth analyze .\fixtures\solidity `
  --solc "C:\tools\solc.exe" `
  --cpu-profile .\benchmarks\cpu.prof `
  --heap-profile .\benchmarks\heap.prof

go tool pprof -top .\bin\scopeth-engine.exe .\benchmarks\cpu.prof
go tool pprof -top .\bin\scopeth-engine.exe .\benchmarks\heap.prof
```

Las rutas son configuración confiable y se crean de forma exclusiva: Scopeth rechaza sobrescribir perfiles existentes.
