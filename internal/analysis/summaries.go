package analysis

import (
	"sort"

	"github.com/Roger08G/scopeth/internal/model"
)

const maxSCCIterations = 64

type mutableSummary struct {
	function       model.Function
	directEffects  effectSet
	effects        effectSet
	directCalls    map[string]struct{}
	allCalls       map[string]struct{}
	callSites      []model.CallSite
	callees        map[string]struct{}
	directAssets   map[string]struct{}
	assets         map[string]struct{}
	directGuards   map[string]struct{}
	guards         map[string]struct{}
	unknownEffects bool
	resolution     string
	iterations     int
	sccID          string
	recursive      bool
	converged      bool
}

// Enrich calcula efectos y resúmenes por punto fijo sobre componentes fuertemente
// conexas. Atraviesa llamadas internas y modifiers resueltos; el resto degrada la cobertura.
func Enrich(program *model.Program) {
	if program == nil {
		return
	}
	addAccountingOperations(program)
	summaries := initializeSummaries(program)
	analysisIDs := sortedKeys(summaries)
	adjacency := make(map[string][]string, len(analysisIDs))
	for _, functionID := range analysisIDs {
		adjacency[functionID] = sorted(summaries[functionID].callees)
	}
	components := stronglyConnectedComponents(analysisIDs, adjacency)
	componentOrder := calleeFirstComponentOrder(components, adjacency)

	for _, componentIndex := range componentOrder {
		component := components[componentIndex]
		sccID := hashID(append([]string{"scc"}, component...)...)
		recursive := len(component) > 1 || componentHasSelfLoop(component, adjacency)
		for _, functionID := range component {
			summary := summaries[functionID]
			summary.sccID = sccID
			summary.recursive = recursive
		}
		converged := iterateComponent(component, summaries)
		for _, functionID := range component {
			summary := summaries[functionID]
			summary.converged = converged
			if !converged {
				summary.unknownEffects = true
				summary.resolution = worseResolution(summary.resolution, "partial")
			}
		}
	}

	program.Summaries = program.Summaries[:0]
	functionIDs := make([]string, 0, len(program.Functions))
	for _, function := range program.Functions {
		functionIDs = append(functionIDs, function.ID)
	}
	sort.Strings(functionIDs)
	for _, functionID := range functionIDs {
		summary := summaries[functionID]
		directReads, directWrites := concreteVariableSets(summary.directEffects)
		reads, writes := concreteVariableSets(summary.effects)
		program.Summaries = append(program.Summaries, model.FunctionSummary{
			FunctionID: functionID, Function: summary.function.Symbol,
			DirectReadVariables: sorted(directReads), DirectWriteVariables: sorted(directWrites),
			TransitiveReadVariables: sorted(reads), TransitiveWriteVariables: sorted(writes),
			DirectCallIDs: sorted(summary.directCalls), TransitiveCallIDs: sorted(summary.allCalls),
			InternalCallees:       sorted(summary.callees),
			DirectAssetOperations: sorted(summary.directAssets), TransitiveAssetOperations: sorted(summary.assets),
			DirectGuardIDs: sorted(summary.directGuards), GuardIDs: sorted(summary.guards),
			DirectStorageEffects: summary.directEffects.values(), TransitiveStorageEffects: summary.effects.values(),
			UnknownEffects: summary.unknownEffects, Resolution: summary.resolution, Iterations: summary.iterations,
			SCCID: summary.sccID, Recursive: summary.recursive, Converged: summary.converged,
		})
	}
	addTransitiveStateAccesses(program)
	enrichF4(program, summaries)
	program.IR = buildIR(program)
}

