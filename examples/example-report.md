# Informe Scopeth de ejemplo

> Extracto de un resultado real del fixture `fixtures/f4/Assets.sol`, ejecutado el 2026-09-17 con `scopeth-engine 1.0.0`, Go 1.27.1 y `solc 0.8.37+commit.f401782d.Windows.msvc`.

## Cobertura declarada

- Estado operativo: `complete`.
- Modo solicitado/utilizado: `semantic`.
- Perfil: `deep`.
- Semántica: completa para la compilación exitosa.
- Efectos: relaciones directas y propagación por SCC sobre llamadas internas y modifiers resueltos.
- Activos: clasificación tipada/simbólica con guards, reachability y tratamiento del retorno.
- Superficies: observaciones respaldadas por evidencia, sin severidad.
- Assembly: presencia y operaciones Yul seleccionadas; argumentos simbólicos.

## Resumen observado

| Entidad | Cantidad |
|---|---:|
| Fuentes | 1 |
| Contratos/interfaces/librerías | 10 |
| Funciones | 57 |
| Modifiers | 1 |
| Variables, parámetros y locales | 134 |
| Llamadas | 42 |
| Accesos de estado | 51 |
| Aliases storage | 0 |
| Comprobaciones | 29 |
| Operaciones de activos | 26 |
| Cambios de configuración | 7 |
| Operaciones assembly | 9 |
| Superficies de revisión | 48 |
| Resúmenes de funciones | 57 |
| Instrucciones IR | 178 |

## Entradas relevantes

- `Assets.sol:AssetsSurface.checkedTransfer(address,uint256)` — retorno ERC-20 propagado.
- `Assets.sol:AssetsSurface.sendNative(address,uint256)` — `send`, `transfer` y `call{value}`.
- `Assets.sol:AssetsSurface.vaultRoundTrip(uint256,uint256,address)` — assets y shares separados.
- `Assets.sol:AssetsSurface.upgradeTo(address)` — cambio de configuración proxy con actor simbólico.
- `Assets.sol:AssetsSurface.assemblySurface(bytes32,address,uint256)` — operaciones Yul enumeradas.
- `Assets.sol:AssetsSurface.receive()` — entrada de valor nativo.

## Relaciones explicables

### Movimiento ERC-20 comprobado

```text
AssetsSurface.requiredTransfer(address,uint256)
  ├─ onlyOwner → require(msg.sender == owner)
  └─ require(erc20.transfer(to, amount))
```

Clasificación: `movement / erc20 / transfer`, unidad `token_units_unknown`, retorno `required`, resolución `partial`. La interfaz tipada permite clasificar la intención, pero no demuestra qué bytecode existe en la dirección de `erc20`.

### ERC-4626

```text
AssetsSurface.vaultRoundTrip(uint256,uint256,address)
  ├─ vault.deposit(assets, receiver)  [unidad: assets]
  └─ vault.redeem(shares, receiver, msg.sender)  [unidad: shares]
```

Ambas operaciones conservan sus cantidades simbólicas y no asumen decimales ni conversión monetaria.

### Autorización separada

`AssetsSurface.authorize(address,uint256)` produce `authorization / erc20 / approve`. No se contabiliza como transferencia.

### Valor nativo

- `receive` y `wrapAndUnwrap`: entradas `payable_entry`.
- `sendNative`: `send`, `transfer` y `call{value: amount}`, con retorno comprobado cuando aplica.
- `deployWithValue`: creación de contrato con valor.
- `destroy`: `selfdestruct` con limitación explícita sobre reglas EVM/EIP-6780.

## Configuración y assembly

- Se enumeraron propietario, tesorería, oráculo, fee, pausa e implementación; sus identidades siguen siendo simbólicas.
- Se observaron `sload`, `sstore`, `tload`, `tstore`, `call`, `staticcall`, `create2`, `log0` y `switch`.

## Observaciones, no vulnerabilidades

- La llamada `delegatecall` es una superficie de revisión; el destino y los efectos permanecen parciales.
- La transferencia ERC-20 sin retorno comprobado genera una observación, no una vulnerabilidad confirmada.
- Bucles, unchecked, división, conversión estrecha, oráculo y escritura posterior a interacción conservan evidencia y límites.
- La función personalizada `custom.transfer` no se clasifica como ERC-20.
- El wrapper local `SafeERC20Surface.safeTransfer` se relaciona con una única operación subyacente alcanzable desde `wrappedTransfer`; no se duplica.
- Una operación posible no demuestra que se ejecutó ni que terminó sin revert.

## Limitaciones de esta ejecución

Hay resúmenes interprocedimentales por SCC para llamadas internas y modifiers resueltos, con aliases/bindings `storage` cuando existe evidencia tipada. No hay CFG completo, orden exacto de placeholders, estado on-chain ni verificación de implementaciones externas. El informe no concluye que el contrato sea seguro o vulnerable.
