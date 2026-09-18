package input

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverDirectoryDeterministic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "B.sol"), []byte("contract B {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "A.sol"), []byte("contract A {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources) != 2 || snapshot.Sources[0].LogicalPath != "A.sol" {
		t.Fatalf("orden inesperado: %#v", snapshot.Sources)
	}
}

func TestDiscoverStandardJSONRejectsRemoteURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.json")
	data := []byte(`{"language":"Solidity","sources":{"A.sol":{"urls":["https://example.invalid/A.sol"]}}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(path); err == nil {
		t.Fatal("se esperaba rechazo de URL remota")
	}
}

func TestCleanLogicalRejectsTraversal(t *testing.T) {
	if _, err := cleanLogical("../secret.sol"); err == nil {
		t.Fatal("se esperaba rechazo de traversal")
	}
}

func TestDiscoverSourcethManifestV1(t *testing.T) {
	root := t.TempDir()
	address := "0x1234"
	sourceDir := filepath.Join(root, "contracts", address, "sources")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("contract FromSourceth {}")
	if err := os.WriteFile(filepath.Join(sourceDir, "FromSourceth.sol"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	manifest := `{"schema_version":1,"tool":{"name":"sourceth","version":"2.0.0"},"run":{"id":"run-1"},"contracts":[{"address":"` + address + `","role":"root","source_status":"downloaded","files":[{"relative_path":"contracts/` + address + `/sources/FromSourceth.sol","size_bytes":` + fmt.Sprint(len(content)) + `,"sha256":"` + hex.EncodeToString(digest[:]) + `"}]}]}`
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := DiscoverSourcethManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Kind != "sourceth_manifest" || len(snapshot.Sources) != 1 {
		t.Fatalf("snapshot inesperado: %#v", snapshot)
	}
	if snapshot.Sources[0].LogicalPath != "sourceth/0x1234/FromSourceth.sol" {
		t.Fatalf("ruta lógica inesperada: %s", snapshot.Sources[0].LogicalPath)
	}
}

func TestDiscoverSourcethManifestRejectsIntegrityMismatch(t *testing.T) {
	root := t.TempDir()
	address := "0x1234"
	sourceDir := filepath.Join(root, "contracts", address, "sources")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("contract Changed {}")
	if err := os.WriteFile(filepath.Join(sourceDir, "Changed.sol"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version":1,"tool":{"name":"sourceth","version":"2.0.0"},"run":{"id":"run-1"},"contracts":[{"address":"` + address + `","source_status":"downloaded","files":[{"relative_path":"contracts/` + address + `/sources/Changed.sol","size_bytes":` + fmt.Sprint(len(content)) + `,"sha256":"` + strings.Repeat("0", 64) + `"}]}]}`
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverSourcethManifest(manifestPath); err == nil {
		t.Fatal("se esperaba rechazo por integridad incorrecta")
	}
}
