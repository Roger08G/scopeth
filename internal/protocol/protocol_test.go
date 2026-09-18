package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeRequest(t *testing.T) {
	raw := `{"protocol_version":"1.0","request_id":"req-1","operation":"doctor"}`
	request, err := DecodeRequest(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if request.RequestID != "req-1" || request.Operation != "doctor" {
		t.Fatalf("solicitud inesperada: %#v", request)
	}
}

func TestDecodeRequestRejectsUnknownField(t *testing.T) {
	raw := `{"protocol_version":"1.0","request_id":"req-1","operation":"doctor","extra":true}`
	if _, err := DecodeRequest(strings.NewReader(raw)); err == nil {
		t.Fatal("se esperaba rechazo de campo desconocido")
	}
}

func TestWriterIncludesEnvelope(t *testing.T) {
	var buffer bytes.Buffer
	writer := NewWriter(&buffer, Request{RequestID: "abc"})
	if err := writer.Progress("input", "descubriendo"); err != nil {
		t.Fatal(err)
	}
	var message Message
	if err := json.Unmarshal(buffer.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	if message.ProtocolVersion != Version || message.RequestID != "abc" || message.Type != "progress" {
		t.Fatalf("mensaje inesperado: %#v", message)
	}
}

func FuzzDecodeRequest(f *testing.F) {
	f.Add([]byte(`{"protocol_version":"1.0","request_id":"x","operation":"doctor"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeRequest(bytes.NewReader(data))
	})
}
