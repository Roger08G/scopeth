package analysis

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Roger08G/scopeth/internal/model"
)

func buildIR(program *model.Program) []model.IRInstruction {
	var result []model.IRInstruction
	for _, access := range program.StateAccesses {
		if !access.Direct {
			continue
		}
		kind := "Read"
		if access.Access == "write" {
			kind = "Write"
		} else if access.Access == "read_write" {
			kind = "ReadWrite"
		}
		result = append(result, instruction(access.FunctionID, access.Function, kind, first(access.Path, access.VariableID), access.Resolution, access.Span, access.EvidenceIDs))
	}
	for _, call := range program.Calls {
		kind := "ExternalCall"
		switch call.Kind {
		case "internal":
			kind = "InternalCall"
		case "modifier":
			kind = "ModifierCall"
		case "emit":
			kind = "Emit"
		case "revert":
			kind = "Revert"
		case "dynamic":
			kind = "UnknownEffect"
		}
		result = append(result, instruction(call.CallerID, call.CallerSymbol, kind, first(call.CalleeID, call.Callee), call.Resolution, call.Span, call.EvidenceIDs))
	}
	for _, guard := range program.Guards {
		result = append(result, instruction(guard.FunctionID, guard.Function, "Branch", guard.Expression, guard.Resolution, guard.Span, guard.EvidenceIDs))
	}
	for _, operation := range program.AssetOperations {
		result = append(result, instruction(operation.FunctionID, operation.Function, "ValueOperation", operation.ID, operation.Resolution, operation.Span, operation.EvidenceIDs))
	}
	for _, observation := range program.Observations {
		result = append(result, instruction(observation.FunctionID, observation.Function, "UnknownEffect", observation.Rule, observation.Resolution, observation.Span, observation.EvidenceIDs))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Function < result[j].Function {
			return true
		}
		if result[i].Function > result[j].Function {
			return false
		}
		left, right := spanOffset(result[i].Span), spanOffset(result[j].Span)
		if left != right {
			return left < right
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func instruction(functionID, function, kind, target, resolution string, span *model.Span, evidence []string) model.IRInstruction {
	id := hashID("ir", functionID, kind, target, spanKey(span))
	return model.IRInstruction{ID: id, FunctionID: functionID, Function: function, Kind: kind, Target: target, Resolution: resolution, Span: span, EvidenceIDs: append([]string(nil), evidence...)}
}

func spanOffset(span *model.Span) int {
	if span == nil {
		return int(^uint(0) >> 1)
	}
	return span.Offset
}

func spanKey(span *model.Span) string {
	if span == nil {
		return "generated"
	}
	return strings.Join([]string{span.LogicalPath, strconv.Itoa(span.Offset), strconv.Itoa(span.Length)}, "\x00")
}
