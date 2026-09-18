package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Roger08G/scopeth/internal/model"
)

type Context struct {
	RunID         string
	Status        string
	InputKind     string
	ModeRequested string
	ModeUsed      string
	Profile       string
	SolcVersion   string
	AnalysisOnly  bool
	FailedFiles   []string
	Limitations   []string
}

func Markdown(program model.Program, context Context) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Informe Scopeth `%s`\n\n", markdown(context.RunID))
	fmt.Fprintf(&out, "> Estado operativo: **%s**. Modo solicitado: `%s`; modo utilizado: `%s`; perfil: `%s`.\n\n",
		markdown(context.Status), markdown(context.ModeRequested), markdown(context.ModeUsed), markdown(context.Profile))
	out.WriteString("## Alcance y procedencia\n\n")
	fmt.Fprintf(&out, "- Entrada: `%s`.\n- Compilador: `%s`.\n- Fuentes analizadas: **%d**.\n",
		markdown(context.InputKind), markdown(context.SolcVersion), len(program.Sources))
	if context.AnalysisOnly {
		out.WriteString("- La identidad de compilación no coincide con la declarada por la entrada: resultado marcado como `analysis_only`.\n")
	}
	out.WriteString("\n## Cobertura y limitaciones\n\n")
	if context.ModeUsed == "semantic" {
		out.WriteString("- Inventario semántico basado en AST, ABI y storage layout de `solc`.\n")
	} else {
		out.WriteString("- Cobertura sintáctica degradada; no se atribuyen slots ni resolución semántica completa.\n")
	}
	for _, limitation := range context.Limitations {
		fmt.Fprintf(&out, "- %s\n", markdown(limitation))
	}
	if len(context.FailedFiles) > 0 {
		fmt.Fprintf(&out, "- Archivos fallidos: %s.\n", markdown(strings.Join(context.FailedFiles, ", ")))
	}

	out.WriteString("\n## Resumen\n\n")
	out.WriteString("| Entidad | Cantidad |\n|---|---:|\n")
	rows := [][2]any{
		{"Contratos", len(program.Contracts)}, {"Funciones", len(program.Functions)}, {"Modifiers", len(program.Modifiers)},
		{"Variables", len(program.Variables)}, {"Llamadas", len(program.Calls)}, {"Accesos de estado", len(program.StateAccesses)},
		{"Aliases storage", len(program.StorageAliases)},
		{"Comprobaciones", len(program.Guards)}, {"Operaciones de activos", len(program.AssetOperations)}, {"Cambios de configuración", len(program.ConfigurationChanges)},
		{"Operaciones assembly", len(program.AssemblyOperations)}, {"Superficies de revisión", len(program.Observations)},
		{"Resúmenes de funciones", len(program.Summaries)}, {"Instrucciones IR", len(program.IR)},
	}
	for _, row := range rows {
		fmt.Fprintf(&out, "| %s | %d |\n", row[0], row[1])
	}

	out.WriteString("\n## Contratos y entradas\n\n")
	for _, contract := range program.Contracts {
		fmt.Fprintf(&out, "- `%s:%s` (%s%s).\n", markdown(contract.LogicalPath), markdown(contract.Name), markdown(contract.Kind), abstractSuffix(contract.Abstract))
	}
	for _, function := range program.Functions {
		if function.Visibility == "public" || function.Visibility == "external" || function.Kind == "receive" || function.Kind == "fallback" {
			generated := ""
			if function.Generated {
				generated = ", getter generado"
			}
			fmt.Fprintf(&out, "- `%s` — %s, %s%s.\n", markdown(function.Symbol), markdown(function.Visibility), markdown(function.StateMutability), generated)
		}
	}
	if len(program.Modifiers) > 0 {
		out.WriteString("\n### Modifiers\n\n")
		for _, modifier := range program.Modifiers {
			fmt.Fprintf(&out, "- `%s:%s.%s` (%s).\n", markdown(modifier.LogicalPath), markdown(modifier.ContractName), markdown(modifier.Name), location(modifier.Span))
		}
	}

	out.WriteString("\n## Estado y dependencias\n\n")
	if len(program.StateAccesses) == 0 {
		out.WriteString("No se observaron accesos directos justificables para el perfil ejecutado.\n")
	} else {
		for _, access := range limitedState(program.StateAccesses, 100) {
			scope := "transitivo"
			if access.Direct {
				scope = "directo"
			}
			target := access.Path
			if target == "" {
				target = access.Variable
			}
			alias := ""
			if access.ViaAlias {
				alias = ", mediante alias"
			}
			fmt.Fprintf(&out, "- `%s` **%s** `%s` (%s%s, %s).\n", markdown(access.Function), markdown(access.Access), markdown(target), scope, alias, location(access.Span))
		}
	}
	if len(program.StorageAliases) > 0 {
		out.WriteString("\n### Aliases storage\n\n")
		for _, alias := range program.StorageAliases {
			fmt.Fprintf(&out, "- `%s`: `%s` → `%s` (`%s`).\n", markdown(alias.Function), markdown(alias.Alias), markdown(alias.Path), markdown(alias.Resolution))
		}
	}

	out.WriteString("\n## Operaciones de activos y autorizaciones\n\n")
	if len(program.AssetOperations) == 0 {
		out.WriteString("No se clasificaron operaciones con la evidencia disponible. Esto no significa ausencia de movimientos posibles.\n")
	} else {
		for _, operation := range program.AssetOperations {
			fmt.Fprintf(&out, "- `%s`: **%s/%s/%s**, activo `%s`, desde `%s`, hacia `%s`, cantidad `%s` (`%s`), retorno `%s`, resolución `%s`, entradas `%s`, guards=%d, estado=%d (%s).\n",
				markdown(operation.Function), markdown(operation.Category), markdown(operation.AssetClass), markdown(operation.Operation),
				markdown(operation.AssetExpression), markdown(operation.FromExpression), markdown(operation.ToExpression),
				markdown(operation.AmountExpression), markdown(operation.Unit), markdown(operation.ReturnHandling), markdown(operation.Resolution),
				markdown(strings.Join(operation.ReachableFrom, ", ")), len(operation.ConditionedBy), len(operation.RelatedState), location(operation.Span))
			if len(operation.ClassificationBasis) > 0 {
				fmt.Fprintf(&out, "  - Base: %s. Dependencias: %s. Callback potencial: `%t`; cardinalidad: `%s`.\n",
					markdown(strings.Join(operation.ClassificationBasis, "; ")), markdown(strings.Join(operation.Dependencies, ", ")), operation.CallbackPotential, markdown(operation.Cardinality))
			}
		}
	}

	out.WriteString("\n## Cambios de configuración\n\n")
	if len(program.ConfigurationChanges) == 0 {
		out.WriteString("No se clasificaron escrituras de configuración sensible con la evidencia disponible.\n")
	} else {
		for _, change := range program.ConfigurationChanges {
			fmt.Fprintf(&out, "- `%s` modifica `%s` (**%s**), actor `%s`, entradas `%s`, guards=%d, resolución `%s` (%s).\n",
				markdown(change.Function), markdown(change.Path), markdown(change.Category), markdown(change.ActorModel),
				markdown(strings.Join(change.ReachableFrom, ", ")), len(change.GuardIDs), markdown(change.Resolution), location(change.Span))
		}
	}

	out.WriteString("\n## Comprobaciones\n\n")
	if len(program.Guards) == 0 {
		out.WriteString("No se modelaron comprobaciones explícitas. No equivale a ausencia de control de acceso.\n")
	} else {
		for _, guard := range program.Guards {
			fmt.Fprintf(&out, "- `%s`: `%s`/%s → `%s` (cobertura `%s`, dependencias `%s`, resolución `%s`, %s).\n",
				markdown(guard.Function), markdown(guard.Kind), markdown(guard.Category), markdown(guard.Expression),
				markdown(guard.Coverage), markdown(strings.Join(guard.Dependencies, ", ")), markdown(guard.Resolution), location(guard.Span))
		}
	}

	out.WriteString("\n## Assembly\n\n")
	if len(program.AssemblyOperations) == 0 {
		out.WriteString("No se enumeraron operaciones Yul seleccionadas.\n")
	} else {
		for _, operation := range program.AssemblyOperations {
			fmt.Fprintf(&out, "- `%s`: `%s` (**%s**), argumentos `%s`, resolución `%s` (%s).\n",
				markdown(operation.Function), markdown(operation.Opcode), markdown(operation.Category),
				markdown(strings.Join(operation.Arguments, ", ")), markdown(operation.Resolution), location(operation.Span))
		}
	}

	out.WriteString("\n## Llamadas externas y superficies de revisión\n\n")
	for _, call := range program.Calls {
		if call.Kind == "external" || call.Kind == "low_level" || call.Resolution != "resolved" {
			fmt.Fprintf(&out, "- `%s` → `%s` (%s, `%s`).\n", markdown(call.CallerSymbol), markdown(call.Callee), markdown(call.Kind), markdown(call.Resolution))
		}
	}
	for _, observation := range program.Observations {
		fmt.Fprintf(&out, "- **%s** (`%s/%s`) en `%s`: %s Resolución `%s`; entradas `%s`; evidencia `%s` (%s).\n",
			markdown(observation.Title), markdown(observation.Category), markdown(observation.Rule), markdown(observation.Function),
			markdown(observation.Description), markdown(observation.Resolution), markdown(strings.Join(observation.ReachableFrom, ", ")),
			markdown(strings.Join(observation.EvidenceIDs, ", ")), location(observation.Span))
		if len(observation.Assumptions)+len(observation.Limitations) > 0 {
			fmt.Fprintf(&out, "  - Supuestos: %s. Límites: %s.\n", markdown(strings.Join(observation.Assumptions, "; ")), markdown(strings.Join(observation.Limitations, "; ")))
		}
	}

	out.WriteString("\n---\n\n")
	out.WriteString("Scopeth enumera relaciones estáticas posibles. No recupera ejecuciones on-chain, no confirma explotabilidad y no concluye que un contrato carezca de vulnerabilidades.\n")
	return out.String()
}

func limitedState(values []model.StateAccess, limit int) []model.StateAccess {
	copyValues := append([]model.StateAccess(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i].ID < copyValues[j].ID })
	if len(copyValues) > limit {
		return copyValues[:limit]
	}
	return copyValues
}

func location(span *model.Span) string {
	if span == nil {
		return "ubicación generada o no disponible"
	}
	return fmt.Sprintf("%s:%d:%d", markdown(span.LogicalPath), span.Line, span.Column)
}

func abstractSuffix(value bool) string {
	if value {
		return ", abstracto"
	}
	return ""
}

func markdown(value string) string {
	value = strings.ReplaceAll(value, "`", "\uff40")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
