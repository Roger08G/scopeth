package ast

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	analyzer "github.com/Roger08G/scopeth/internal/analysis"
	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
)

func TestRealSolcFixture(t *testing.T) {
	solc := os.Getenv("SCOPETH_TEST_SOLC")
	if solc == "" {
		t.Skip("SCOPETH_TEST_SOLC no configurado")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "solidity"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := input.Discover(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile(context.Background(), snapshot, compiler.Options{
		Mode: "semantic", SolcPath: solc, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := Normalize(snapshot, result.Output, Options{Profile: "deep", Mode: result.ModeUsed})
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Contracts) != 3 {
		t.Fatalf("se esperaban 3 contratos/interfaces, llegaron %d", len(program.Contracts))
	}
	var transfer, approve, writeBalances, eventTopic, errorSelector bool
	for _, operation := range program.AssetOperations {
		transfer = transfer || (operation.AssetClass == "erc20" && operation.Operation == "transfer")
		approve = approve || (operation.Category == "authorization" && operation.Operation == "approve")
	}
	for _, access := range program.StateAccesses {
		writeBalances = writeBalances || (access.Variable == "balances" && (access.Access == "write" || access.Access == "read_write"))
	}
	for _, event := range program.Events {
		eventTopic = eventTopic || (event.Name == "Deposited" && len(event.Topic0) == 66)
	}
	for _, customError := range program.Errors {
		errorSelector = errorSelector || (customError.Name == "InsufficientBalance" && len(customError.Selector) == 10)
	}
	if !transfer || !approve || !writeBalances || !eventTopic || !errorSelector {
		t.Fatalf("relaciones esperadas ausentes: transfer=%v approve=%v writeBalances=%v topic=%v errorSelector=%v", transfer, approve, writeBalances, eventTopic, errorSelector)
	}
}

func TestRealSolcAdvancedFixture(t *testing.T) {
	solc := os.Getenv("SCOPETH_TEST_SOLC")
	if solc == "" {
		t.Skip("SCOPETH_TEST_SOLC no configurado")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "advanced"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := input.Discover(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile(context.Background(), snapshot, compiler.Options{
		Mode: "semantic", SolcPath: solc, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := Normalize(snapshot, result.Output, Options{Profile: "deep", Mode: result.ModeUsed})
	if err != nil {
		t.Fatal(err)
	}
	analyzer.Enrich(&program)
	var erc721, erc1155, authorization, storageAlias, aliasBalance, aliasNonce, arrayMutation, modifierStorageCall, staticcall, superInternal, thisExternal, transientStorage bool
	for _, operation := range program.AssetOperations {
		erc721 = erc721 || operation.AssetClass == "erc721"
		erc1155 = erc1155 || operation.AssetClass == "erc1155"
		authorization = authorization || operation.Category == "authorization"
	}
	for _, alias := range program.StorageAliases {
		storageAlias = storageAlias || (alias.Alias == "account" && alias.RootKind == "state" && alias.TargetVariable == "accounts" && alias.Resolution == "resolved")
	}
	for _, access := range program.StateAccesses {
		if access.Function == "Advanced.sol:Advanced.updateThroughAlias(uint128)" && access.ViaAlias {
			aliasBalance = aliasBalance || (access.Path == "accounts[msg.sender].balance" && access.Access == "read_write")
			aliasNonce = aliasNonce || (access.Path == "accounts[msg.sender].nonce" && access.Access == "read_write")
		}
		arrayMutation = arrayMutation || (access.Function == "Advanced.sol:Advanced.mutateArray(uint256)" && access.Path == "queue" && access.Access == "read_write")
	}
	for _, call := range program.Calls {
		staticcall = staticcall || call.Callee == "staticcall"
		superInternal = superInternal || (strings.Contains(call.CallerSymbol, ".hook(") && call.Kind == "internal" && call.Resolution == "resolved")
		thisExternal = thisExternal || (strings.Contains(call.CallerSymbol, ".callThis(") && call.Kind == "external" && call.Resolution == "partial")
		modifierStorageCall = modifierStorageCall || (strings.Contains(call.CallerSymbol, ".modifierThroughStorageBinding(") && call.Kind == "modifier" && len(call.StorageBindings) == 1 && call.StorageBindings[0].ArgumentPath == "accounts[msg.sender]")
	}
	for _, variable := range program.Variables {
		transientStorage = transientStorage || (variable.Name == "transientCounter" && variable.StorageLocation == "transient")
	}
	var chainedBalance, chainedNonce, recursiveCaller, recursiveSCC, modifierRead, modifierWrite, modifierStorageEffect, modifierLinked bool
	for _, summary := range program.Summaries {
		for _, effect := range summary.TransitiveStorageEffects {
			if summary.Function == "Advanced.sol:Advanced.updateThroughInternalAlias(uint128)" {
				chainedBalance = chainedBalance || (effect.Path == "accounts[msg.sender].balance" && effect.Access == "read_write")
				chainedNonce = chainedNonce || (effect.Path == "accounts[msg.sender].nonce" && effect.Access == "read_write")
			}
			if summary.Function == "Advanced.sol:Advanced.updateThroughRecursiveAlias(uint128,uint256)" {
				recursiveCaller = recursiveCaller || (effect.Path == "accounts[msg.sender].balance" && effect.Access == "read_write")
			}
			if summary.Function == "Advanced.sol:Advanced.repeatedEffect(uint128)" {
				modifierRead = modifierRead || (effect.Path == "paused" && (effect.Access == "read" || effect.Access == "read_write"))
				modifierWrite = modifierWrite || (effect.Path == "accounts[msg.sender].balance" && (effect.Access == "write" || effect.Access == "read_write"))
			}
			if summary.Function == "Advanced.sol:Advanced.modifierThroughStorageBinding()" {
				modifierStorageEffect = modifierStorageEffect || (effect.Path == "accounts[msg.sender].nonce" && effect.Access == "read_write")
			}
		}
		recursiveSCC = recursiveSCC || (strings.Contains(summary.Function, "._recursiveApply(") && summary.Recursive && summary.Converged)
	}
	for _, function := range program.Functions {
		modifierLinked = modifierLinked || (function.Symbol == "Advanced.sol:Advanced.repeatedEffect(uint128)" && len(function.ModifierIDs) == 1)
	}
	if !erc721 || !erc1155 || !authorization || !storageAlias || !aliasBalance || !aliasNonce || !arrayMutation || !chainedBalance || !chainedNonce || !recursiveCaller || !recursiveSCC || !modifierRead || !modifierWrite || !modifierStorageCall || !modifierStorageEffect || !modifierLinked || !staticcall || !superInternal || !thisExternal || !transientStorage {
		t.Fatalf("cobertura avanzada ausente: erc721=%v erc1155=%v auth=%v alias=%v directBalance=%v directNonce=%v arrayMutation=%v chainedBalance=%v chainedNonce=%v recursiveCaller=%v recursiveSCC=%v modifierRead=%v modifierWrite=%v modifierStorageCall=%v modifierStorageEffect=%v modifierLinked=%v staticcall=%v superInternal=%v thisExternal=%v transient=%v", erc721, erc1155, authorization, storageAlias, aliasBalance, aliasNonce, arrayMutation, chainedBalance, chainedNonce, recursiveCaller, recursiveSCC, modifierRead, modifierWrite, modifierStorageCall, modifierStorageEffect, modifierLinked, staticcall, superInternal, thisExternal, transientStorage)
	}
}

func TestRealSolcF4Fixture(t *testing.T) {
	solc := os.Getenv("SCOPETH_TEST_SOLC")
	if solc == "" {
		t.Skip("SCOPETH_TEST_SOLC no configurado")
	}
	fixture, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "f4"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := input.Discover(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile(context.Background(), snapshot, compiler.Options{
		Mode: "semantic", SolcPath: solc, Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := Normalize(snapshot, result.Output, Options{Profile: "deep", Mode: result.ModeUsed})
	if err != nil {
		t.Fatal(err)
	}
	analyzer.Enrich(&program)

	operations := make(map[string]bool)
	var propagated, required, customMisclassified, related, loopCardinality bool
	wrapperUnderlying := 0
	for _, operation := range program.AssetOperations {
		operations[operation.Operation] = true
		propagated = propagated || (strings.Contains(operation.Function, ".checkedTransfer(") && operation.ReturnHandling == "propagated")
		required = required || (strings.Contains(operation.Function, ".requiredTransfer(") && operation.ReturnHandling == "required")
		customMisclassified = customMisclassified || strings.Contains(operation.Function, ".customNamedTransfer(")
		related = related || (operation.Operation == "transfer" && len(operation.ClassificationBasis) > 0 && len(operation.Dependencies) > 0 && len(operation.ReachableFrom) > 0)
		loopCardinality = loopCardinality || (strings.Contains(operation.Function, ".batchTokens(") && operation.Cardinality == "loop_dependent_unknown")
		if operation.Operation == "transfer" && containsString(operation.ReachableFrom, "Assets.sol:AssetsSurface.wrappedTransfer(address,uint256)") {
			wrapperUnderlying++
		}
	}
	for _, expected := range []string{"payable_entry", "transfer", "transferFrom", "approve", "permit", "safeTransferFrom", "safeBatchTransferFrom", "deposit", "redeem", "wrap", "unwrap", "native_send", "native_transfer", "call_with_value", "create_with_value", "selfdestruct", "state_accounting_update"} {
		if !operations[expected] {
			t.Errorf("operación F4 ausente: %s", expected)
		}
	}
	if !propagated || !required || customMisclassified || !related || !loopCardinality || wrapperUnderlying != 1 {
		t.Errorf("clasificación/relaciones F4 incorrectas: propagated=%v required=%v custom=%v related=%v loop=%v wrapper=%d", propagated, required, customMisclassified, related, loopCardinality, wrapperUnderlying)
	}

	configuration := make(map[string]bool)
	for _, change := range program.ConfigurationChanges {
		configuration[change.Category] = true
		if change.ActorModel == "" || change.Resolution == "" || change.Span == nil {
			t.Errorf("cambio de configuración incompleto: %#v", change)
		}
	}
	for _, expected := range []string{"access_control", "recipient", "oracle", "fee", "pause", "proxy"} {
		if !configuration[expected] {
			t.Errorf("categoría de configuración ausente: %s", expected)
		}
	}

	opcodes := make(map[string]bool)
	for _, operation := range program.AssemblyOperations {
		opcodes[operation.Opcode] = true
	}
	for _, expected := range []string{"sload", "sstore", "tload", "tstore", "call", "staticcall", "create2", "log0", "switch"} {
		if !opcodes[expected] {
			t.Errorf("operación Yul ausente: %s", expected)
		}
	}

	rules := make(map[string]bool)
	for _, observation := range program.Observations {
		rules[observation.Rule] = true
		if observation.Resolution == "" || len(observation.EvidenceIDs) == 0 {
			t.Errorf("observación sin trazabilidad: %#v", observation)
		}
	}
	for _, expected := range []string{"unchecked-asset-return", "state-write-after-interaction", "delegatecall-context", "data-dependent-loop", "unchecked-arithmetic", "division-rounding", "narrowing-conversion", "oracle-data-dependency", "proxy-upgrade-entrypoint", "inline-assembly"} {
		if !rules[expected] {
			t.Errorf("superficie de revisión ausente: %s", expected)
		}
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
