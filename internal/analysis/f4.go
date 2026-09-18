package analysis

import (
	"sort"
	"strings"

	"github.com/Roger08G/scopeth/internal/model"
)

func enrichF4(program *model.Program, summaries map[string]*mutableSummary) {
	reachable := entrypointReachability(program, summaries)
	enrichAssetRelations(program, summaries)
	detectConfigurationChanges(program, summaries, reachable)
	detectPostInteractionWrites(program)
	detectProxyIndicators(program)
	detectUncheckedAssetReturns(program)
	detectOracleDependencies(program)
	detectGuardSurfaces(program)
	for index := range program.Observations {
		observation := &program.Observations[index]
		observation.ReachableFrom = sorted(reachable[observation.FunctionID])
	}
	sort.Slice(program.AssetOperations, func(i, j int) bool { return program.AssetOperations[i].ID < program.AssetOperations[j].ID })
	sort.Slice(program.ConfigurationChanges, func(i, j int) bool { return program.ConfigurationChanges[i].ID < program.ConfigurationChanges[j].ID })
	sort.Slice(program.AssemblyOperations, func(i, j int) bool { return program.AssemblyOperations[i].ID < program.AssemblyOperations[j].ID })
	sort.Slice(program.Observations, func(i, j int) bool { return program.Observations[i].ID < program.Observations[j].ID })
	sort.Slice(program.Evidence, func(i, j int) bool { return program.Evidence[i].ID < program.Evidence[j].ID })
}

func detectGuardSurfaces(program *model.Program) {
	seen := observationIDs(program.Observations)
	for _, guard := range program.Guards {
		rule, title := "", ""
		switch guard.Category {
		case "signature":
			rule, title = "signature-guard", "Comprobación de firma"
		case "nonce":
			rule, title = "nonce-guard", "Comprobación de nonce"
		case "time":
			rule, title = "temporal-guard", "Comprobación temporal o deadline"
		case "bounds":
			rule, title = "bounds-guard", "Comprobación de límites o slippage"
		case "pause":
			rule, title = "pause-guard", "Comprobación de pausa"
		case "initialization":
			rule, title = "initialization-guard", "Comprobación de inicialización"
		default:
			continue
		}
		id := hashID("observation", guard.FunctionID, rule, guard.ID)
		if _, exists := seen[id]; exists {
			continue
		}
		evidenceID := hashID("evidence", id)
		program.Evidence = append(program.Evidence, model.Evidence{
			ID: evidenceID, Kind: "derived", Resolution: guard.Resolution,
			Description: "Guard clasificado por sus dependencias y expresión simbólica: " + guard.Expression, Span: guard.Span,
			Limitations: []string{"La cobertura se limita a la ruta modelada y no demuestra dominancia sobre todas las rutas."},
		})
		program.Observations = append(program.Observations, model.Observation{
			ID: id, Rule: rule, Category: "authorization", Title: title,
			Description: "La función contiene una comprobación relevante para autorización o condiciones de ejecución.",
			FunctionID:  guard.FunctionID, Function: guard.Function, Resolution: guard.Resolution, Span: guard.Span,
			EvidenceIDs: []string{evidenceID},
			Assumptions: []string{"La categoría deriva de la expresión del guard y sus dependencias."},
			Limitations: []string{"No prueba que el guard cubra todas las rutas hacia un efecto."},
		})
		seen[id] = struct{}{}
	}
}

func entrypointReachability(program *model.Program, summaries map[string]*mutableSummary) map[string]map[string]struct{} {
	entrypoints := make([]model.Function, 0)
	for _, function := range program.Functions {
		if function.Visibility == "external" || function.Visibility == "public" || function.Kind == "receive" || function.Kind == "fallback" {
			entrypoints = append(entrypoints, function)
		}
	}
	sort.Slice(entrypoints, func(i, j int) bool { return entrypoints[i].Symbol < entrypoints[j].Symbol })
	reachable := make(map[string]map[string]struct{})
	for _, entry := range entrypoints {
		visited := set()
		frontier := []string{entry.ID}
		for len(frontier) > 0 {
			current := frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
			if _, seen := visited[current]; seen {
				continue
			}
			visited[current] = struct{}{}
			if reachable[current] == nil {
				reachable[current] = set()
			}
			reachable[current][entry.Symbol] = struct{}{}
			if summary := summaries[current]; summary != nil {
				for callee := range summary.callees {
					frontier = append(frontier, callee)
				}
			}
		}
	}
	return reachable
}

