package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
)

func TestCacheRoundTripAndCorruption(t *testing.T) {
	base := t.TempDir()
	snapshot := input.Snapshot{Sources: []input.Source{{LogicalPath: "A.sol", SHA256: "abc"}}}
	key := Key(snapshot, "0.8.37", "semantic", nil, "1.0.0")
	want := compiler.Result{Status: "complete", ModeUsed: "semantic", SolcVersion: "0.8.37"}
	if err := Save(base, key, want); err != nil {
		t.Fatal(err)
	}
	got, hit, err := Load(base, key)
	if err != nil || !hit || got.Status != want.Status || got.SolcVersion != want.SolcVersion {
		t.Fatalf("round trip falló: hit=%v result=%#v err=%v", hit, got, err)
	}
	path := filepath.Join(base, ".scopeth", "cache", "compilation", key+".json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(base, key); err == nil {
		t.Fatal("se esperaba detectar caché corrupta")
	}
}

func TestKeyIncludesLogicalPath(t *testing.T) {
	a := input.Snapshot{Sources: []input.Source{{LogicalPath: "A.sol", SHA256: "same"}}}
	b := input.Snapshot{Sources: []input.Source{{LogicalPath: "B.sol", SHA256: "same"}}}
	if Key(a, "0.8.37", "semantic", nil, "1.0.0") == Key(b, "0.8.37", "semantic", nil, "1.0.0") {
		t.Fatal("la clave debe conservar identidad de source unit")
	}
}

func TestKeyInvalidatesRulesetAndDependency(t *testing.T) {
	base := input.Snapshot{Sources: []input.Source{{LogicalPath: "A.sol", SHA256: "a"}, {LogicalPath: "Lib.sol", SHA256: "lib-v1"}}}
	changed := input.Snapshot{Sources: []input.Source{{LogicalPath: "A.sol", SHA256: "a"}, {LogicalPath: "Lib.sol", SHA256: "lib-v2"}}}
	if Key(base, "0.8.37", "semantic", nil, "1.0.0") == Key(changed, "0.8.37", "semantic", nil, "1.0.0") {
		t.Fatal("un cambio de dependencia debe invalidar conservadoramente la caché")
	}
	if Key(base, "0.8.37", "semantic", nil, "0.9.0") == Key(base, "0.8.37", "semantic", nil, "1.0.0") {
		t.Fatal("un cambio del ruleset debe invalidar la caché")
	}
}
