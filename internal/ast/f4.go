package ast

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/Roger08G/scopeth/internal/model"
)

func analyzeReturnHandling(body *Node) map[int]string {
	result := make(map[int]string)
	assignedCalls := make(map[int][]int)
	walk(body, func(node *Node) {
		switch node.NodeType {
		case "VariableDeclarationStatement":
			calls := nestedCallIDs(node.InitialValue)
			if len(calls) == 0 {
				return
			}
			for _, declaration := range node.Declarations {
				if declaration != nil {
					assignedCalls[declaration.ID] = calls
				}
			}
		case "Assignment":
			if node.LeftHandSide != nil && node.LeftHandSide.NodeType == "Identifier" {
				assignedCalls[node.LeftHandSide.ReferencedDeclaration] = nestedCallIDs(node.RightHandSide)
			}
		}
	})
	walk(body, func(node *Node) {
		switch node.NodeType {
		case "FunctionCall":
			if node.Expression == nil || node.Expression.NodeType != "Identifier" || (node.Expression.Name != "require" && node.Expression.Name != "assert") {
				return
			}
			if len(node.Arguments) == 0 {
				return
			}
			markNestedCalls(result, &node.Arguments[0], "required")
			walk(&node.Arguments[0], func(candidate *Node) {
				if candidate.NodeType != "Identifier" {
					return
				}
				for _, callID := range assignedCalls[candidate.ReferencedDeclaration] {
					result[callID] = "required"
				}
			})
		case "Return":
			markNestedCalls(result, node.Expression, "propagated")
		case "TryStatement":
			markNestedCalls(result, node.ExternalCall, "try_catch")
		}
	})
	return result
}

func nestedCallIDs(node *Node) []int {
	var result []int
	walk(node, func(candidate *Node) {
		if candidate.NodeType == "FunctionCall" {
			result = append(result, candidate.ID)
		}
	})
	return result
}

func markNestedCalls(target map[int]string, node *Node, handling string) {
	for _, callID := range nestedCallIDs(node) {
		target[callID] = handling
	}
}

func (n *normalizer) addNodeReviewSurface(function model.Function, node *Node, span *model.Span) {
	switch node.NodeType {
	case "ForStatement", "WhileStatement", "DoWhileStatement":
		n.addReviewObservation(function, "data-dependent-loop", "control_flow", "Bucle dependiente de datos", "El cuerpo contiene un bucle cuya cardinalidad efectiva depende de expresiones de ejecución.", "partial", span, node.ID,
			[]string{"La presencia del bucle no demuestra agotamiento de gas."}, []string{"El límite de iteraciones no se prueba sin valores concretos."}, nil, nil)
	case "UncheckedBlock":
		n.addReviewObservation(function, "unchecked-arithmetic", "arithmetic", "Bloque aritmético unchecked", "El bloque desactiva las comprobaciones automáticas de overflow/underflow de Solidity.", "resolved", span, node.ID,
			nil, []string{"Unchecked no constituye por sí solo una vulnerabilidad."}, nil, nil)
	case "BinaryOperation":
		if node.Operator == "/" || node.Operator == "%" {
			n.addReviewObservation(function, "division-rounding", "arithmetic", "División o módulo", "La expresión puede requerir revisar redondeo, denominador y unidades.", "resolved", span, node.ID,
				nil, []string{"No se infiere pérdida económica sin contexto y valores."}, nil, nil)
		}
	case "FunctionCall":
		if node.Kind == "typeConversion" && len(node.Arguments) == 1 {
			fromWidth, fromOK := integerWidth(typeOf(&node.Arguments[0]))
			toWidth, toOK := integerWidth(typeOf(node))
			if fromOK && toOK && toWidth < fromWidth {
				n.addReviewObservation(function, "narrowing-conversion", "type_conversion", "Conversión entera estrecha", "La conversión reduce el ancho entero de "+strconv.Itoa(fromWidth)+" a "+strconv.Itoa(toWidth)+" bits.", "resolved", span, node.ID,
					nil, []string{"La conversión puede ser intencional y estar validada por una comprobación separada."}, nil, nil)
			}
		}
	}
}

