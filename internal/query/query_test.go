package query

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Roger08G/scopeth/internal/model"
	"github.com/Roger08G/scopeth/internal/protocol"
)

func TestCollectOptionalAllowsAdditiveResultFile(t *testing.T) {
	values, err := collectOptional[map[string]any](t.TempDir(), "storage-aliases.jsonl", func(map[string]any) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("se esperaba una colección vacía: %#v", values)
	}
}

func TestSurfaceAndConfigurationFilters(t *testing.T) {
	directory := t.TempDir()
	writeJSONL(t, directory, "observations.jsonl", []model.Observation{
		{ID: "a", Rule: "dynamic-call", Category: "external_interaction", Resolution: "unknown"},
		{ID: "b", Rule: "division-rounding", Category: "arithmetic", Resolution: "resolved"},
	})
	writeJSONL(t, directory, "configuration-changes.jsonl", []model.ConfigurationChange{
		{ID: "a", Category: "oracle", ReachableFrom: []string{"A.sol:A.setOracle(address)"}},
		{ID: "b", Category: "fee", ReachableFrom: []string{"A.sol:A.setFee(uint256)"}},
	})

	var buffer bytes.Buffer
	writer := protocol.NewWriter(&buffer, protocol.Request{RequestID: "query"})
	count, err := Surfaces(directory, map[string]any{"category": "arithmetic", "limit": float64(10)}, writer)
	if err != nil || count != 1 || !strings.Contains(buffer.String(), "division-rounding") || strings.Contains(buffer.String(), "dynamic-call") {
		t.Fatalf("filtro de superficies incorrecto: count=%d err=%v output=%s", count, err, buffer.String())
	}

	buffer.Reset()
	count, err = Configuration(directory, map[string]any{"reachable_from": "A.sol:A.setFee(uint256)", "limit": float64(10)}, writer)
	if err != nil || count != 1 || !strings.Contains(buffer.String(), `"category":"fee"`) || strings.Contains(buffer.String(), `"category":"oracle"`) {
		t.Fatalf("filtro de configuración incorrecto: count=%d err=%v output=%s", count, err, buffer.String())
	}
}

func TestGraphKindsAndRoot(t *testing.T) {
	directory := t.TempDir()
	writeJSONL(t, directory, "calls.jsonl", []model.CallSite{
		{ID: "1", CallerSymbol: "A.sol:A.entry()", Callee: "A.sol:A.inner()", Kind: "internal", Resolution: "resolved"},
		{ID: "2", CallerSymbol: "A.sol:A.inner()", Callee: "target.call", Kind: "low_level", Resolution: "partial"},
		{ID: "3", CallerSymbol: "A.sol:A.other()", Callee: "ignored", Kind: "external", Resolution: "partial"},
	})
	writeJSONL(t, directory, "edges.jsonl", []model.Edge{{ID: "i", Kind: "import", From: "A.sol", To: "B.sol", Resolution: "resolved"}})
	writeJSONL(t, directory, "state-accesses.jsonl", []model.StateAccess{{ID: "s", Function: "A.sol:A.entry()", Path: "balances[msg.sender]", Access: "write", Resolution: "resolved"}})
	writeJSONL(t, directory, "asset-operations.jsonl", []model.AssetOperation{{ID: "a", Function: "A.sol:A.entry()", Category: "movement", AssetClass: "erc20", Operation: "transfer", AssetExpression: "token", Resolution: "partial"}})

	for _, kind := range []string{"calls", "imports", "state", "assets"} {
		var buffer bytes.Buffer
		writer := protocol.NewWriter(&buffer, protocol.Request{RequestID: kind})
		options := map[string]any{"kind": kind, "format": "json", "depth": float64(4)}
		if kind == "calls" {
			options["root"] = "A.sol:A.entry()"
		}
		count, err := Graph(directory, options, writer)
		if err != nil || count == 0 {
			t.Fatalf("grafo %s incorrecto: count=%d err=%v", kind, count, err)
		}
		if kind == "calls" && strings.Contains(buffer.String(), "ignored") {
			t.Fatalf("el root incluyó una rama ajena: %s", buffer.String())
		}
	}
}

func TestPaginationRejectsNegativeAndFractionalValues(t *testing.T) {
	directory := t.TempDir()
	writeJSONL(t, directory, "observations.jsonl", []model.Observation{})
	for _, options := range []map[string]any{{"offset": float64(-1)}, {"limit": 1.5}, {"limit": float64(10_001)}} {
		var buffer bytes.Buffer
		writer := protocol.NewWriter(&buffer, protocol.Request{RequestID: "invalid"})
		if _, err := Surfaces(directory, options, writer); err == nil || !strings.Contains(err.Error(), "INPUT_INVALID") {
			t.Fatalf("se esperaba INPUT_INVALID para %#v: %v", options, err)
		}
	}
}

func writeJSONL[T any](t *testing.T, directory, name string, values []T) {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, name), buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
