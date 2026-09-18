package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Roger08G/scopeth/internal/input"
)

const (
	MaxCompilerOutput = 128 << 20
	MaxCompilerStderr = 1 << 20
)

type Diagnostic struct {
	Component        string `json:"component,omitempty"`
	ErrorCode        string `json:"errorCode,omitempty"`
	FormattedMessage string `json:"formattedMessage,omitempty"`
	Message          string `json:"message"`
	Severity         string `json:"severity"`
	Type             string `json:"type,omitempty"`
	SourceLocation   *struct {
		File  string `json:"file"`
		Start int    `json:"start"`
		End   int    `json:"end"`
	} `json:"sourceLocation,omitempty"`
}

type ABIParameter struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	InternalType string         `json:"internalType,omitempty"`
	Components   []ABIParameter `json:"components,omitempty"`
}

type ABIEntry struct {
	Type            string         `json:"type"`
	Name            string         `json:"name,omitempty"`
	Inputs          []ABIParameter `json:"inputs,omitempty"`
	Outputs         []ABIParameter `json:"outputs,omitempty"`
	StateMutability string         `json:"stateMutability,omitempty"`
	Anonymous       bool           `json:"anonymous,omitempty"`
}

type StorageEntry struct {
	ASTID    int    `json:"astId"`
	Contract string `json:"contract"`
	Label    string `json:"label"`
	Offset   int    `json:"offset"`
	Slot     string `json:"slot"`
	Type     string `json:"type"`
}

type StorageType struct {
	Encoding      string `json:"encoding"`
	Label         string `json:"label"`
	NumberOfBytes string `json:"numberOfBytes"`
}

type StorageLayout struct {
	Storage []StorageEntry         `json:"storage"`
	Types   map[string]StorageType `json:"types"`
}

type ContractOutput struct {
	ABI                    []ABIEntry    `json:"abi"`
	StorageLayout          StorageLayout `json:"storageLayout"`
	TransientStorageLayout StorageLayout `json:"transientStorageLayout"`
}

type SourceOutput struct {
	ID  int             `json:"id"`
	AST json.RawMessage `json:"ast"`
}

type Output struct {
	Contracts map[string]map[string]ContractOutput `json:"contracts"`
	Sources   map[string]SourceOutput              `json:"sources"`
	Errors    []Diagnostic                         `json:"errors"`
}

type Result struct {
	Output        Output        `json:"output"`
	ModeRequested string        `json:"mode_requested"`
	ModeUsed      string        `json:"mode_used"`
	Status        string        `json:"status"`
	SolcPath      string        `json:"solc_path"`
	SolcVersion   string        `json:"solc_version"`
	SolcSHA256    string        `json:"solc_sha256"`
	AnalysisOnly  bool          `json:"analysis_only"`
	Diagnostics   []Diagnostic  `json:"diagnostics"`
	Duration      time.Duration `json:"-"`
}

type Identity struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type Options struct {
	Mode       string
	SolcPath   string
	Timeout    time.Duration
	Remappings []string
}

var versionPattern = regexp.MustCompile(`Version:\s*([^\s]+)`)

func Version(ctx context.Context, path string) (string, error) {
	identity, err := Inspect(ctx, path)
	if err != nil {
		return "", err
	}
	return identity.Version, nil
}

