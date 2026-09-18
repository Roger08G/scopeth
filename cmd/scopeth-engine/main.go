package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	analyzer "github.com/Roger08G/scopeth/internal/analysis"
	"github.com/Roger08G/scopeth/internal/ast"
	"github.com/Roger08G/scopeth/internal/cache"
	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
	"github.com/Roger08G/scopeth/internal/profiling"
	"github.com/Roger08G/scopeth/internal/protocol"
	"github.com/Roger08G/scopeth/internal/query"
	"github.com/Roger08G/scopeth/internal/store"
)

const version = "1.0.0"

func main() {
	request, err := protocol.DecodeRequest(os.Stdin)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(protocol.Message{
			ProtocolVersion: protocol.Version, RequestID: "unknown", Type: "error",
			Error: &protocol.ErrorBody{Code: errorCode(err), Message: err.Error()},
		})
		os.Exit(4)
	}
	writer := protocol.NewWriter(os.Stdout, request)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := dispatch(ctx, request, writer); err != nil {
		_ = writer.Error(errorCode(err), cleanError(err), nil)
		return
	}
}

func dispatch(ctx context.Context, request protocol.Request, writer *protocol.Writer) error {
	switch request.Operation {
	case "doctor":
		return doctor(ctx, request, writer)
	case "analyze":
		return analyze(ctx, request, writer)
	case "query_functions":
		return queryAndFinish(request, writer, query.Functions)
	case "query_flows":
		return queryAndFinish(request, writer, query.Flows)
	case "query_state":
		return queryAndFinish(request, writer, query.State)
	case "query_surfaces":
		return queryAndFinish(request, writer, query.Surfaces)
	case "query_configuration":
		return queryAndFinish(request, writer, query.Configuration)
	case "query_explain":
		return queryAndFinish(request, writer, query.Explain)
	case "query_graph":
		return queryAndFinish(request, writer, query.Graph)
	default:
		return fmt.Errorf("INPUT_INVALID: operación desconocida: %s", request.Operation)
	}
}

func doctor(ctx context.Context, request protocol.Request, writer *protocol.Writer) error {
	solcPath := protocol.StringOption(request.Options, "solc", "solc")
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	solcIdentity, err := compiler.Inspect(checkCtx, solcPath)
	result := map[string]any{
		"engine_version": version, "protocol_version": protocol.Version, "go_version": runtime.Version(),
		"result_schema_version": store.SchemaVersion,
		"solc_path":             solcPath, "solc_available": err == nil,
		"capabilities": []string{"solidity_file", "directory", "standard_json", "build_info", "sourceth_manifest_v1", "semantic_ast", "syntax_fallback", "scc_summaries", "asset_enumeration", "review_surfaces", "streaming_queries", "on_demand_graphs"},
		"network_used": false,
	}
	if err == nil {
		result["solc_version"] = solcIdentity.Version
		result["solc_sha256"] = solcIdentity.SHA256
		result["solc_executable"] = filepath.Base(solcIdentity.Path)
	} else {
		result["solc_error"] = cleanError(err)
	}
	return writer.Result(result)
}

