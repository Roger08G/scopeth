package query

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Roger08G/scopeth/internal/model"
	"github.com/Roger08G/scopeth/internal/protocol"
)

const maxJSONLRecord = 4 << 20

func Functions(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	visibility := protocol.StringOption(options, "visibility", "")
	mutability := protocol.StringOption(options, "mutability", "")
	modifier := protocol.StringOption(options, "modifier", "")
	payable, filterPayable := options["payable"].(bool)
	return streamJSONL[model.SymbolRecord](runDir, "symbols.jsonl", options, writer, func(record model.SymbolRecord) (any, bool) {
		if record.SymbolType != "function" || record.Function == nil {
			return nil, false
		}
		if visibility != "" && record.Function.Visibility != visibility {
			return nil, false
		}
		if mutability != "" && record.Function.StateMutability != mutability {
			return nil, false
		}
		if modifier != "" && !contains(record.Function.Modifiers, modifier) && !contains(record.Function.ModifierIDs, modifier) {
			return nil, false
		}
		if filterPayable && record.Function.Payable != payable {
			return nil, false
		}
		return record.Function, true
	})
}

func Flows(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	assetClass := protocol.StringOption(options, "asset_class", "")
	category := protocol.StringOption(options, "category", "")
	operation := protocol.StringOption(options, "operation", "")
	reachableFrom := protocol.StringOption(options, "reachable_from", "")
	return streamJSONL[model.AssetOperation](runDir, "asset-operations.jsonl", options, writer, func(record model.AssetOperation) (any, bool) {
		if assetClass != "" && record.AssetClass != assetClass {
			return nil, false
		}
		if category != "" && record.Category != category {
			return nil, false
		}
		if operation != "" && record.Operation != operation {
			return nil, false
		}
		if reachableFrom != "" && !contains(record.ReachableFrom, reachableFrom) {
			return nil, false
		}
		return record, true
	})
}

func State(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	writtenBy := protocol.StringOption(options, "written_by", "")
	variable := protocol.StringOption(options, "variable", "")
	access := protocol.StringOption(options, "access", "")
	return streamJSONL[model.StateAccess](runDir, "state-accesses.jsonl", options, writer, func(record model.StateAccess) (any, bool) {
		if writtenBy != "" && record.Function != writtenBy && record.FunctionID != writtenBy {
			return nil, false
		}
		if writtenBy != "" && record.Access != "write" && record.Access != "read_write" {
			return nil, false
		}
		if variable != "" && record.Variable != variable && record.VariableID != variable && record.Path != variable {
			return nil, false
		}
		if access != "" && record.Access != access {
			return nil, false
		}
		return record, true
	})
}

func Surfaces(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	category := protocol.StringOption(options, "category", "")
	rule := protocol.StringOption(options, "rule", "")
	resolution := protocol.StringOption(options, "resolution", "")
	return streamJSONL[model.Observation](runDir, "observations.jsonl", options, writer, func(record model.Observation) (any, bool) {
		if category != "" && record.Category != category {
			return nil, false
		}
		if rule != "" && record.Rule != rule {
			return nil, false
		}
		if resolution != "" && record.Resolution != resolution {
			return nil, false
		}
		return record, true
	})
}

func Configuration(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	category := protocol.StringOption(options, "category", "")
	reachableFrom := protocol.StringOption(options, "reachable_from", "")
	return streamJSONL[model.ConfigurationChange](runDir, "configuration-changes.jsonl", options, writer, func(record model.ConfigurationChange) (any, bool) {
		if category != "" && record.Category != category {
			return nil, false
		}
		if reachableFrom != "" && !contains(record.ReachableFrom, reachableFrom) {
			return nil, false
		}
		return record, true
	})
}