func initializeSummaries(program *model.Program) map[string]*mutableSummary {
	summaries := make(map[string]*mutableSummary, len(program.Functions)+len(program.Modifiers))
	for _, function := range program.Functions {
		summaries[function.ID] = emptySummary(function)
	}
	for _, modifier := range program.Modifiers {
		if _, exists := summaries[modifier.ID]; exists {
			continue
		}
		summaries[modifier.ID] = emptySummary(model.Function{
			ID: modifier.ID, Symbol: modifierSummarySymbol(modifier), Name: modifier.Name,
			Kind: "modifier", ContractID: modifier.ContractID, ContractName: modifier.ContractName,
			LogicalPath: modifier.LogicalPath, Visibility: "internal", Implemented: true,
			Parameters: modifier.Parameters, CompilerID: modifier.CompilerID, Span: modifier.Span,
		})
	}
	for _, access := range program.StateAccesses {
		if !access.Direct {
			continue
		}
		summary := summaries[access.FunctionID]
		if summary == nil {
			continue
		}
		effect := storageEffect(access)
		summary.directEffects.add(effect)
		summary.effects.add(effect)
	}
	for _, call := range program.Calls {
		summary := summaries[call.CallerID]
		if summary == nil {
			continue
		}
		summary.directCalls[call.ID] = struct{}{}
		summary.allCalls[call.ID] = struct{}{}
		if (call.Kind == "internal" || call.Kind == "modifier") && call.Resolution == "resolved" {
			if _, ok := summaries[call.CalleeID]; ok {
				summary.callees[call.CalleeID] = struct{}{}
				summary.callSites = append(summary.callSites, call)
				continue
			}
		}
		if call.Kind == "external" || call.Kind == "low_level" || call.Kind == "dynamic" {
			summary.unknownEffects = true
			summary.resolution = worseResolution(summary.resolution, call.Resolution)
		}
	}
	for _, observation := range program.Observations {
		if summary := summaries[observation.FunctionID]; summary != nil && observation.Resolution != "resolved" {
			summary.unknownEffects = true
			summary.resolution = worseResolution(summary.resolution, observation.Resolution)
		}
	}
	for _, guard := range program.Guards {
		if summary := summaries[guard.FunctionID]; summary != nil {
			summary.directGuards[guard.ID] = struct{}{}
			summary.guards[guard.ID] = struct{}{}
		}
	}
	for _, operation := range program.AssetOperations {
		if summary := summaries[operation.FunctionID]; summary != nil {
			summary.directAssets[operation.ID] = struct{}{}
			summary.assets[operation.ID] = struct{}{}
		}
	}
	for _, summary := range summaries {
		sort.Slice(summary.callSites, func(i, j int) bool { return summary.callSites[i].ID < summary.callSites[j].ID })
	}
	return summaries
}

func emptySummary(function model.Function) *mutableSummary {
	return &mutableSummary{
		function: function, directEffects: newEffectSet(), effects: newEffectSet(),
		directCalls: set(), allCalls: set(), callees: set(), directAssets: set(), assets: set(),
		directGuards: set(), guards: set(), resolution: "resolved", converged: true,
	}
}

func modifierSummarySymbol(modifier model.Modifier) string {
	return modifier.LogicalPath + ":" + modifier.ContractName + ".modifier " + modifier.Name
}

func iterateComponent(component []string, summaries map[string]*mutableSummary) bool {
	for iteration := 1; iteration <= maxSCCIterations; iteration++ {
		changed := false
		for _, functionID := range component {
			summary := summaries[functionID]
			for _, call := range summary.callSites {
				callee := summaries[call.CalleeID]
				if callee == nil {
					continue
				}
				changed = mergeInstantiatedEffects(summary, callee, call) || changed
				changed = merge(summary.assets, callee.assets) || changed
				changed = merge(summary.guards, callee.guards) || changed
				changed = merge(summary.allCalls, callee.allCalls) || changed
				if callee.unknownEffects && !summary.unknownEffects {
					summary.unknownEffects, changed = true, true
				}
				resolution := worseResolution(summary.resolution, callee.resolution)
				if resolution != summary.resolution {
					summary.resolution, changed = resolution, true
				}
			}
			summary.iterations = iteration
		}
		if !changed {
			return true
		}
	}
	return false
}

func mergeInstantiatedEffects(caller, callee *mutableSummary, call model.CallSite) bool {
	changed := false
	for _, effect := range callee.effects.values() {
		instantiated, ok := instantiateEffect(effect, call.StorageBindings)
		if !ok {
			if !caller.unknownEffects {
				caller.unknownEffects, changed = true, true
			}
			resolution := worseResolution(caller.resolution, "unknown")
			if resolution != caller.resolution {
				caller.resolution, changed = resolution, true
			}
			continue
		}
		changed = caller.effects.add(instantiated) || changed
	}
	return changed
}