func addAccountingOperations(program *model.Program) {
	seen := make(map[string]struct{})
	for _, operation := range program.AssetOperations {
		for _, stateID := range operation.RelatedState {
			seen[stateID] = struct{}{}
		}
	}
	for _, access := range program.StateAccesses {
		if !access.Direct || access.RootKind != "state" || (access.Access != "write" && access.Access != "read_write") {
			continue
		}
		assetClass, ok := accountingClass(access.Path)
		if !ok {
			continue
		}
		if _, exists := seen[access.ID]; exists {
			continue
		}
		evidenceID := hashID("evidence", "accounting", access.ID)
		program.Evidence = append(program.Evidence, model.Evidence{
			ID: evidenceID, Kind: "derived", Resolution: access.Resolution,
			Description: "Escritura de estado clasificada como modificación potencial de contabilidad interna.", Span: access.Span,
			Assumptions: []string{"La clasificación se apoya en el nombre y tipo de la raíz de estado, no en saldos on-chain."},
			Limitations: []string{"No equivale a un movimiento de activos ni demuestra el significado económico de la variable."},
		})
		path := access.Path
		if path == "" {
			path = access.Variable
		}
		program.AssetOperations = append(program.AssetOperations, model.AssetOperation{
			ID: hashID("asset", "accounting", access.ID), FunctionID: access.FunctionID, Function: access.Function,
			Category: "accounting", AssetClass: assetClass, Operation: "state_accounting_update",
			AssetExpression: path, FromExpression: "previous_state", ToExpression: "updated_state",
			AmountExpression: "symbolic_state_delta", Unit: "internal_units_unknown",
			ClassificationBasis: []string{"state_path=" + path, "access=" + access.Access},
			Dependencies:        []string{path}, ReturnHandling: "not_applicable", SuccessCondition: "transaction_does_not_revert",
			Cardinality: "single", Resolution: worseResolution(access.Resolution, "partial"), Span: access.Span,
			EvidenceIDs: []string{evidenceID}, RelatedState: []string{access.ID},
			Limitations: []string{"Contabilidad interna se mantiene separada de movimiento y autorización de activos."},
		})
		seen[access.ID] = struct{}{}
	}
}

func accountingClass(path string) (string, bool) {
	value := strings.ToLower(path)
	switch {
	case strings.Contains(value, "allowance"):
		return "allowance_ledger", true
	case strings.Contains(value, "share"):
		return "share_ledger", true
	case strings.Contains(value, "supply"):
		return "supply_ledger", true
	case strings.Contains(value, "balance") || strings.Contains(value, "credit") || strings.Contains(value, "debt") || strings.Contains(value, "reserve"):
		return "balance_ledger", true
	default:
		return "", false
	}
}

func detectConfigurationChanges(program *model.Program, summaries map[string]*mutableSummary, reachable map[string]map[string]struct{}) {
	functions := make(map[string]model.Function, len(program.Functions))
	for _, function := range program.Functions {
		functions[function.ID] = function
	}
	for _, access := range program.StateAccesses {
		if !access.Direct || access.RootKind != "state" || (access.Access != "write" && access.Access != "read_write") {
			continue
		}
		category, ok := configurationCategory(access.Path)
		if !ok {
			continue
		}
		guardIDs := []string(nil)
		if summary := summaries[access.FunctionID]; summary != nil {
			guardIDs = sorted(summary.guards)
		}
		actorModel := "reachable_entrypoints"
		if function, exists := functions[access.FunctionID]; exists {
			switch function.Kind {
			case "constructor":
				actorModel = "deployment"
			default:
				if function.Visibility == "public" || function.Visibility == "external" {
					actorModel = "any_external_caller"
					if len(guardIDs) > 0 {
						actorModel = "external_caller_subject_to_symbolic_guards"
					}
				}
			}
		} else if strings.Contains(access.Function, ".modifier ") {
			actorModel = "modifier_context"
		}
		path := access.Path
		if path == "" {
			path = access.Variable
		}
		evidenceID := hashID("evidence", "configuration", access.ID, category)
		program.Evidence = append(program.Evidence, model.Evidence{
			ID: evidenceID, Kind: "derived", Resolution: access.Resolution,
			Description: "Escritura sobre una raíz de configuración sensible clasificada por su identidad simbólica.", Span: access.Span,
			Limitations: []string{"El actor es simbólico y la cobertura de guards no se prueba sobre un CFG completo."},
		})
		program.ConfigurationChanges = append(program.ConfigurationChanges, model.ConfigurationChange{
			ID: hashID("configuration", access.ID, category), FunctionID: access.FunctionID, Function: access.Function,
			VariableID: access.VariableID, Variable: access.Variable, Path: path, Category: category,
			ActorModel: actorModel, GuardIDs: guardIDs, ReachableFrom: sorted(reachable[access.FunctionID]),
			Resolution: worseResolution(access.Resolution, "partial"), Span: access.Span, EvidenceIDs: []string{evidenceID},
			Limitations: []string{"No se recupera la identidad on-chain del propietario, rol, administrador u oráculo."},
		})
	}
}

