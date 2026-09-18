package store

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
	"github.com/Roger08G/scopeth/internal/model"
	"github.com/Roger08G/scopeth/internal/report"
)

const SchemaVersion = "1.0"

type Metadata struct {
	ToolVersion      string
	EngineVersion    string
	Status           string
	ModeRequested    string
	ModeUsed         string
	Profile          string
	Jobs             int
	CompilerJobs     int
	StartedAt        time.Time
	CompletedAt      time.Time
	StageDurationsMS map[string]int64
	Limitations      []string
	OptionsHash      string
	CacheKey         string
	CacheReused      bool
}

type Result struct {
	RunID    string         `json:"run_id"`
	RunDir   string         `json:"run_dir"`
	Status   string         `json:"status"`
	Summary  map[string]any `json:"summary"`
	Manifest map[string]any `json:"manifest"`
}

func Publish(base string, snapshot input.Snapshot, compilation compiler.Result, program model.Program, metadata Metadata) (Result, error) {
	publishStarted := time.Now()
	if strings.TrimSpace(base) == "" {
		return Result{}, errors.New("INPUT_INVALID: directorio de salida vacío")
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return Result{}, err
	}
	if info, statErr := os.Lstat(abs); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return Result{}, errors.New("UNSAFE_PATH: la salida no puede ser un enlace simbólico")
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return Result{}, fmt.Errorf("crear salida: %w", err)
	}
	runID := makeRunID(snapshot, metadata)
	random, err := randomHex(6)
	if err != nil {
		return Result{}, err
	}
	stage := filepath.Join(abs, ".scopeth-staging-"+runID+"-"+random)
	final := filepath.Join(abs, runID)
	if _, err := os.Stat(final); err == nil {
		return Result{}, errors.New("CACHE_CORRUPT: el identificador de ejecución ya existe; no se sobrescribe")
	}
	if err := os.Mkdir(stage, 0o755); err != nil {
		return Result{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()

	summary := makeSummary(snapshot, compilation, program, metadata)
	if err := writeJSON(filepath.Join(stage, "summary.json"), summary); err != nil {
		return Result{}, err
	}
	if err := writeSymbols(filepath.Join(stage, "symbols.jsonl"), program); err != nil {
		return Result{}, err
	}
	files := []struct {
		name  string
		items any
	}{
		{"calls.jsonl", program.Calls}, {"state-accesses.jsonl", program.StateAccesses}, {"guards.jsonl", program.Guards},
		{"storage-aliases.jsonl", program.StorageAliases},
		{"asset-operations.jsonl", program.AssetOperations}, {"observations.jsonl", program.Observations},
		{"configuration-changes.jsonl", program.ConfigurationChanges}, {"assembly-operations.jsonl", program.AssemblyOperations},
		{"evidence.jsonl", program.Evidence}, {"edges.jsonl", program.Edges},
		{"summaries.jsonl", program.Summaries}, {"ir.jsonl", program.IR},
	}
	for _, file := range files {
		if err := writeSliceJSONL(filepath.Join(stage, file.name), file.items); err != nil {
			return Result{}, err
		}
	}
	limitations := append([]string(nil), metadata.Limitations...)
	if metadata.Profile == "inventory" {
		limitations = append(limitations, "El perfil inventory no recorre cuerpos para efectos, llamadas o activos.")
	}
	reportText := report.Markdown(program, report.Context{
		RunID: runID, Status: metadata.Status, InputKind: snapshot.Kind, ModeRequested: metadata.ModeRequested,
		ModeUsed: metadata.ModeUsed, Profile: metadata.Profile, SolcVersion: compilation.SolcVersion,
		AnalysisOnly: compilation.AnalysisOnly, FailedFiles: failedSourceFiles(compilation), Limitations: limitations,
	})
	if err := os.WriteFile(filepath.Join(stage, "report.md"), []byte(reportText), 0o644); err != nil {
		return Result{}, err
	}
	hashes, err := resultHashes(stage)
	if err != nil {
		return Result{}, err
	}
	if metadata.StageDurationsMS == nil {
		metadata.StageDurationsMS = make(map[string]int64)
	}
	metadata.StageDurationsMS["results_write"] = time.Since(publishStarted).Milliseconds()
	metadata.CompletedAt = time.Now().UTC()
	manifest := makeManifest(runID, snapshot, compilation, metadata, hashes)
	if err := writeJSON(filepath.Join(stage, "manifest.json"), manifest); err != nil {
		return Result{}, err
	}
	if err := os.Rename(stage, final); err != nil {
		return Result{}, fmt.Errorf("publicación atómica: %w", err)
	}
	committed = true
	return Result{RunID: runID, RunDir: final, Status: metadata.Status, Summary: summary, Manifest: manifest}, nil
}

func makeSummary(snapshot input.Snapshot, compilation compiler.Result, program model.Program, metadata Metadata) map[string]any {
	return map[string]any{
		"schema_version": SchemaVersion,
		"status":         metadata.Status,
		"coverage": map[string]any{
			"syntax":          "complete_for_compiler_output",
			"semantic":        semanticCoverage(compilation),
			"calls":           resolutionCountsCalls(program.Calls),
			"effects":         effectCoverage(metadata.Profile, compilation.ModeUsed),
			"assets":          assetCoverage(metadata.Profile, compilation.ModeUsed),
			"review_surfaces": reviewCoverage(metadata.Profile, compilation.ModeUsed),
			"assembly":        assemblyCoverage(metadata.Profile, compilation.ModeUsed),
		},
		"input_coverage": map[string]int{
			"discovered": len(snapshot.Sources), "analyzed": len(program.Sources), "excluded": 0, "failed": failedSourceCount(compilation),
		},
		"compilation_units": map[string]int{"planned": 1, "analyzed": 1, "failed": 0, "degraded": degradedUnitCount(compilation)},
		"modules":           moduleCoverage(metadata.Profile, compilation.ModeUsed),
		"counts": map[string]int{
			"sources": len(snapshot.Sources), "contracts": len(program.Contracts), "functions": len(program.Functions),
			"modifiers": len(program.Modifiers), "variables": len(program.Variables), "events": len(program.Events),
			"errors": len(program.Errors), "calls": len(program.Calls), "state_accesses": len(program.StateAccesses),
			"storage_aliases": len(program.StorageAliases),
			"guards":          len(program.Guards), "asset_operations": len(program.AssetOperations), "observations": len(program.Observations),
			"configuration_changes": len(program.ConfigurationChanges), "assembly_operations": len(program.AssemblyOperations),
			"function_summaries": len(program.Summaries), "ir_instructions": len(program.IR),
		},
		"limitations": metadata.Limitations,
	}
}

func makeManifest(runID string, snapshot input.Snapshot, compilation compiler.Result, metadata Metadata, hashes map[string]string) map[string]any {
	sources := make([]map[string]any, 0, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		sources = append(sources, map[string]any{
			"logical_path": source.LogicalPath, "sha256": source.SHA256, "size_bytes": len(source.Content),
			"origin": source.Origin, "address": source.Address,
		})
	}
	return map[string]any{
		"schema_version": SchemaVersion,
		"schemas":        map[string]string{"protocol": "1.0", "results": SchemaVersion},
		"tool":           map[string]string{"name": "scopeth", "version": metadata.ToolVersion, "engine_version": metadata.EngineVersion},
		"run": map[string]any{
			"id": runID, "status": metadata.Status, "started_at": metadata.StartedAt.UTC().Format(time.RFC3339Nano),
			"completed_at": metadata.CompletedAt.UTC().Format(time.RFC3339Nano),
		},
		"input": map[string]any{"kind": snapshot.Kind, "sources": sources, "provenance": snapshot.Provenance, "compilation_units": 1},
		"compilation": map[string]any{
			"solc_version": compilation.SolcVersion, "solc_executable": filepath.Base(compilation.SolcPath),
			"solc_sha256":    compilation.SolcSHA256,
			"mode_requested": metadata.ModeRequested, "mode_used": metadata.ModeUsed, "analysis_only": compilation.AnalysisOnly,
		},
		"analysis": map[string]any{
			"profile": metadata.Profile, "jobs": metadata.Jobs, "compiler_jobs": metadata.CompilerJobs,
			"options_sha256": metadata.OptionsHash, "ruleset_version": metadata.EngineVersion,
			"modules": analysisModules(metadata.Profile), "limitations": metadata.Limitations,
			"limits": map[string]int{
				"max_source_files": input.MaxSourceFiles, "max_source_bytes": input.MaxSourceBytes,
				"max_total_bytes": input.MaxTotalBytes, "max_json_bytes": input.MaxJSONBytes,
				"max_compiler_output_bytes": compiler.MaxCompilerOutput, "max_compiler_stderr_bytes": compiler.MaxCompilerStderr,
			},
		},
		"cache":       map[string]any{"key": metadata.CacheKey, "reused": metadata.CacheReused, "layer": "compilation", "integrity": "sha256_verified_on_load"},
		"metrics":     map[string]any{"stage_durations_ms": metadata.StageDurationsMS},
		"results":     hashes,
		"diagnostics": compilation.Diagnostics,
	}
}

func writeSymbols(path string, program model.Program) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriterSize(file, 64*1024)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(true)
	for index := range program.Contracts {
		if err := encoder.Encode(model.SymbolRecord{SymbolType: "contract", Contract: &program.Contracts[index]}); err != nil {
			return err
		}
	}
	for index := range program.Functions {
		if err := encoder.Encode(model.SymbolRecord{SymbolType: "function", Function: &program.Functions[index]}); err != nil {
			return err
		}
	}
	for index := range program.Modifiers {
		if err := encoder.Encode(model.SymbolRecord{SymbolType: "modifier", Modifier: &program.Modifiers[index]}); err != nil {
			return err
		}
	}
	for index := range program.Variables {
		if err := encoder.Encode(model.SymbolRecord{SymbolType: "variable", Variable: &program.Variables[index]}); err != nil {
			return err
		}
	}
	for index := range program.Events {
		if err := encoder.Encode(model.SymbolRecord{SymbolType: "event", Event: &program.Events[index]}); err != nil {
			return err
		}
	}
	for index := range program.Errors {
		if err := encoder.Encode(model.SymbolRecord{SymbolType: "error", Error: &program.Errors[index]}); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func writeSliceJSONL(path string, values any) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriterSize(file, 64*1024)
	for _, row := range rows {
		if _, err := writer.Write(row); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func resultHashes(directory string) (map[string]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	hashes := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "manifest.json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		hashes[entry.Name()] = hex.EncodeToString(hash.Sum(nil))
	}
	return hashes, nil
}

func makeRunID(snapshot input.Snapshot, metadata Metadata) string {
	hash := sha256.New()
	for _, source := range snapshot.Sources {
		_, _ = io.WriteString(hash, source.LogicalPath+"\x00"+source.SHA256+"\n")
	}
	_, _ = io.WriteString(hash, metadata.OptionsHash+"\x00"+metadata.EngineVersion)
	digest := hex.EncodeToString(hash.Sum(nil))[:12]
	return metadata.StartedAt.UTC().Format("20060102T150405.000000000Z") + "-" + digest
}

func randomHex(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func semanticCoverage(compilation compiler.Result) string {
	if compilation.ModeUsed == "semantic" {
		return "complete_for_successful_compilation"
	}
	return "unavailable"
}

func effectCoverage(profile, mode string) string {
	if profile == "deep" {
		if mode != "semantic" {
			return "syntactic_only_no_semantic_propagation"
		}
		return "direct_alias_and_scc_internal_modifier_transitive"
	}
	return "not_requested"
}

func assetCoverage(profile, mode string) string {
	if profile == "deep" {
		if mode != "semantic" {
			return "syntactic_partial_without_typed_guarantee"
		}
		return "typed_symbolic_with_reachability_guards_and_return_handling"
	}
	return "not_requested"
}

func reviewCoverage(profile, mode string) string {
	if profile == "deep" {
		if mode != "semantic" {
			return "syntactic_observations_partial"
		}
		return "evidence_backed_observations_without_severity"
	}
	return "not_requested"
}

func assemblyCoverage(profile, mode string) string {
	if profile == "deep" {
		if mode != "semantic" {
			return "selected_yul_operations_syntactic_partial"
		}
		return "presence_and_selected_yul_operations"
	}
	return "not_requested"
}

func analysisModules(profile string) []string {
	modules := []string{"inventory", "abi", "storage_layout"}
	if profile == "deep" {
		modules = append(modules, "calls", "effects", "storage_aliases", "function_summaries", "assets", "authorizations", "configuration", "review_surfaces", "assembly_enumeration", "reporting")
	}
	return modules
}

func moduleCoverage(profile, mode string) map[string]string {
	result := map[string]string{"inventory": "produced", "abi": "produced", "storage_layout": "produced"}
	if mode != "semantic" {
		result["inventory"] = "partial"
		result["abi"] = "unavailable"
		result["storage_layout"] = "unavailable"
	}
	for _, module := range []string{"calls", "effects", "storage_aliases", "function_summaries", "assets", "authorizations", "configuration", "review_surfaces", "assembly_enumeration", "reporting"} {
		result[module] = "not_requested"
		if profile == "deep" {
			result[module] = "produced"
			if mode != "semantic" && module != "reporting" {
				result[module] = "partial"
			}
		}
	}
	return result
}

func degradedUnitCount(compilation compiler.Result) int {
	if compilation.ModeUsed != "semantic" {
		return 1
	}
	return 0
}

func failedSourceCount(compilation compiler.Result) int {
	return len(failedSourceFiles(compilation))
}

func failedSourceFiles(compilation compiler.Result) []string {
	failed := make(map[string]struct{})
	for _, diagnostic := range compilation.Diagnostics {
		if diagnostic.Severity == "error" && diagnostic.SourceLocation != nil && diagnostic.SourceLocation.File != "" {
			failed[diagnostic.SourceLocation.File] = struct{}{}
		}
	}
	result := make([]string, 0, len(failed))
	for file := range failed {
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

func resolutionCountsCalls(calls []model.CallSite) map[string]int {
	counts := map[string]int{"resolved": 0, "partial": 0, "unknown": 0}
	for _, call := range calls {
		counts[call.Resolution]++
	}
	return counts
}

func OptionsHash(options map[string]any) string {
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([][2]any, 0, len(keys))
	for _, key := range keys {
		ordered = append(ordered, [2]any{key, options[key]})
	}
	data, _ := json.Marshal(ordered)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
