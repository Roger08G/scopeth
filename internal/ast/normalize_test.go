package ast

import (
	"encoding/json"
	"testing"

	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
)

func TestNormalizeContractAndFunction(t *testing.T) {
	content := []byte("contract A { function f() external {} }")
	root := map[string]any{
		"id": 1, "nodeType": "SourceUnit", "src": "0:42:0",
		"nodes": []any{map[string]any{
			"id": 2, "nodeType": "ContractDefinition", "src": "0:42:0", "name": "A", "contractKind": "contract",
			"nodes": []any{map[string]any{
				"id": 3, "nodeType": "FunctionDefinition", "src": "13:25:0", "name": "f", "kind": "function",
				"visibility": "external", "stateMutability": "nonpayable", "implemented": true,
				"parameters":       map[string]any{"nodeType": "ParameterList", "parameters": []any{}},
				"returnParameters": map[string]any{"nodeType": "ParameterList", "parameters": []any{}},
				"body":             map[string]any{"id": 4, "nodeType": "Block", "src": "37:2:0", "statements": []any{}},
			}},
		}},
	}
	astJSON, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := input.Snapshot{Sources: []input.Source{{LogicalPath: "A.sol", Content: content, SHA256: "x"}}}
	output := compiler.Output{Sources: map[string]compiler.SourceOutput{"A.sol": {ID: 0, AST: astJSON}}}
	program, err := Normalize(snapshot, output, Options{Profile: "deep"})
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Contracts) != 1 || len(program.Functions) != 1 {
		t.Fatalf("inventario inesperado: contratos=%d funciones=%d", len(program.Contracts), len(program.Functions))
	}
	if program.Functions[0].Symbol != "A.sol:A.f()" {
		t.Fatalf("símbolo inesperado: %s", program.Functions[0].Symbol)
	}
}

func TestByteSpanWithUTF8AndCRLF(t *testing.T) {
	content := []byte("// ñ\r\ncontract A {}\r\n")
	n := &normalizer{sourceByID: map[int]input.Source{0: {LogicalPath: "A.sol", Content: content}}}
	span := n.span("7:8:0")
	if span.Line != 2 || span.Column != 1 {
		t.Fatalf("ubicación inesperada: %#v", span)
	}
}

func TestStableID(t *testing.T) {
	if stableID("a", "b") != stableID("a", "b") || stableID("a", "b") == stableID("ab") {
		t.Fatal("IDs no deterministas o ambiguos")
	}
}