func configurationCategory(path string) (string, bool) {
	value := strings.ToLower(path)
	switch {
	case strings.Contains(value, "implementation") || strings.Contains(value, "admin") || strings.Contains(value, "beacon"):
		return "proxy", true
	case strings.Contains(value, "owner") || strings.Contains(value, "role") || strings.Contains(value, "guardian"):
		return "access_control", true
	case strings.Contains(value, "treasury") || strings.Contains(value, "recipient") || strings.Contains(value, "beneficiary"):
		return "recipient", true
	case strings.Contains(value, "oracle") || strings.Contains(value, "feed"):
		return "oracle", true
	case strings.Contains(value, "fee"):
		return "fee", true
	case strings.Contains(value, "limit") || strings.Contains(value, "cap"):
		return "limit", true
	case strings.Contains(value, "pause"):
		return "pause", true
	case strings.Contains(value, "initializ"):
		return "initialization", true
	case strings.Contains(value, "asset") || strings.Contains(value, "token"):
		return "asset", true
	default:
		return "", false
	}
}

func detectPostInteractionWrites(program *model.Program) {
	seen := observationIDs(program.Observations)
	for _, call := range program.Calls {
		if call.Kind != "external" && call.Kind != "low_level" && call.Kind != "dynamic" || call.Span == nil {
			continue
		}
		for _, access := range program.StateAccesses {
			if !access.Direct || access.FunctionID != call.CallerID || access.Span == nil || (access.Access != "write" && access.Access != "read_write") {
				continue
			}
			if access.Span.LogicalPath != call.Span.LogicalPath || access.Span.Offset <= call.Span.Offset {
				continue
			}
			id := hashID("observation", call.CallerID, "state-write-after-interaction", call.ID, access.ID)
			if _, exists := seen[id]; exists {
				continue
			}
			evidenceID := hashID("evidence", id)
			program.Evidence = append(program.Evidence, model.Evidence{
				ID: evidenceID, Kind: "derived", Resolution: "partial",
				Description: "Una escritura aparece después de una interacción externa en el orden de fuente de la misma función.", Span: access.Span,
				Assumptions: []string{"Los offsets de fuente aproximan el orden local, no la factibilidad de una ruta."},
				Limitations: []string{"No constituye una confirmación de reentrancia ni prueba que ambas operaciones compartan ruta."},
			})
			program.Observations = append(program.Observations, model.Observation{
				ID: id, Rule: "state-write-after-interaction", Category: "external_interaction",
				Title: "Escritura posterior a interacción", Description: "Se observa una escritura de estado posterior a una llamada externa en el orden de fuente.",
				FunctionID: call.CallerID, Function: call.CallerSymbol, Resolution: "partial", Span: access.Span,
				EvidenceIDs: []string{evidenceID}, RelatedCallIDs: []string{call.ID}, RelatedStateAccessIDs: []string{access.ID},
				Assumptions: []string{"La relación es intrafunción y basada en offsets."},
				Limitations: []string{"Requiere CFG para demostrar orden y alcanzabilidad en una misma ruta."},
			})
			seen[id] = struct{}{}
		}
	}
}