func analyze(ctx context.Context, request protocol.Request, writer *protocol.Writer) (returnErr error) {
	if request.Input == "" {
		return errors.New("INPUT_INVALID: falta la entrada")
	}
	profiles, err := profiling.Start(
		protocol.StringOption(request.Options, "cpu_profile", ""),
		protocol.StringOption(request.Options, "heap_profile", ""),
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := profiles.Stop(); returnErr == nil && err != nil {
			returnErr = fmt.Errorf("ENGINE_FAILED: cerrar perfiles: %w", err)
		}
	}()
	started := time.Now().UTC()
	stages := make(map[string]int64)
	_ = writer.Progress("input", "Creando snapshot seguro de las fuentes")
	stage := time.Now()
	var snapshot input.Snapshot
	if protocol.StringOption(request.Options, "input_kind", "") == "sourceth_manifest" {
		snapshot, err = input.DiscoverSourcethManifest(request.Input)
	} else {
		snapshot, err = input.Discover(request.Input)
	}
	stages["discovery"] = time.Since(stage).Milliseconds()
	if err != nil {
		return err
	}

	mode := protocol.StringOption(request.Options, "mode", "auto")
	profile := protocol.StringOption(request.Options, "profile", "deep")
	if profile != "inventory" && profile != "deep" {
		return fmt.Errorf("INPUT_INVALID: perfil desconocido: %s", profile)
	}
	timeoutSeconds := protocol.IntOption(request.Options, "timeout_seconds", 120)
	if timeoutSeconds < 1 || timeoutSeconds > 3600 {
		return errors.New("INPUT_INVALID: timeout_seconds debe estar entre 1 y 3600")
	}
	remappings, err := stringList(request.Options["remappings"])
	if err != nil {
		return err
	}
	output := protocol.StringOption(request.Options, "output", "analysis")
	solcPath := protocol.StringOption(request.Options, "solc", "solc")
	versionCtx, cancelVersion := context.WithTimeout(ctx, 10*time.Second)
	solcIdentity, err := compiler.Inspect(versionCtx, solcPath)
	cancelVersion()
	if err != nil {
		return err
	}
	cacheKey := cache.Key(snapshot, solcIdentity.Version+"|"+solcIdentity.SHA256, mode, remappings, version)
	refresh := protocol.BoolOption(request.Options, "refresh", false)
	compilation, cacheReused := compiler.Result{}, false
	if !refresh {
		stage = time.Now()
		compilation, cacheReused, err = cache.Load(output, cacheKey)
		stages["cache_lookup"] = time.Since(stage).Milliseconds()
		if err != nil {
			return err
		}
	}
	if cacheReused {
		_ = writer.Progress("compiler", "Reutilizando una compilación verificada por hash")
	} else {
		_ = writer.Progress("compiler", fmt.Sprintf("Compilando %d fuentes con solc", len(snapshot.Sources)))
		stage = time.Now()
		compilation, err = compiler.Compile(ctx, snapshot, compiler.Options{
			Mode: mode, SolcPath: solcIdentity.Path, Timeout: time.Duration(timeoutSeconds) * time.Second, Remappings: remappings,
		})
		stages["compilation"] = time.Since(stage).Milliseconds()
		if err != nil {
			return err
		}
		stage = time.Now()
		if err := cache.Save(output, cacheKey, compilation); err != nil {
			return err
		}
		stages["cache_write"] = time.Since(stage).Milliseconds()
	}

	_ = writer.Progress("analysis", "Normalizando símbolos, referencias y efectos directos")
	stage = time.Now()
	program, err := ast.Normalize(snapshot, compilation.Output, ast.Options{Profile: profile, Mode: compilation.ModeUsed})
	if err != nil {
		return fmt.Errorf("ENGINE_FAILED: %w", err)
	}
	stages["normalization"] = time.Since(stage).Milliseconds()
	if profile == "deep" {
		stage = time.Now()
		analyzer.Enrich(&program)
		stages["interprocedural_summaries"] = time.Since(stage).Milliseconds()
	}

	jobs := bounded(protocol.IntOption(request.Options, "jobs", runtime.NumCPU()), 1, 64)
	compilerJobs := bounded(protocol.IntOption(request.Options, "compiler_jobs", 1), 1, 8)
	limitations := []string{
		"Los resúmenes transitivos cruzan llamadas internas y modifiers resueltos; destinos externos y dinámicos degradan la resolución.",
		"Los aliases locales y parámetros storage se resuelven cuando solc aporta referencias tipadas; bindings incompletos se conservan como efectos unknown.",
		"La unión de efectos de modifiers está cubierta; su orden de ejecución, múltiples placeholders y el CFG completo quedan fuera de F3.",
		"Inline assembly enumera operaciones Yul seleccionadas; sus argumentos se conservan de forma simbólica y no se reconstruye un CFG completo.",
		"Las llamadas externas describen interfaces y destinos simbólicos; no prueban la identidad del código desplegado.",
		"Las superficies de revisión son observaciones con evidencia y limitaciones; no son vulnerabilidades ni asignan severidad.",
		"Las clasificaciones de activos y configuración son estáticas y conservadoras; no sustituyen trazas, balances, permisos o código on-chain.",
		"El análisis es estático y no contiene historial on-chain, balances ni transacciones ejecutadas.",
		"La normalización 1.0.0 es determinista y monohilo; --jobs queda registrado para pases paralelos futuros.",
		"La planificación 1.0.0 usa una unidad Standard JSON; --compiler-jobs se conserva para múltiples unidades futuras.",
	}
	status := compilation.Status
	_ = writer.Progress("store", "Publicando un directorio de resultados inmutable")
	completed := time.Now().UTC()
	result, err := store.Publish(output, snapshot, compilation, program, store.Metadata{
		ToolVersion: version, EngineVersion: version, Status: status, ModeRequested: mode, ModeUsed: compilation.ModeUsed,
		Profile: profile, Jobs: jobs, CompilerJobs: compilerJobs, StartedAt: started, CompletedAt: completed,
		StageDurationsMS: stages, Limitations: limitations, OptionsHash: store.OptionsHash(publicOptions(request.Options)),
		CacheKey: cacheKey, CacheReused: cacheReused,
	})
	if err != nil {
		return err
	}
	if err := profiles.Stop(); err != nil {
		return fmt.Errorf("ENGINE_FAILED: cerrar perfiles: %w", err)
	}
	return writer.Result(map[string]any{
		"run_id": result.RunID, "run_dir": result.RunDir, "status": result.Status, "summary": result.Summary,
		"report": result.RunDir + string(os.PathSeparator) + "report.md", "solc_version": compilation.SolcVersion,
		"solc_sha256":           compilation.SolcSHA256,
		"mode_used":             compilation.ModeUsed,
		"cache_reused":          cacheReused,
		"engine_version":        version,
		"result_schema_version": store.SchemaVersion,
	})
}