func Inspect(ctx context.Context, path string) (Identity, error) {
	if path == "" {
		path = "solc"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return Identity{}, fmt.Errorf("COMPILER_NOT_FOUND: no se encontró %s: %w", path, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return Identity{}, fmt.Errorf("COMPILER_NOT_FOUND: resolver solc: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Identity{}, errors.New("COMPILER_NOT_FOUND: solc debe ser un archivo regular confiable")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return Identity{}, fmt.Errorf("COMPILER_NOT_FOUND: abrir solc: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return Identity{}, fmt.Errorf("COMPILER_NOT_FOUND: calcular identidad de solc: %w", copyErr)
	}
	if closeErr != nil {
		return Identity{}, closeErr
	}
	command := exec.CommandContext(ctx, resolved, "--version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &limitedWriter{writer: &stderr, remaining: MaxCompilerStderr}
	if err := command.Run(); err != nil {
		return Identity{}, fmt.Errorf("COMPILER_NOT_FOUND: no se pudo ejecutar %s: %w: %s", filepath.Base(resolved), err, strings.TrimSpace(stderr.String()))
	}
	match := versionPattern.FindStringSubmatch(stdout.String())
	if len(match) != 2 {
		return Identity{}, errors.New("COMPILER_UNSUPPORTED: solc no devolvió una versión reconocible")
	}
	return Identity{Path: resolved, Version: match[1], SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func Compile(ctx context.Context, snapshot input.Snapshot, options Options) (Result, error) {
	mode := strings.ToLower(options.Mode)
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "semantic" && mode != "syntax" {
		return Result{}, fmt.Errorf("INPUT_INVALID: modo desconocido: %s", mode)
	}
	if options.Timeout <= 0 {
		options.Timeout = 120 * time.Second
	}
	versionCtx, cancelVersion := context.WithTimeout(ctx, 10*time.Second)
	defer cancelVersion()
	identity, err := Inspect(versionCtx, options.SolcPath)
	if err != nil {
		return Result{}, err
	}
	options.SolcPath = identity.Path
	started := time.Now()
	if mode == "syntax" {
		output, runErr := invoke(ctx, snapshot, options, true)
		return finishResult(snapshot, options, identity, mode, "syntax", output, runErr, started)
	}
	output, runErr := invoke(ctx, snapshot, options, false)
	if runErr == nil && !hasErrors(output.Errors) {
		return finishResult(snapshot, options, identity, mode, "semantic", output, nil, started)
	}
	if mode == "semantic" {
		return finishResult(snapshot, options, identity, mode, "semantic", output, runErr, started)
	}
	syntaxOutput, syntaxErr := invoke(ctx, snapshot, options, true)
	result, finalErr := finishResult(snapshot, options, identity, mode, "syntax", syntaxOutput, syntaxErr, started)
	if finalErr == nil {
		result.Status = "partial"
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Severity: "warning",
			Type:     "ScopethFallback",
			Message:  "El análisis semántico falló; se conservaron exclusivamente hechos sintácticos.",
		})
		result.Diagnostics = append(result.Diagnostics, output.Errors...)
	}
	return result, finalErr
}

func finishResult(snapshot input.Snapshot, options Options, identity Identity, requested, used string, output Output, runErr error, started time.Time) (Result, error) {
	result := Result{
		Output:        output,
		ModeRequested: requested,
		ModeUsed:      used,
		Status:        "complete",
		SolcPath:      options.SolcPath,
		SolcVersion:   identity.Version,
		SolcSHA256:    identity.SHA256,
		Diagnostics:   output.Errors,
		Duration:      time.Since(started),
	}
	if result.SolcPath == "" {
		result.SolcPath = "solc"
	}
	if snapshot.CompilerVersion != "" && !strings.Contains(identity.Version, snapshot.CompilerVersion) {
		result.AnalysisOnly = true
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Severity: "warning",
			Type:     "CompilerIdentity",
			Message:  fmt.Sprintf("El build-info declara solc %s y el análisis usa %s.", snapshot.CompilerVersion, identity.Version),
		})
	}
	if runErr != nil {
		return result, runErr
	}
	if hasErrors(output.Errors) && used != "syntax" {
		return result, errors.New("COMPILATION_FAILED: solc devolvió diagnósticos de severidad error")
	}
	if len(output.Sources) == 0 {
		return result, errors.New("COMPILATION_FAILED: solc no devolvió AST aprovechable")
	}
	if used == "syntax" {
		result.Status = "partial"
	}
	return result, nil
}

func invoke(parent context.Context, snapshot input.Snapshot, options Options, syntaxOnly bool) (Output, error) {
	request := map[string]any{
		"language": "Solidity",
		"sources":  make(map[string]map[string]string, len(snapshot.Sources)),
		"settings": map[string]any{
			"outputSelection": map[string]any{
				"*": map[string]any{
					"": []string{"ast"},
				},
			},
		},
	}
	sources := request["sources"].(map[string]map[string]string)
	for _, source := range snapshot.Sources {
		sources[source.LogicalPath] = map[string]string{"content": string(source.Content)}
	}
	settings := request["settings"].(map[string]any)
	if syntaxOnly {
		settings["stopAfter"] = "parsing"
	} else {
		settings["outputSelection"].(map[string]any)["*"].(map[string]any)["*"] = []string{"abi", "storageLayout", "transientStorageLayout"}
	}
	if len(options.Remappings) > 0 {
		remappings := append([]string(nil), options.Remappings...)
		sort.Strings(remappings)
		settings["remappings"] = remappings
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return Output{}, err
	}
	ctx, cancel := context.WithTimeout(parent, options.Timeout)
	defer cancel()
	path := options.SolcPath
	if path == "" {
		path = "solc"
	}
	command := exec.CommandContext(ctx, path, "--standard-json")
	command.Stdin = bytes.NewReader(payload)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Output{}, err
	}
	var stderr bytes.Buffer
	command.Stderr = &limitedWriter{writer: &stderr, remaining: MaxCompilerStderr}
	if err := command.Start(); err != nil {
		return Output{}, fmt.Errorf("COMPILER_NOT_FOUND: iniciar solc: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, MaxCompilerOutput+1))
	waitErr := command.Wait()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Output{}, errors.New("RESOURCE_LIMIT: solc superó el tiempo permitido")
	}
	if readErr != nil {
		return Output{}, fmt.Errorf("ENGINE_FAILED: leer stdout de solc: %w", readErr)
	}
	if len(data) > MaxCompilerOutput {
		return Output{}, errors.New("RESOURCE_LIMIT: stdout de solc supera 128 MiB")
	}
	var output Output
	if err := json.Unmarshal(data, &output); err != nil {
		diagnostic := strings.TrimSpace(stderr.String())
		if len(diagnostic) > 512 {
			diagnostic = diagnostic[:512]
		}
		return Output{}, fmt.Errorf("COMPILATION_FAILED: respuesta JSON inválida de solc: %w: %s", err, diagnostic)
	}
	if waitErr != nil && len(output.Errors) == 0 {
		return output, fmt.Errorf("COMPILATION_FAILED: solc terminó de forma anormal: %w", waitErr)
	}
	return output, nil
}

func hasErrors(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			return true
		}
	}
	return false
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	original := len(p)
	if w.remaining <= 0 {
		return original, nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	_, err := w.writer.Write(p)
	w.remaining -= len(p)
	return original, err
}
