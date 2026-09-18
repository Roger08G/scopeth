package analysis

import (
	"testing"

	"github.com/Roger08G/scopeth/internal/model"
)

func TestEnrichInstantiatesStorageAliasesAcrossCallChain(t *testing.T) {
	program := model.Program{
		Functions: []model.Function{
			{ID: "entry", Symbol: "A.sol:A.entry()", Visibility: "external"},
			{ID: "middle", Symbol: "A.sol:A.middle(Account)", Visibility: "internal"},
			{ID: "leaf", Symbol: "A.sol:A.leaf(Account)", Visibility: "internal"},
		},
		Calls: []model.CallSite{
			{ID: "entry-middle", CallerID: "entry", Kind: "internal", CalleeID: "middle", Resolution: "resolved", StorageBindings: []model.StorageBinding{{ParameterID: "middle-param", ArgumentRootKind: "state", ArgumentVariableID: "accounts", ArgumentVariable: "accounts", ArgumentPath: "accounts[msg.sender]", Resolution: "resolved", ViaAlias: true}}},
			{ID: "middle-leaf", CallerID: "middle", Kind: "internal", CalleeID: "leaf", Resolution: "resolved", StorageBindings: []model.StorageBinding{{ParameterID: "leaf-param", ArgumentRootKind: "storage_parameter", ArgumentVariableID: "middle-param", ArgumentVariable: "account", ArgumentPath: "account", Resolution: "resolved"}}},
		},
		StateAccesses: []model.StateAccess{{
			ID: "leaf-effect", FunctionID: "leaf", Function: "A.sol:A.leaf(Account)",
			VariableID: "leaf-param", Variable: "target", RootKind: "storage_parameter",
			Path: "target.balance", Access: "read_write", Direct: true, Resolution: "resolved",
		}},
	}
	Enrich(&program)
	entry := summaryByID(t, program.Summaries, "entry")
	if !entry.Converged || entry.Recursive {
		t.Fatalf("estado de punto fijo inesperado: %#v", entry)
	}
	assertEffect(t, entry.TransitiveStorageEffects, model.StorageEffect{
		RootKind: "state", VariableID: "accounts", Path: "accounts[msg.sender].balance", Access: "read_write",
	})
	var concrete bool
	for _, access := range program.StateAccesses {
		concrete = concrete || (access.FunctionID == "entry" && !access.Direct && access.Path == "accounts[msg.sender].balance" && access.Access == "read_write")
	}
	if !concrete {
		t.Fatalf("no se materializó el acceso transitivo: %#v", program.StateAccesses)
	}
}

func TestEnrichConvergesRecursiveSCC(t *testing.T) {
	program := model.Program{
		Functions: []model.Function{{ID: "a", Symbol: "A.a()"}, {ID: "b", Symbol: "A.b()"}},
		Calls: []model.CallSite{
			{ID: "a-b", CallerID: "a", Kind: "internal", CalleeID: "b", Resolution: "resolved"},
			{ID: "b-a", CallerID: "b", Kind: "internal", CalleeID: "a", Resolution: "resolved"},
		},
		StateAccesses: []model.StateAccess{{
			ID: "write", FunctionID: "b", VariableID: "counter", Variable: "counter", RootKind: "state",
			Path: "counter", Access: "write", Direct: true, Resolution: "resolved",
		}},
	}
	Enrich(&program)
	a, b := summaryByID(t, program.Summaries, "a"), summaryByID(t, program.Summaries, "b")
	if !a.Recursive || !b.Recursive || !a.Converged || !b.Converged || a.SCCID != b.SCCID {
		t.Fatalf("SCC recursiva incorrecta: a=%#v b=%#v", a, b)
	}
	if len(a.TransitiveWriteVariables) != 1 || a.TransitiveWriteVariables[0] != "counter" {
		t.Fatalf("efecto recursivo no propagado: %#v", a)
	}
}

func TestEnrichDegradesUnboundStorageParameter(t *testing.T) {
	program := model.Program{
		Functions: []model.Function{{ID: "entry", Symbol: "A.entry()"}, {ID: "leaf", Symbol: "A.leaf(S)"}},
		Calls:     []model.CallSite{{ID: "call", CallerID: "entry", Kind: "internal", CalleeID: "leaf", Resolution: "resolved"}},
		StateAccesses: []model.StateAccess{{
			ID: "effect", FunctionID: "leaf", VariableID: "parameter", Variable: "value", RootKind: "storage_parameter",
			Path: "value.x", Access: "write", Direct: true, Resolution: "resolved",
		}},
	}
	Enrich(&program)
	entry := summaryByID(t, program.Summaries, "entry")
	if !entry.UnknownEffects || entry.Resolution != "unknown" || len(entry.TransitiveStorageEffects) != 0 {
		t.Fatalf("binding ausente no degradado correctamente: %#v", entry)
	}
}