type queryFunc func(string, map[string]any, *protocol.Writer) (int, error)

func queryAndFinish(request protocol.Request, writer *protocol.Writer, operation queryFunc) error {
	if request.RunDir == "" {
		return errors.New("INPUT_INVALID: falta run_dir")
	}
	count, err := operation(request.RunDir, request.Options, writer)
	if err != nil {
		return err
	}
	return writer.Result(map[string]any{"count": count, "status": "complete"})
}

func errorCode(err error) string {
	text := err.Error()
	codes := []string{"INPUT_INVALID", "COMPILER_NOT_FOUND", "COMPILER_UNSUPPORTED", "IMPORT_UNRESOLVED", "COMPILATION_FAILED", "PROTOCOL_MISMATCH", "RESOURCE_LIMIT", "ANALYSIS_INCOMPLETE", "UNSAFE_PATH", "CACHE_CORRUPT", "ENGINE_FAILED"}
	for _, code := range codes {
		if strings.Contains(text, code) {
			return code
		}
	}
	if strings.Contains(strings.ToLower(text), "protocolo") {
		return "PROTOCOL_MISMATCH"
	}
	return "ENGINE_FAILED"
}

func cleanError(err error) string {
	text := err.Error()
	if len(text) > 4096 {
		return text[:4096] + "..."
	}
	return text
}

func stringList(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, errors.New("INPUT_INVALID: remappings debe ser una lista")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.ContainsAny(text, "\r\n\x00") {
			return nil, errors.New("INPUT_INVALID: remapping inválido")
		}
		result = append(result, text)
	}
	return result, nil
}

func bounded(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func publicOptions(options map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range options {
		if key == "solc" {
			result[key] = fmt.Sprint(value)
			continue
		}
		result[key] = value
	}
	return result
}