func addTransitiveStateAccesses(program *model.Program) {
	for _, summary := range program.Summaries {
		direct := effectSetFrom(summary.DirectStorageEffects)
		for _, effect := range summary.TransitiveStorageEffects {
			if effect.RootKind != "state" {
				continue
			}
			current, exists := direct.get(effect)
			missing := missingAccess(current, exists, effect.Access)
			if missing == "" {
				continue
			}
			appendTransitiveAccess(program, summary, effect, missing)
		}
	}
	sort.Slice(program.StateAccesses, func(i, j int) bool { return program.StateAccesses[i].ID < program.StateAccesses[j].ID })
	sort.Slice(program.Evidence, func(i, j int) bool { return program.Evidence[i].ID < program.Evidence[j].ID })
}

func appendTransitiveAccess(program *model.Program, summary model.FunctionSummary, effect model.StorageEffect, access string) {
	evidenceID := hashID("evidence", "transitive-state", summary.FunctionID, effect.VariableID, effect.Path, access)
	program.Evidence = append(program.Evidence, model.Evidence{
		ID: evidenceID, Kind: "derived", Resolution: effect.Resolution,
		Description: "Acceso transitivo propagado por llamadas internas o modifiers resueltos y bindings storage tipados.",
		Assumptions: []string{"Las referencias y bindings proceden del AST semántico de solc."},
		Limitations: []string{"No se propaga a través de llamadas externas, delegatecall ni destinos dinámicos."},
	})
	program.StateAccesses = append(program.StateAccesses, model.StateAccess{
		ID:         hashID("state", "transitive", summary.FunctionID, effect.VariableID, effect.Path, access),
		FunctionID: summary.FunctionID, Function: summary.Function, VariableID: effect.VariableID,
		Variable: effect.Variable, Access: access, Direct: false, RootKind: effect.RootKind,
		Path: effect.Path, ViaAlias: effect.ViaAlias, Resolution: effect.Resolution,
		EvidenceIDs: []string{evidenceID},
	})
}

func enrichAssetRelations(program *model.Program, summaries map[string]*mutableSummary) {
	reachable := entrypointReachability(program, summaries)
	guardsByID := make(map[string]model.Guard)
	stateByFunction := make(map[string][]string)
	for _, guard := range program.Guards {
		guardsByID[guard.ID] = guard
	}
	for _, access := range program.StateAccesses {
		stateByFunction[access.FunctionID] = append(stateByFunction[access.FunctionID], access.ID)
	}
	for index := range program.AssetOperations {
		operation := &program.AssetOperations[index]
		guardIDs := set()
		if summary := summaries[operation.FunctionID]; summary != nil {
			for guardID := range summary.guards {
				guard := guardsByID[guardID]
				if guard.FunctionID == operation.FunctionID && operation.Span != nil && guard.Span != nil && guard.Span.Offset > operation.Span.Offset {
					continue
				}
				guardIDs[guardID] = struct{}{}
			}
		}
		operation.ConditionedBy = sorted(guardIDs)
		operation.RelatedState = uniqueSorted(stateByFunction[operation.FunctionID])
		operation.ReachableFrom = sorted(reachable[operation.FunctionID])
	}
}

func componentHasSelfLoop(component []string, adjacency map[string][]string) bool {
	if len(component) != 1 {
		return false
	}
	for _, callee := range adjacency[component[0]] {
		if callee == component[0] {
			return true
		}
	}
	return false
}

func concreteVariableSets(effects effectSet) (map[string]struct{}, map[string]struct{}) {
	reads, writes := set(), set()
	for _, effect := range effects.values() {
		if effect.RootKind != "state" {
			continue
		}
		if effect.Access == "read" || effect.Access == "read_write" {
			reads[effect.VariableID] = struct{}{}
		}
		if effect.Access == "write" || effect.Access == "read_write" {
			writes[effect.VariableID] = struct{}{}
		}
	}
	return reads, writes
}

func sortedKeys(values map[string]*mutableSummary) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