func integerWidth(value string) (int, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "type(")
	value = strings.TrimSuffix(value, ")")
	for _, prefix := range []string{"uint", "int"} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		width := strings.TrimPrefix(value, prefix)
		if width == "" {
			return 256, true
		}
		parsed, err := strconv.Atoi(width)
		return parsed, err == nil && parsed >= 8 && parsed <= 256
	}
	return 0, false
}

func (n *normalizer) addFunctionReviewSurface(function model.Function) {
	name := strings.ToLower(function.Name)
	span := function.Span
	switch {
	case strings.Contains(name, "upgrade") || name == "changeadmin":
		n.addReviewObservation(function, "proxy-upgrade-entrypoint", "proxy", "Entrada de actualización o administración", "El nombre y la firma exponen una superficie asociada a actualización o administración de proxy.", "partial", span, function.CompilerID,
			[]string{"La identidad actual de implementación o administrador requiere estado on-chain."}, []string{"El nombre no demuestra por sí solo que el contrato sea un proxy."}, nil, nil)
	case name == "initialize" || strings.HasPrefix(name, "reinitialize"):
		n.addReviewObservation(function, "initialization-entrypoint", "initialization", "Entrada de inicialización", "La función participa potencialmente en el ciclo de inicialización del contrato.", "partial", span, function.CompilerID,
			nil, []string{"La herramienta no demuestra si la inicialización ya ocurrió on-chain."}, nil, nil)
	case strings.Contains(name, "multicall") || strings.Contains(name, "batch"):
		n.addReviewObservation(function, "operation-composition", "composition", "Composición de operaciones", "La entrada puede componer o repetir operaciones y requiere revisar aislamiento y orden.", "partial", span, function.CompilerID,
			nil, []string{"La clasificación por firma se conserva como superficie de revisión, no como hallazgo."}, nil, nil)
	case name == "permit" || strings.Contains(name, "signature"):
		n.addReviewObservation(function, "signature-authorization", "authorization", "Autorización basada en firma", "La entrada sugiere validación de firma, nonce o deadline.", "partial", span, function.CompilerID,
			nil, []string{"La corrección criptográfica no se prueba únicamente por la firma de la función."}, nil, nil)
	}
}

func (n *normalizer) addCallReviewSurface(function model.Function, callSite model.CallSite, call *Node) {
	if callSite.Kind != "external" && callSite.Kind != "low_level" && callSite.Kind != "dynamic" {
		return
	}
	rule, category, title := "external-interaction", "external_interaction", "Interacción externa"
	resolution := callSite.Resolution
	limitations := []string{"La interacción posible no demuestra callback, reentrancia ni ejecución exitosa."}
	switch callSite.Callee {
	case "delegatecall":
		rule, category, title = "delegatecall-context", "proxy", "Delegatecall con contexto del llamador"
	case "staticcall":
		rule, title = "staticcall-interaction", "Consulta externa mediante staticcall"
	case "call":
		rule, title = "dynamic-call", "Llamada de bajo nivel"
	}
	if callSite.Kind == "dynamic" {
		rule, title, resolution = "dynamic-call-target", "Destino de llamada dinámico", "unknown"
	}
	n.addReviewObservation(function, rule, category, title, "El destino o sus efectos no quedan completamente determinados por el código local.", resolution, n.span(call.Src), call.ID,
		[]string{"La identidad real del código requiere resolver el destino en el contexto de despliegue."}, limitations, []string{callSite.ID}, nil)
}

