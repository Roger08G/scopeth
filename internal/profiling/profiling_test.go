package profiling

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHeapProfileIsWrittenWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heap.prof")
	session, err := Start("", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Stop(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("el perfil heap está vacío")
	}
	if _, err := Start("", path); err == nil {
		t.Fatal("se esperaba rechazo al intentar sobrescribir un perfil")
	}
}
