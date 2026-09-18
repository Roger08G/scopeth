package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
	"github.com/Roger08G/scopeth/internal/model"
)

func TestPublishCreatesImmutableRun(t *testing.T) {
	started := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	snapshot := input.Snapshot{Kind: "directory", Sources: []input.Source{{LogicalPath: "A.sol", Content: []byte("contract A{}"), SHA256: "abc", Origin: "directory"}}}
	metadata := Metadata{ToolVersion: "test", EngineVersion: "test", Status: "complete", ModeRequested: "semantic", ModeUsed: "semantic", Profile: "deep", StartedAt: started, CompletedAt: started.Add(time.Second), OptionsHash: "options"}
	result, err := Publish(t.TempDir(), snapshot, compiler.Result{ModeUsed: "semantic", SolcVersion: "test"}, model.Program{}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "report.md")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"storage-aliases.jsonl", "configuration-changes.jsonl", "assembly-operations.jsonl"} {
		if _, err := os.Stat(filepath.Join(result.RunDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	analysis, ok := result.Manifest["analysis"].(map[string]any)
	if !ok || analysis["ruleset_version"] != "test" {
		t.Fatalf("manifiesto de análisis incompleto: %#v", result.Manifest)
	}
	results, ok := result.Manifest["results"].(map[string]string)
	if !ok || results["configuration-changes.jsonl"] == "" || results["report.md"] == "" {
		t.Fatalf("hashes de resultados incompletos: %#v", result.Manifest["results"])
	}
}

func TestOptionsHashDeterministic(t *testing.T) {
	a := OptionsHash(map[string]any{"b": 2, "a": 1})
	b := OptionsHash(map[string]any{"a": 1, "b": 2})
	if a != b {
		t.Fatal("hash de opciones no determinista")
	}
}

func TestSummaryDistinguishesSyntaxDegradation(t *testing.T) {
	summary := makeSummary(
		input.Snapshot{Sources: []input.Source{{LogicalPath: "A.sol"}}},
		compiler.Result{ModeUsed: "syntax", Status: "partial"},
		model.Program{Sources: []model.SourceUnit{{LogicalPath: "A.sol"}}},
		Metadata{Status: "partial", Profile: "deep"},
	)
	coverage := summary["coverage"].(map[string]any)
	if coverage["semantic"] != "unavailable" || coverage["assets"] != "syntactic_partial_without_typed_guarantee" {
		t.Fatalf("cobertura degradada incorrecta: %#v", coverage)
	}
	modules := summary["modules"].(map[string]string)
	if modules["abi"] != "unavailable" || modules["assets"] != "partial" || modules["reporting"] != "produced" {
		t.Fatalf("módulos degradados incorrectos: %#v", modules)
	}
	units := summary["compilation_units"].(map[string]int)
	if units["degraded"] != 1 {
		t.Fatalf("unidad degradada ausente: %#v", units)
	}
}