func (n *normalizer) addReviewObservation(function model.Function, rule, category, title, description, resolution string, span *model.Span, nodeID int, assumptions, limitations, relatedCalls, relatedState []string) {
	key := stableID("observation", function.ID, rule, spanKey(span), strconv.Itoa(nodeID))
	if _, exists := n.observationSeen[key]; exists {
		return
	}
	n.observationSeen[key] = struct{}{}
	evidence := n.addEvidence("derived", resolution, description, span, nodeID, limitations)
	n.program.Observations = append(n.program.Observations, model.Observation{
		ID: key, Rule: rule, Category: category, Title: title, Description: description,
		FunctionID: function.ID, Function: function.Symbol, Resolution: resolution, Span: span,
		EvidenceIDs: []string{evidence}, Assumptions: assumptions, Limitations: limitations,
		RelatedCallIDs: relatedCalls, RelatedStateAccessIDs: relatedState,
	})
}

func (n *normalizer) analyzeAssembly(function model.Function, node *Node) {
	n.addReviewObservation(function, "inline-assembly", "assembly", "Assembly requiere revisión manual", "El bloque contiene Yul y puede expresar efectos que no se atribuyen a variables Solidity concretas.", "partial", n.span(node.Src), node.ID,
		nil, []string{"La enumeración de opcodes no equivale a análisis semántico completo de Yul."}, nil, nil)
	if len(node.AST) == 0 {
		return
	}
	var root any
	if err := json.Unmarshal(node.AST, &root); err != nil {
		return
	}
	visitYul(root, func(object map[string]any) {
		nodeType, _ := object["nodeType"].(string)
		opcode := ""
		if nodeType == "YulFunctionCall" {
			if functionName, ok := object["functionName"].(map[string]any); ok {
				opcode, _ = functionName["name"].(string)
			}
		} else {
			switch nodeType {
			case "YulBreak", "YulContinue", "YulLeave", "YulSwitch", "YulForLoop":
				opcode = strings.TrimPrefix(nodeType, "Yul")
			}
		}
		opcode = strings.ToLower(opcode)
		category, relevant := assemblyCategory(opcode)
		if !relevant {
			return
		}
		src, _ := object["src"].(string)
		span := n.span(src)
		arguments := yulArguments(object)
		evidence := n.addEvidence("compiler_fact", "partial", "Operación Yul reconocida en el AST de solc.", span, 0, []string{"Los argumentos se conservan simbólicamente."})
		op := model.AssemblyOperation{
			ID: stableID("assembly", function.ID, opcode, spanKey(span)), FunctionID: function.ID, Function: function.Symbol,
			Opcode: opcode, Category: category, Arguments: arguments, Resolution: "partial", Span: span,
			EvidenceIDs: []string{evidence}, Limitations: []string{"No se atribuyen slots o destinos dinámicos sin evidencia adicional."},
		}
		n.program.AssemblyOperations = append(n.program.AssemblyOperations, op)
		n.addReviewObservation(function, "assembly-"+opcode, "assembly", "Operación Yul "+opcode, "El bloque assembly contiene la operación `"+opcode+"` con argumentos simbólicos.", "partial", span, 0,
			nil, op.Limitations, nil, nil)
		if opcode == "selfdestruct" {
			n.emitAssemblyAsset(function, op, "movement", "native", "selfdestruct", "address(this)", firstString(arguments, "unknown"), "address(this).balance")
		}
	})
}

func visitYul(value any, visit func(map[string]any)) {
	switch current := value.(type) {
	case map[string]any:
		visit(current)
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			visitYul(current[key], visit)
		}
	case []any:
		for _, item := range current {
			visitYul(item, visit)
		}
	}
}

func assemblyCategory(opcode string) (string, bool) {
	switch opcode {
	case "sload", "sstore":
		return "persistent_storage", true
	case "tload", "tstore":
		return "transient_storage", true
	case "call", "delegatecall", "staticcall":
		return "external_interaction", true
	case "create", "create2":
		return "contract_creation", true
	case "selfdestruct":
		return "lifecycle", true
	case "log0", "log1", "log2", "log3", "log4":
		return "log", true
	case "break", "continue", "leave", "switch", "forloop":
		return "control_flow", true
	default:
		return "", false
	}
}

