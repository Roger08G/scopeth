package compiler

import "testing"

func TestHasErrors(t *testing.T) {
	if hasErrors([]Diagnostic{{Severity: "warning"}}) {
		t.Fatal("warning no debe fallar la compilación")
	}
	if !hasErrors([]Diagnostic{{Severity: "error"}}) {
		t.Fatal("error debe fallar la compilación")
	}
}

func TestLimitedWriterTruncates(t *testing.T) {
	buffer := &countingWriter{}
	writer := &limitedWriter{writer: buffer, remaining: 3}
	n, err := writer.Write([]byte("abcdef"))
	if err != nil || n != 6 || buffer.count != 3 {
		t.Fatalf("resultado inesperado n=%d count=%d err=%v", n, buffer.count, err)
	}
}

type countingWriter struct{ count int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.count += len(p)
	return len(p), nil
}