func TestEnrichPropagatesGuardsAssetsAndReachability(t *testing.T) {
	program := model.Program{
		Functions: []model.Function{
			{ID: "entry", Symbol: "A.sol:A.entry()", Visibility: "external"},
			{ID: "inner", Symbol: "A.sol:A.inner()", Visibility: "internal"},
		},
		Calls:           []model.CallSite{{ID: "call", CallerID: "entry", CallerSymbol: "A.sol:A.entry()", Kind: "internal", CalleeID: "inner", Resolution: "resolved"}},
		Guards:          []model.Guard{{ID: "guard", FunctionID: "inner", Function: "A.sol:A.inner()", Resolution: "partial"}},
		AssetOperations: []model.AssetOperation{{ID: "asset", FunctionID: "inner", Function: "A.sol:A.inner()", Resolution: "partial"}},
	}
	Enrich(&program)
	entry := summaryByID(t, program.Summaries, "entry")
	if len(entry.GuardIDs) != 1 || len(entry.TransitiveAssetOperations) != 1 || len(entry.TransitiveCallIDs) != 1 {
		t.Fatalf("relaciones transitivas incompletas: %#v", entry)
	}
	operation := program.AssetOperations[0]
	if len(operation.ReachableFrom) != 1 || operation.ReachableFrom[0] != "A.sol:A.entry()" {
		t.Fatalf("alcanzabilidad inesperada: %#v", operation)
	}
}

func TestEnrichPropagatesModifierEffectsWithoutPublishingPseudoSummary(t *testing.T) {
	program := model.Program{
		Functions: []model.Function{{ID: "entry", Symbol: "A.sol:A.entry()", Visibility: "external", ModifierIDs: []string{"only-ready"}}},
		Modifiers: []model.Modifier{{ID: "only-ready", Name: "onlyReady", ContractName: "A", LogicalPath: "A.sol"}},
		Calls: []model.CallSite{{
			ID: "modifier-call", CallerID: "entry", CallerSymbol: "A.sol:A.entry()", Kind: "modifier",
			CalleeID: "only-ready", Callee: "A.sol:A.modifier onlyReady", Resolution: "resolved",
		}},
		StateAccesses: []model.StateAccess{{
			ID: "ready-read", FunctionID: "only-ready", Function: "A.sol:A.modifier onlyReady",
			VariableID: "ready", Variable: "ready", RootKind: "state", Path: "ready",
			Access: "read", Direct: true, Resolution: "resolved",
		}},
		Guards: []model.Guard{{ID: "guard", FunctionID: "only-ready", Function: "A.sol:A.modifier onlyReady", Resolution: "resolved"}},
	}
	Enrich(&program)
	if len(program.Summaries) != 1 {
		t.Fatalf("los modifiers son nodos de análisis, no funciones publicadas: %#v", program.Summaries)
	}
	entry := summaryByID(t, program.Summaries, "entry")
	assertEffect(t, entry.TransitiveStorageEffects, model.StorageEffect{
		RootKind: "state", VariableID: "ready", Path: "ready", Access: "read",
	})
	if len(entry.GuardIDs) != 1 || len(entry.TransitiveCallIDs) != 1 {
		t.Fatalf("efectos del modifier no propagados: %#v", entry)
	}
}

func TestEffectSetMergesReadAndWrite(t *testing.T) {
	effects := newEffectSet()
	effects.add(model.StorageEffect{RootKind: "state", VariableID: "x", Path: "x", Access: "read", Resolution: "resolved"})
	effects.add(model.StorageEffect{RootKind: "state", VariableID: "x", Path: "x", Access: "write", Resolution: "partial", ViaAlias: true})
	values := effects.values()
	if len(values) != 1 || values[0].Access != "read_write" || values[0].Resolution != "partial" || !values[0].ViaAlias {
		t.Fatalf("fusión de efectos incorrecta: %#v", values)
	}
}

func summaryByID(t *testing.T, summaries []model.FunctionSummary, id string) model.FunctionSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.FunctionID == id {
			return summary
		}
	}
	t.Fatalf("resumen ausente: %s", id)
	return model.FunctionSummary{}
}

func assertEffect(t *testing.T, effects []model.StorageEffect, want model.StorageEffect) {
	t.Helper()
	for _, effect := range effects {
		if effect.RootKind == want.RootKind && effect.VariableID == want.VariableID && effect.Path == want.Path && effect.Access == want.Access {
			return
		}
	}
	t.Fatalf("efecto ausente %#v en %#v", want, effects)
}