func Explain(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	symbol := protocol.StringOption(options, "symbol", "")
	if symbol == "" {
		return 0, errors.New("INPUT_INVALID: explain requiere symbol")
	}
	var function *model.Function
	err := scanJSONL(runDir, "symbols.jsonl", func(line []byte) error {
		var record model.SymbolRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if record.SymbolType == "function" && record.Function != nil && (record.Function.Symbol == symbol || record.Function.ID == symbol) {
			copyFunction := *record.Function
			function = &copyFunction
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if function == nil {
		return 0, fmt.Errorf("INPUT_INVALID: símbolo no encontrado: %s", symbol)
	}
	result := map[string]any{"function": function}
	accesses, err := collect[model.StateAccess](runDir, "state-accesses.jsonl", func(value model.StateAccess) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	aliases, err := collectOptional[model.StorageAlias](runDir, "storage-aliases.jsonl", func(value model.StorageAlias) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	calls, err := collect[model.CallSite](runDir, "calls.jsonl", func(value model.CallSite) bool { return value.CallerID == function.ID })
	if err != nil {
		return 0, err
	}
	guards, err := collect[model.Guard](runDir, "guards.jsonl", func(value model.Guard) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	assets, err := collect[model.AssetOperation](runDir, "asset-operations.jsonl", func(value model.AssetOperation) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	observations, err := collect[model.Observation](runDir, "observations.jsonl", func(value model.Observation) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	summaries, err := collect[model.FunctionSummary](runDir, "summaries.jsonl", func(value model.FunctionSummary) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	configuration, err := collectOptional[model.ConfigurationChange](runDir, "configuration-changes.jsonl", func(value model.ConfigurationChange) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	assembly, err := collectOptional[model.AssemblyOperation](runDir, "assembly-operations.jsonl", func(value model.AssemblyOperation) bool { return value.FunctionID == function.ID })
	if err != nil {
		return 0, err
	}
	result["state_accesses"], result["storage_aliases"], result["calls"], result["guards"], result["asset_operations"], result["observations"] = accesses, aliases, calls, guards, assets, observations
	result["configuration_changes"], result["assembly_operations"] = configuration, assembly
	events := make([]model.CallSite, 0)
	reverts := make([]model.CallSite, 0)
	unresolved := make([]model.CallSite, 0)
	for _, call := range calls {
		switch call.Kind {
		case "emit":
			events = append(events, call)
		case "revert":
			reverts = append(reverts, call)
		}
		if call.Resolution != "resolved" {
			unresolved = append(unresolved, call)
		}
	}
	evidenceIDs := make(map[string]struct{})
	addEvidenceIDs(evidenceIDs, function.EvidenceIDs)
	for _, value := range accesses {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range aliases {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range calls {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range guards {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range assets {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range observations {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range configuration {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	for _, value := range assembly {
		addEvidenceIDs(evidenceIDs, value.EvidenceIDs)
	}
	evidence, err := collect[model.Evidence](runDir, "evidence.jsonl", func(value model.Evidence) bool {
		_, exists := evidenceIDs[value.ID]
		return exists
	})
	if err != nil {
		return 0, err
	}
	result["events"], result["reverts"], result["unresolved_relations"], result["evidence"] = events, reverts, unresolved, evidence
	result["caller_model"] = map[string]any{
		"visibility": function.Visibility, "entrypoint": function.Visibility == "public" || function.Visibility == "external" || function.Kind == "receive" || function.Kind == "fallback",
		"guard_count": len(guards), "coverage": "symbolic; no demuestra autorización efectiva sin CFG y estado on-chain",
	}
	if len(summaries) == 1 {
		result["summary"] = summaries[0]
	}
	if err := writer.Record(result); err != nil {
		return 0, err
	}
	return 1, nil
}

func addEvidenceIDs(target map[string]struct{}, values []string) {
	for _, value := range values {
		if value != "" {
			target[value] = struct{}{}
		}
	}
}

func Graph(runDir string, options map[string]any, writer *protocol.Writer) (int, error) {
	kind := protocol.StringOption(options, "kind", "calls")
	format := protocol.StringOption(options, "format", "mermaid")
	if kind != "calls" && kind != "imports" && kind != "inheritance" && kind != "state" && kind != "assets" {
		return 0, errors.New("INPUT_INVALID: kind debe ser calls, imports, inheritance, state o assets")
	}
	if format != "mermaid" && format != "json" {
		return 0, errors.New("INPUT_INVALID: format debe ser mermaid o json")
	}
	root := protocol.StringOption(options, "root", "")
	depth, err := integerOption(options, "depth", 4)
	if err != nil {
		return 0, err
	}
	if depth < 1 || depth > 32 {
		return 0, errors.New("INPUT_INVALID: depth debe estar entre 1 y 32")
	}
	edges, err := graphEdges(runDir, kind)
	if err != nil {
		return 0, err
	}
	filtered := reachableGraph(edges, root, depth)
	if format == "json" {
		if err := writer.Record(map[string]any{"kind": kind, "format": "json", "edges": filtered, "edge_count": len(filtered)}); err != nil {
			return 0, err
		}
		return len(filtered), nil
	}
	var output strings.Builder
	output.WriteString("flowchart LR\n")
	for index, edge := range filtered {
		from := "n" + strconv.Itoa(index) + "a"
		to := "n" + strconv.Itoa(index) + "b"
		fmt.Fprintf(&output, "  %s[\"%s\"] -->|%s/%s| %s[\"%s\"]\n", from, mermaid(edge.From), mermaid(edge.Kind), mermaid(edge.Resolution), to, mermaid(edge.To))
	}
	if err := writer.Record(map[string]any{"kind": kind, "format": "mermaid", "content": output.String(), "edge_count": len(filtered)}); err != nil {
		return 0, err
	}
	return len(filtered), nil
}

func streamJSONL[T any](runDir, name string, options map[string]any, writer *protocol.Writer, selectRecord func(T) (any, bool)) (int, error) {
	offset, err := integerOption(options, "offset", 0)
	if err != nil {
		return 0, err
	}
	limit, err := integerOption(options, "limit", 100)
	if err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, errors.New("INPUT_INVALID: offset no puede ser negativo")
	}
	if limit < 1 || limit > 10_000 {
		return 0, errors.New("INPUT_INVALID: limit debe estar entre 1 y 10000")
	}
	matched, emitted := 0, 0
	err = scanJSONL(runDir, name, func(line []byte) error {
		var record T
		if err := json.Unmarshal(line, &record); err != nil {
			return fmt.Errorf("CACHE_CORRUPT: %s contiene JSON inválido: %w", name, err)
		}
		selected, ok := selectRecord(record)
		if !ok {
			return nil
		}
		if matched < offset {
			matched++
			return nil
		}
		matched++
		if emitted >= limit {
			return nil
		}
		if err := writer.Record(selected); err != nil {
			return err
		}
		emitted++
		return nil
	})
	return emitted, err
}

func integerOption(options map[string]any, key string, fallback int) (int, error) {
	value, exists := options[key]
	if !exists {
		return fallback, nil
	}
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < math.MinInt32 || number > math.MaxInt32 {
		return 0, fmt.Errorf("INPUT_INVALID: %s debe ser un entero", key)
	}
	return int(number), nil
}

func collect[T any](runDir, name string, predicate func(T) bool) ([]T, error) {
	var result []T
	err := scanJSONL(runDir, name, func(line []byte) error {
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			return fmt.Errorf("CACHE_CORRUPT: %s: %w", name, err)
		}
		if predicate(value) {
			result = append(result, value)
		}
		return nil
	})
	return result, err
}

func collectOptional[T any](runDir, name string, predicate func(T) bool) ([]T, error) {
	_, err := safeResultFile(runDir, name)
	if err == nil {
		return collect(runDir, name, predicate)
	}
	abs, absErr := filepath.Abs(runDir)
	if absErr != nil {
		return nil, err
	}
	info, dirErr := os.Lstat(abs)
	if dirErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || filepath.Base(name) != name {
		return nil, err
	}
	if _, statErr := os.Lstat(filepath.Join(abs, name)); os.IsNotExist(statErr) {
		return nil, nil
	}
	return nil, err
}

func scanJSONL(runDir, name string, visit func([]byte) error) error {
	path, err := safeResultFile(runDir, name)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("INPUT_INVALID: abrir resultado %s: %w", name, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxJSONLRecord)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		copyLine := append([]byte(nil), scanner.Bytes()...)
		if err := visit(copyLine); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("CACHE_CORRUPT: leer %s: %w", name, err)
	}
	return nil
}

func safeResultFile(runDir, name string) (string, error) {
	abs, err := filepath.Abs(runDir)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("UNSAFE_PATH: run_dir debe ser un directorio real")
	}
	if filepath.Base(name) != name {
		return "", errors.New("UNSAFE_PATH: nombre de resultado inválido")
	}
	candidate := filepath.Join(abs, name)
	fileInfo, err := os.Lstat(candidate)
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("UNSAFE_PATH: resultado no regular o ausente: %s", name)
	}
	return candidate, nil
}

type graphEdge struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	From       string `json:"from"`
	To         string `json:"to"`
	Resolution string `json:"resolution"`
}

func graphEdges(runDir, kind string) ([]graphEdge, error) {
	var result []graphEdge
	switch kind {
	case "calls":
		values, err := collect[model.CallSite](runDir, "calls.jsonl", func(model.CallSite) bool { return true })
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			result = append(result, graphEdge{ID: value.ID, Kind: value.Kind, From: first(value.CallerSymbol, value.CallerID), To: first(value.Callee, value.CalleeID, "unknown"), Resolution: value.Resolution})
		}
	case "imports", "inheritance":
		values, err := collect[model.Edge](runDir, "edges.jsonl", func(value model.Edge) bool { return value.Kind == strings.TrimSuffix(kind, "s") })
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			result = append(result, graphEdge{ID: value.ID, Kind: value.Kind, From: value.From, To: value.To, Resolution: value.Resolution})
		}
	case "state":
		values, err := collect[model.StateAccess](runDir, "state-accesses.jsonl", func(model.StateAccess) bool { return true })
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			result = append(result, graphEdge{ID: value.ID, Kind: value.Access, From: first(value.Function, value.FunctionID), To: "state:" + first(value.Path, value.Variable, value.VariableID), Resolution: value.Resolution})
		}
	case "assets":
		values, err := collect[model.AssetOperation](runDir, "asset-operations.jsonl", func(model.AssetOperation) bool { return true })
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			target := "asset:" + value.AssetClass + ":" + first(value.AssetExpression, value.Operation)
			result = append(result, graphEdge{ID: value.ID, Kind: value.Category + "/" + value.Operation, From: first(value.Function, value.FunctionID), To: target, Resolution: value.Resolution})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func reachableGraph(edges []graphEdge, root string, depth int) []graphEdge {
	if root == "" {
		result := append([]graphEdge(nil), edges...)
		sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
		return result
	}
	frontier := map[string]struct{}{root: {}}
	seen := make(map[string]struct{})
	var result []graphEdge
	for level := 0; level < depth && len(frontier) > 0; level++ {
		next := make(map[string]struct{})
		for _, edge := range edges {
			if _, ok := frontier[edge.From]; !ok {
				continue
			}
			if _, ok := seen[edge.ID]; ok {
				continue
			}
			seen[edge.ID] = struct{}{}
			result = append(result, edge)
			next[edge.To] = struct{}{}
		}
		frontier = next
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func mermaid(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "'")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