func yulArguments(object map[string]any) []string {
	values, _ := object["arguments"].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, yulText(value))
	}
	return result
}

func yulText(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return "unknown"
	}
	if name, ok := object["name"].(string); ok {
		return name
	}
	if literal, ok := object["value"].(string); ok {
		return literal
	}
	if functionName, ok := object["functionName"].(map[string]any); ok {
		name, _ := functionName["name"].(string)
		return name + "(...)"
	}
	return "<" + firstStringValue(object, "nodeType", "expression") + ">"
}

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return "unknown"
}

func firstString(values []string, fallback string) string {
	if len(values) > 0 && values[0] != "" {
		return values[0]
	}
	return fallback
}

func expressionDependencies(nodes ...*Node) []string {
	values := make(map[string]struct{})
	for _, node := range nodes {
		walk(node, func(candidate *Node) {
			switch candidate.NodeType {
			case "MemberAccess":
				text := expressionText(candidate, 0)
				if strings.HasPrefix(text, "msg.") || strings.HasPrefix(text, "tx.") || strings.HasPrefix(text, "block.") {
					values[text] = struct{}{}
				}
			case "Identifier":
				if candidate.Name != "" && candidate.ReferencedDeclaration > 0 {
					values[candidate.Name] = struct{}{}
				}
			}
		})
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func guardCategory(expression string) string {
	value := strings.ToLower(expression)
	switch {
	case strings.Contains(value, "msg.sender") || strings.Contains(value, "tx.origin") || strings.Contains(value, "owner") || strings.Contains(value, "role"):
		return "access_control"
	case strings.Contains(value, "signature") || strings.Contains(value, "ecrecover") || strings.Contains(value, "signer"):
		return "signature"
	case strings.Contains(value, "nonce"):
		return "nonce"
	case strings.Contains(value, "deadline") || strings.Contains(value, "timestamp") || strings.Contains(value, "block.number") || strings.Contains(value, "timelock"):
		return "time"
	case strings.Contains(value, "pause"):
		return "pause"
	case strings.Contains(value, "initializ"):
		return "initialization"
	case strings.Contains(value, "min") || strings.Contains(value, "max") || strings.Contains(value, "slippage"):
		return "bounds"
	default:
		return "condition"
	}
}

func firstArgument(call *Node) *Node {
	if call == nil || len(call.Arguments) == 0 {
		return nil
	}
	return &call.Arguments[0]
}

func argumentsNode(call *Node) *Node {
	return call
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func successCondition(handling string) string {
	switch handling {
	case "required":
		return "boolean_return_required"
	case "propagated":
		return "return_propagated_to_caller"
	case "try_catch":
		return "try_catch_controls_failure"
	case "reverts_on_failure":
		return "call_reverts_on_failure"
	case "not_applicable":
		return "operation_semantics"
	case "return_value_not_a_success_flag":
		return "returned_amount_or_shares"
	default:
		return "unknown_or_unchecked"
	}
}

func (n *normalizer) emitAssemblyAsset(function model.Function, operation model.AssemblyOperation, category, assetClass, action, from, to, amount string) {
	n.program.AssetOperations = append(n.program.AssetOperations, model.AssetOperation{
		ID:         stableID("asset", function.ID, "assembly", action, spanKey(operation.Span)),
		FunctionID: function.ID, Function: function.Symbol, Category: category, AssetClass: assetClass,
		Operation: action, AssetExpression: "assembly." + action, FromExpression: from, ToExpression: to,
		AmountExpression: amount, Unit: "wei", ClassificationBasis: []string{"yul_opcode=" + action},
		Dependencies: append([]string(nil), operation.Arguments...), ReturnHandling: "not_applicable",
		SuccessCondition: "evm_rules_apply", Cardinality: "single", Resolution: "partial", Span: operation.Span,
		EvidenceIDs: append([]string(nil), operation.EvidenceIDs...),
		Limitations: []string{"La semántica efectiva depende del EVM objetivo y de argumentos assembly potencialmente dinámicos."},
	})
}