func detectProxyIndicators(program *model.Program) {
	seen := observationIDs(program.Observations)
	for _, variable := range program.Variables {
		value := strings.ToLower(variable.Name)
		if !strings.Contains(value, "implementation") && !strings.Contains(value, "admin_slot") && !strings.Contains(value, "beacon") && !strings.Contains(value, "proxiable") {
			continue
		}
		id := hashID("observation", "proxy-slot-indicator", variable.ID)
		if _, exists := seen[id]; exists {
			continue
		}
		evidenceID := hashID("evidence", id)
		program.Evidence = append(program.Evidence, model.Evidence{
			ID: evidenceID, Kind: "heuristic", Resolution: "partial", Description: "Nombre de variable compatible con configuración de proxy.", Span: variable.Span,
			Limitations: []string{"No certifica un slot EIP-1967 ni la implementación desplegada."},
		})
		program.Observations = append(program.Observations, model.Observation{
			ID: id, Rule: "proxy-slot-indicator", Category: "proxy", Title: "Indicio de configuración proxy",
			Description: "Una variable contiene una identidad simbólica asociada a implementación, admin o beacon.",
			Resolution:  "partial", Span: variable.Span, EvidenceIDs: []string{evidenceID},
			Limitations: []string{"La identidad real requiere storage on-chain y validación del layout."},
		})
		seen[id] = struct{}{}
	}
}

func detectUncheckedAssetReturns(program *model.Program) {
	seen := observationIDs(program.Observations)
	for _, operation := range program.AssetOperations {
		if operation.ReturnHandling != "unchecked_or_unknown" || !booleanReturnOperation(operation.Operation) {
			continue
		}
		id := hashID("observation", operation.FunctionID, "unchecked-asset-return", operation.ID)
		if _, exists := seen[id]; exists {
			continue
		}
		evidenceID := hashID("evidence", id)
		program.Evidence = append(program.Evidence, model.Evidence{
			ID: evidenceID, Kind: "derived", Resolution: "partial",
			Description: "La operación tiene una forma que puede devolver un indicador booleano y no se demostró su comprobación.", Span: operation.Span,
			Limitations: []string{"La ABI real del destino y wrappers externos pueden alterar la semántica."},
		})
		program.Observations = append(program.Observations, model.Observation{
			ID: id, Rule: "unchecked-asset-return", Category: "asset_handling", Title: "Retorno de operación no demostrado",
			Description: "No se demostró que el valor de retorno potencial de la operación sea comprobado o propagado.",
			FunctionID:  operation.FunctionID, Function: operation.Function, Resolution: "partial", Span: operation.Span,
			EvidenceIDs: []string{evidenceID}, RelatedCallIDs: append([]string(nil), operation.RelatedCallIDs...),
			Limitations: []string{"No implica que el token retorne false ni que la operación falle."},
		})
		seen[id] = struct{}{}
	}
}

func booleanReturnOperation(operation string) bool {
	switch operation {
	case "transfer", "transferFrom", "approve", "increaseAllowance", "decreaseAllowance", "native_send", "call_with_value":
		return true
	default:
		return false
	}
}

func detectOracleDependencies(program *model.Program) {
	seen := observationIDs(program.Observations)
	for _, call := range program.Calls {
		value := strings.ToLower(call.Callee)
		if !strings.Contains(value, "latestanswer") && !strings.Contains(value, "latestrounddata") && !strings.Contains(value, "getprice") && !strings.Contains(value, "oracle") {
			continue
		}
		id := hashID("observation", call.CallerID, "oracle-data-dependency", call.ID)
		if _, exists := seen[id]; exists {
			continue
		}
		evidenceID := hashID("evidence", id)
		program.Evidence = append(program.Evidence, model.Evidence{
			ID: evidenceID, Kind: "heuristic", Resolution: "partial", Description: "Llamada compatible con lectura de oráculo o precio.", Span: call.Span,
			Limitations: []string{"No se demuestra frescura, unidad, fuente ni manipulabilidad del dato."},
		})
		program.Observations = append(program.Observations, model.Observation{
			ID: id, Rule: "oracle-data-dependency", Category: "oracle", Title: "Dependencia potencial de oráculo",
			Description: "La llamada sugiere consumo de precio u oráculo y requiere revisar frescura y unidades.",
			FunctionID:  call.CallerID, Function: call.CallerSymbol, Resolution: "partial", Span: call.Span,
			EvidenceIDs: []string{evidenceID}, RelatedCallIDs: []string{call.ID},
			Limitations: []string{"La clasificación nominal no prueba que el retorno se use en una decisión económica."},
		})
		seen[id] = struct{}{}
	}
}

func observationIDs(observations []model.Observation) map[string]struct{} {
	result := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		result[observation.ID] = struct{}{}
	}
	return result
}
