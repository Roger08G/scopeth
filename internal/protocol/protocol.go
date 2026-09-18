package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	Version           = "1.0"
	MaxRequestBytes   = 1 << 20
	MaxMessageBytes   = 4 << 20
	MaxDiagnosticSize = 1 << 20
)

type Request struct {
	ProtocolVersion string         `json:"protocol_version"`
	RequestID       string         `json:"request_id"`
	Operation       string         `json:"operation"`
	Input           string         `json:"input,omitempty"`
	RunDir          string         `json:"run_dir,omitempty"`
	Options         map[string]any `json:"options,omitempty"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Message struct {
	ProtocolVersion string         `json:"protocol_version"`
	RequestID       string         `json:"request_id"`
	Type            string         `json:"type"`
	Phase           string         `json:"phase,omitempty"`
	Message         string         `json:"message,omitempty"`
	Record          any            `json:"record,omitempty"`
	Result          map[string]any `json:"result,omitempty"`
	Error           *ErrorBody     `json:"error,omitempty"`
}

type Writer struct {
	mu  sync.Mutex
	enc *json.Encoder
	req Request
}

func DecodeRequest(r io.Reader) (Request, error) {
	limited := io.LimitReader(r, MaxRequestBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return Request{}, fmt.Errorf("leer solicitud: %w", err)
	}
	if len(data) > MaxRequestBytes {
		return Request{}, errors.New("la solicitud supera el límite de 1 MiB")
	}
	var request Request
	decoder := json.NewDecoder(bufio.NewReaderSize(io.NopCloser(&byteReader{data: data}), 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("JSON de solicitud inválido: %w", err)
	}
	if request.ProtocolVersion != Version {
		return Request{}, fmt.Errorf("versión de protocolo incompatible: %q", request.ProtocolVersion)
	}
	if request.RequestID == "" || len(request.RequestID) > 128 {
		return Request{}, errors.New("request_id ausente o demasiado largo")
	}
	if request.Operation == "" || len(request.Operation) > 64 {
		return Request{}, errors.New("operation ausente o demasiado larga")
	}
	return request, nil
}

// byteReader evita que el decodificador mantenga una referencia a un archivo o socket.
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func NewWriter(w io.Writer, request Request) *Writer {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	return &Writer{enc: encoder, req: request}
}

func (w *Writer) emit(message Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	message.ProtocolVersion = Version
	message.RequestID = w.req.RequestID
	return w.enc.Encode(message)
}

func (w *Writer) Progress(phase, message string) error {
	return w.emit(Message{Type: "progress", Phase: phase, Message: message})
}

func (w *Writer) Record(record any) error {
	return w.emit(Message{Type: "record", Record: record})
}

func (w *Writer) Result(result map[string]any) error {
	return w.emit(Message{Type: "result", Result: result})
}

func (w *Writer) Error(code, message string, details map[string]any) error {
	return w.emit(Message{Type: "error", Error: &ErrorBody{Code: code, Message: message, Details: details}})
}

func StringOption(options map[string]any, key, fallback string) string {
	if value, ok := options[key].(string); ok && value != "" {
		return value
	}
	return fallback
}

func IntOption(options map[string]any, key string, fallback int) int {
	if value, ok := options[key].(float64); ok && value >= 0 {
		return int(value)
	}
	return fallback
}

func BoolOption(options map[string]any, key string, fallback bool) bool {
	if value, ok := options[key].(bool); ok {
		return value
	}
	return fallback
}
