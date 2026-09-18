package ast

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
	"github.com/Roger08G/scopeth/internal/model"
)

type Options struct {
	Profile string
	Mode    string
}

type normalizer struct {
	snapshot           input.Snapshot
	output             compiler.Output
	program            model.Program
	sourceByID         map[int]input.Source
	contractByNode     map[int]model.Contract
	functionByNode     map[int]model.Function
	functionNodeByNode map[int]*Node
	variableByNode     map[int]model.Variable
	modifierByNode     map[int]model.Modifier
	modifierNodeByNode map[int]*Node
	eventByNode        map[int]model.Event
	errorByNode        map[int]model.CustomError
	accessSeen         map[string]struct{}
	observationSeen    map[string]struct{}
	profile            string
}

func Normalize(snapshot input.Snapshot, output compiler.Output, options Options) (model.Program, error) {
	n := &normalizer{
		snapshot:           snapshot,
		output:             output,
		sourceByID:         make(map[int]input.Source),
		contractByNode:     make(map[int]model.Contract),
		functionByNode:     make(map[int]model.Function),
		functionNodeByNode: make(map[int]*Node),
		variableByNode:     make(map[int]model.Variable),
		modifierByNode:     make(map[int]model.Modifier),
		modifierNodeByNode: make(map[int]*Node),
		eventByNode:        make(map[int]model.Event),
		errorByNode:        make(map[int]model.CustomError),
		accessSeen:         make(map[string]struct{}),
		observationSeen:    make(map[string]struct{}),
		profile:            options.Profile,
	}
	if n.profile == "" {
		n.profile = "deep"
	}
	sourceByPath := make(map[string]input.Source, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		sourceByPath[source.LogicalPath] = source
		compiled, ok := output.Sources[source.LogicalPath]
		compilerID := -1
		if ok {
			compilerID = compiled.ID
			n.sourceByID[compiled.ID] = source
		}
		n.program.Sources = append(n.program.Sources, model.SourceUnit{
			ID:           stableID("source", source.LogicalPath, source.SHA256),
			LogicalPath:  source.LogicalPath,
			PhysicalPath: source.PhysicalPath,
			SHA256:       source.SHA256,
			SizeBytes:    len(source.Content),
			CompilerID:   compilerID,
		})
	}
	var roots []*Node
	paths := make([]string, 0, len(output.Sources))
	for path := range output.Sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		sourceOutput := output.Sources[path]
		if len(sourceOutput.AST) == 0 {
			continue
		}
		var root Node
		if err := json.Unmarshal(sourceOutput.AST, &root); err != nil {
			return model.Program{}, fmt.Errorf("normalizar AST de %s: %w", path, err)
		}
		roots = append(roots, &root)
		n.indexDeclarations(&root, "", "")
	}
	n.applyStorageLayouts()
	n.addGeneratedGetters()
	if n.profile == "deep" {
		for _, root := range roots {
			n.analyzeBodies(root, "", "")
		}
		n.addModifierApplicationCalls()
	}
	n.addImportAndInheritanceEdges(roots)
	n.sortProgram()
	return n.program, nil
}

func (n *normalizer) indexDeclarations(node *Node, contractID, contractName string) {
	if node == nil {
		return
	}
	span := n.span(node.Src)
	switch node.NodeType {
	case "ContractDefinition":
		logical := logicalOf(span)
		contract := model.Contract{
			ID:            stableID("contract", logical, node.Name, strconv.Itoa(node.ID)),
			Name:          node.Name,
			Kind:          node.ContractKind,
			Abstract:      node.Abstract,
			LogicalPath:   logical,
			CompilerID:    node.ID,
			Linearized:    append([]int(nil), node.LinearizedBaseContracts...),
			Documentation: documentationText(node.Documentation),
			Span:          span,
		}
		for _, base := range node.BaseContracts {
			if base.BaseName != nil {
				contract.Bases = append(contract.Bases, base.BaseName.Name)
			}
		}
		n.contractByNode[node.ID] = contract
		n.program.Contracts = append(n.program.Contracts, contract)
		n.addEvidence("compiler_fact", "resolved", "Declaración de contrato proporcionada por el AST de solc.", span, node.ID, nil)
		contractID, contractName = contract.ID, contract.Name
	case "FunctionDefinition":
		function := n.makeFunction(node, contractID, contractName)
		n.functionByNode[node.ID] = function
		n.functionNodeByNode[node.ID] = node
		n.program.Functions = append(n.program.Functions, function)
	case "ModifierDefinition":
		modifier := model.Modifier{
			ID:           stableID("modifier", logicalOf(span), contractName, node.Name, strconv.Itoa(node.ID)),
			Name:         node.Name,
			ContractID:   contractID,
			ContractName: contractName,
			LogicalPath:  logicalOf(span),
			Parameters:   parameters(node.Parameters),
			CompilerID:   node.ID,
			Span:         span,
		}
		n.modifierByNode[node.ID] = modifier
		n.modifierNodeByNode[node.ID] = node
		n.program.Modifiers = append(n.program.Modifiers, modifier)
		n.addEvidence("compiler_fact", "resolved", "Declaración de modifier proporcionada por solc.", span, node.ID, nil)
	case "VariableDeclaration":
		scope := "local"
		if node.StateVariable {
			scope = "state"
		} else if node.StorageLocation != "" {
			scope = "parameter_or_local"
		}
		variable := model.Variable{
			ID:              stableID("variable", logicalOf(span), contractName, node.Name, strconv.Itoa(node.ID)),
			Name:            node.Name,
			Type:            node.TypeDescriptions.TypeString,
			Scope:           scope,
			Visibility:      node.Visibility,
			StorageLocation: node.StorageLocation,
			Constant:        node.Constant || node.Mutability == "constant",
			Immutable:       node.Mutability == "immutable",
			ContractID:      contractID,
			ContractName:    contractName,
			LogicalPath:     logicalOf(span),
			CompilerID:      node.ID,
			Span:            span,
		}
		n.variableByNode[node.ID] = variable
		n.program.Variables = append(n.program.Variables, variable)
	case "EventDefinition":
		params := parameters(node.Parameters)
		event := model.Event{
			ID: stableID("event", logicalOf(span), contractName, node.Name, strconv.Itoa(node.ID)), Name: node.Name,
			Signature: signature(node.Name, params), Topic0: prefixSelector(node.EventSelector), Anonymous: node.Anonymous, ContractID: contractID,
			ContractName: contractName, LogicalPath: logicalOf(span), Parameters: params, CompilerID: node.ID, Span: span,
		}
		n.program.Events = append(n.program.Events, event)
		n.eventByNode[node.ID] = event
	case "ErrorDefinition":
		params := parameters(node.Parameters)
		customError := model.CustomError{
			ID: stableID("error", logicalOf(span), contractName, node.Name, strconv.Itoa(node.ID)), Name: node.Name,
			Signature: signature(node.Name, params), Selector: prefixSelector(node.ErrorSelector), ContractID: contractID, ContractName: contractName,
			LogicalPath: logicalOf(span), Parameters: params, CompilerID: node.ID, Span: span,
		}
		n.program.Errors = append(n.program.Errors, customError)
		n.errorByNode[node.ID] = customError
	}
	for _, child := range node.children() {
		n.indexDeclarations(child, contractID, contractName)
	}
}

func (n *normalizer) makeFunction(node *Node, contractID, contractName string) model.Function {
	span := n.span(node.Src)
	name := node.Name
	if name == "" {
		name = node.Kind
	}
	params := parameters(node.Parameters)
	returns := parameters(node.ReturnParameters)
	stateMutability := node.StateMutability
	if entry, ok := n.abiEntry(logicalOf(span), contractName, node, len(params)); ok {
		params = abiParameters(entry.Inputs)
		returns = abiParameters(entry.Outputs)
		if entry.StateMutability != "" {
			stateMutability = entry.StateMutability
		}
	}
	sig := signature(name, params)
	symbol := symbol(logicalOf(span), contractName, sig)
	mods := make([]string, 0, len(node.Modifiers))
	for _, invocation := range node.Modifiers {
		if invocation.ModifierName != nil {
			mods = append(mods, invocation.ModifierName.Name)
		}
	}
	evidence := n.addEvidence("compiler_fact", "resolved", "Firma y propiedades declaradas por el AST de solc.", span, node.ID, nil)
	function := model.Function{
		ID: stableID("function", symbol, strconv.Itoa(node.ID)), Symbol: symbol, Name: name, Signature: sig,
		Kind: node.Kind, ContractID: contractID, ContractName: contractName, LogicalPath: logicalOf(span),
		Visibility: node.Visibility, StateMutability: stateMutability, Payable: stateMutability == "payable",
		Virtual: node.Virtual, Implemented: node.Implemented || node.Body != nil, Selector: prefixSelector(node.FunctionSelector),
		Parameters: params, Returns: returns, Modifiers: mods, CompilerID: node.ID, Span: span,
		EvidenceIDs: []string{evidence},
	}
	if function.Payable && (function.Visibility == "public" || function.Visibility == "external" || node.Kind == "receive" || node.Kind == "fallback") {
		n.program.AssetOperations = append(n.program.AssetOperations, model.AssetOperation{
			ID: stableID("asset", function.ID, "payable_entry"), FunctionID: function.ID, Function: function.Symbol,
			Category: "movement", AssetClass: "native", Operation: "payable_entry", FromExpression: "msg.sender",
			ToExpression: "address(this)", AmountExpression: "msg.value", Unit: "wei",
			ClassificationBasis: []string{"state_mutability=payable", "entrypoint_visibility=" + function.Visibility},
			Dependencies:        []string{"msg.sender", "msg.value"}, ReturnHandling: "not_applicable",
			SuccessCondition: "entrypoint_accepts_value", Cardinality: "single", Resolution: "resolved", Span: span,
			EvidenceIDs: []string{evidence}, Limitations: []string{"Payable permite recibir valor, pero no demuestra una ejecución ni un saldo real."},
		})
	}
	for _, modifier := range mods {
		n.program.Guards = append(n.program.Guards, model.Guard{
			ID: stableID("guard", function.ID, "modifier", modifier), FunctionID: function.ID, Function: function.Symbol,
			Kind: "modifier_application", Category: "modifier", Expression: modifier,
			Coverage: "applied_not_path_proven", Resolution: "partial", Span: span,
			EvidenceIDs: []string{evidence},
		})
	}
	return function
}

func (n *normalizer) abiEntry(logical, contractName string, node *Node, parameterCount int) (compiler.ABIEntry, bool) {
	if contractName == "" {
		return compiler.ABIEntry{}, false
	}
	contracts, ok := n.output.Contracts[logical]
	if !ok {
		return compiler.ABIEntry{}, false
	}
	contract, ok := contracts[contractName]
	if !ok {
		return compiler.ABIEntry{}, false
	}
	wantedType := node.Kind
	if wantedType == "" {
		wantedType = "function"
	}
	for _, entry := range contract.ABI {
		if entry.Type != wantedType || len(entry.Inputs) != parameterCount {
			continue
		}
		if wantedType == "function" && entry.Name != node.Name {
			continue
		}
		return entry, true
	}
	return compiler.ABIEntry{}, false
}

func (n *normalizer) applyStorageLayouts() {
	persistent := make(map[int]compiler.StorageEntry)
	transient := make(map[int]compiler.StorageEntry)
	for _, contracts := range n.output.Contracts {
		for _, contract := range contracts {
			for _, entry := range contract.StorageLayout.Storage {
				persistent[entry.ASTID] = entry
			}
			for _, entry := range contract.TransientStorageLayout.Storage {
				transient[entry.ASTID] = entry
			}
		}
	}
	for index := range n.program.Variables {
		variable := &n.program.Variables[index]
		if entry, ok := persistent[variable.CompilerID]; ok {
			variable.Slot, variable.Offset = entry.Slot, entry.Offset
		}
		if entry, ok := transient[variable.CompilerID]; ok {
			variable.Slot, variable.Offset, variable.StorageLocation = entry.Slot, entry.Offset, "transient"
		}
		n.variableByNode[variable.CompilerID] = *variable
	}
}

func (n *normalizer) addGeneratedGetters() {
	existing := make(map[string]struct{})
	for _, function := range n.program.Functions {
		existing[function.ContractID+"|"+function.Name+"|"+strconv.Itoa(len(function.Parameters))] = struct{}{}
	}
	contractByKey := make(map[string]model.Contract)
	for _, contract := range n.program.Contracts {
		contractByKey[contract.LogicalPath+"|"+contract.Name] = contract
	}
	for logical, contracts := range n.output.Contracts {
		for contractName, output := range contracts {
			contract, ok := contractByKey[logical+"|"+contractName]
			if !ok {
				continue
			}
			for _, entry := range output.ABI {
				if entry.Type != "function" {
					continue
				}
				key := contract.ID + "|" + entry.Name + "|" + strconv.Itoa(len(entry.Inputs))
				if _, ok := existing[key]; ok {
					continue
				}
				var sourceVariable *model.Variable
				for index := range n.program.Variables {
					candidate := &n.program.Variables[index]
					if candidate.ContractID == contract.ID && candidate.Name == entry.Name && candidate.Scope == "state" && candidate.Visibility == "public" {
						sourceVariable = candidate
						break
					}
				}
				if sourceVariable == nil {
					continue
				}
				params := abiParameters(entry.Inputs)
				sig := signature(entry.Name, params)
				sym := symbol(logical, contractName, sig)
				evidence := n.addEvidence("compiler_fact", "resolved", "Getter generado presente en el ABI de solc y asociado a una variable public.", sourceVariable.Span, sourceVariable.CompilerID, nil)
				n.program.Functions = append(n.program.Functions, model.Function{
					ID: stableID("getter", sym), Symbol: sym, Name: entry.Name, Signature: sig, Kind: "function",
					ContractID: contract.ID, ContractName: contractName, LogicalPath: logical, Visibility: "external",
					StateMutability: entry.StateMutability, Payable: entry.StateMutability == "payable", Implemented: true,
					Generated: true, GeneratedFrom: sourceVariable.ID, Parameters: params, Returns: abiParameters(entry.Outputs),
					Span: sourceVariable.Span, EvidenceIDs: []string{evidence},
				})
				existing[key] = struct{}{}
			}
		}
	}
}

type storageLocation struct {
	rootKind   string
	variable   model.Variable
	path       string
	resolution string
	viaAlias   bool
	aliasID    string
}

type functionContext struct {
	aliases        map[int]storageLocation
	returnHandling map[int]string
	loopDepth      int
}

func (n *normalizer) analyzeBodies(node *Node, contractID, contractName string) {
	if node == nil {
		return
	}
	if node.NodeType == "ContractDefinition" {
		if contract, ok := n.contractByNode[node.ID]; ok {
			contractID, contractName = contract.ID, contract.Name
		}
	}
	if node.NodeType == "FunctionDefinition" {
		function, ok := n.functionByNode[node.ID]
		if ok && node.Body != nil {
			n.addFunctionReviewSurface(function)
			context := &functionContext{aliases: make(map[int]storageLocation), returnHandling: analyzeReturnHandling(node.Body)}
			n.recordStorageParameters(node, function)
			n.walkFunction(node.Body, function, "read", context)
		}
		return
	}
	if node.NodeType == "ModifierDefinition" {
		modifier, ok := n.modifierByNode[node.ID]
		if ok && node.Body != nil {
			owner := model.Function{
				ID: modifier.ID, Symbol: modifierSymbol(modifier), Name: modifier.Name,
				ContractID: modifier.ContractID, ContractName: modifier.ContractName, LogicalPath: modifier.LogicalPath,
			}
			context := &functionContext{aliases: make(map[int]storageLocation), returnHandling: make(map[int]string)}
			n.recordStorageParameters(node, owner)
			n.walkFunction(node.Body, owner, "read", context)
		}
		return
	}
	for _, child := range node.children() {
		n.analyzeBodies(child, contractID, contractName)
	}
}

func (n *normalizer) addModifierApplicationCalls() {
	nodeIDs := make([]int, 0, len(n.functionNodeByNode))
	for nodeID := range n.functionNodeByNode {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Ints(nodeIDs)
	for _, nodeID := range nodeIDs {
		node := n.functionNodeByNode[nodeID]
		function, ok := n.functionByNode[nodeID]
		if !ok {
			continue
		}
		for index := range node.Modifiers {
			invocation := &node.Modifiers[index]
			if invocation.ModifierName == nil {
				continue
			}
			modifier, resolved := n.modifierByNode[invocation.ModifierName.ReferencedDeclaration]
			if !resolved {
				continue
			}
			span := n.span(invocation.Src)
			evidence := n.addEvidence("compiler_fact", "resolved", "Aplicación de modifier resuelta por referencia del AST.", span, invocation.ID, nil)
			callID := stableID("call", function.ID, "modifier", modifier.ID, strconv.Itoa(invocation.ID))
			callSite := model.CallSite{
				ID: callID, CallerID: function.ID, CallerSymbol: function.Symbol, Kind: "modifier",
				CalleeID: modifier.ID, Callee: modifierSymbol(modifier), Resolution: "resolved",
				Span: span, EvidenceIDs: []string{evidence},
			}
			context := &functionContext{aliases: make(map[int]storageLocation), returnHandling: make(map[int]string)}
			storageArguments := make(map[int]bool)
			if targetNode := n.modifierNodeByNode[invocation.ModifierName.ReferencedDeclaration]; targetNode != nil && targetNode.Parameters != nil {
				for parameterIndex := range targetNode.Parameters.Parameters {
					parameterNode := &targetNode.Parameters.Parameters[parameterIndex]
					if parameterNode.StorageLocation != "storage" || parameterIndex >= len(invocation.Arguments) {
						continue
					}
					storageArguments[parameterIndex] = true
					parameter := n.variableByNode[parameterNode.ID]
					binding := model.StorageBinding{ParameterID: parameter.ID, Parameter: parameter.Name, ArgumentRootKind: "unknown", Resolution: "unknown"}
					if location, ok := n.resolveStorageLocation(&invocation.Arguments[parameterIndex], context); ok {
						binding.ArgumentRootKind = location.rootKind
						binding.ArgumentVariableID = location.variable.ID
						binding.ArgumentVariable = location.variable.Name
						binding.ArgumentPath = location.path
						binding.ViaAlias = location.viaAlias
						binding.Resolution = location.resolution
					}
					callSite.StorageBindings = append(callSite.StorageBindings, binding)
				}
			}
			for argumentIndex := range invocation.Arguments {
				argument := &invocation.Arguments[argumentIndex]
				if storageArguments[argumentIndex] {
					n.walkAddressingExpressions(argument, function, context)
					continue
				}
				n.walkFunction(argument, function, "read", context)
			}
			n.program.Calls = append(n.program.Calls, callSite)
			n.program.Edges = append(n.program.Edges, model.Edge{
				ID: stableID("edge", "modifier", function.ID, modifier.ID), Kind: "modifier",
				From: function.ID, To: modifier.ID, Resolution: "resolved", EvidenceIDs: []string{evidence},
			})
			function.ModifierIDs = append(function.ModifierIDs, modifier.ID)
		}
		sort.Strings(function.ModifierIDs)
		n.functionByNode[nodeID] = function
		for index := range n.program.Functions {
			if n.program.Functions[index].ID == function.ID {
				n.program.Functions[index] = function
				break
			}
		}
	}
}

func modifierSymbol(modifier model.Modifier) string {
	return symbol(modifier.LogicalPath, modifier.ContractName, "modifier "+modifier.Name)
}

func (n *normalizer) recordStorageParameters(node *Node, function model.Function) {
	if node.Parameters == nil {
		return
	}
	for index := range node.Parameters.Parameters {
		parameter := &node.Parameters.Parameters[index]
		if parameter.StorageLocation != "storage" {
			continue
		}
		variable, ok := n.variableByNode[parameter.ID]
		if !ok {
			continue
		}
		evidence := n.addEvidence("compiler_fact", "resolved", "Parámetro formal storage proporcionado por el AST de solc.", n.span(parameter.Src), parameter.ID, nil)
		n.program.StorageAliases = append(n.program.StorageAliases, model.StorageAlias{
			ID: stableID("storage-alias", function.ID, variable.ID, "parameter"), FunctionID: function.ID,
			Function: function.Symbol, AliasID: variable.ID, Alias: variable.Name, RootKind: "storage_parameter",
			TargetVariableID: variable.ID, TargetVariable: variable.Name, Path: variable.Name,
			Resolution: "resolved", Span: n.span(parameter.Src), EvidenceIDs: []string{evidence},
		})
	}
}

func (n *normalizer) walkFunction(node *Node, function model.Function, access string, context *functionContext) {
	if node == nil {
		return
	}
	span := n.span(node.Src)
	n.addNodeReviewSurface(function, node, span)
	if context != nil && (node.NodeType == "ForStatement" || node.NodeType == "WhileStatement" || node.NodeType == "DoWhileStatement") {
		context.loopDepth++
		defer func() { context.loopDepth-- }()
	}
	switch node.NodeType {
	case "Assignment":
		leftAccess := "write"
		if node.Operator != "=" {
			leftAccess = "read_write"
		}
		n.walkLocation(node.LeftHandSide, function, leftAccess, context)
		n.walkFunction(node.RightHandSide, function, "read", context)
		return
	case "UnaryOperation":
		if node.Operator == "++" || node.Operator == "--" || node.Operator == "delete" {
			n.walkLocation(node.SubExpression, function, "read_write", context)
			return
		}
	case "Identifier", "MemberAccess", "IndexAccess":
		if location, ok := n.resolveStorageLocation(node, context); ok {
			n.addStorageAccess(function, location, access, span, node.ID)
			n.walkAddressingExpressions(node, function, context)
			return
		}
	case "FunctionCall":
		storageArguments := n.addCall(function, node, context)
		n.addGuard(function, node)
		n.addAssetOperation(function, node, context)
		if !n.addBuiltinStorageMutation(function, node, context) {
			n.walkFunction(node.Expression, function, "read", context)
		}
		for index := range node.Arguments {
			argument := &node.Arguments[index]
			if storageArguments[index] {
				n.walkAddressingExpressions(argument, function, context)
				continue
			}
			n.walkFunction(argument, function, "read", context)
		}
		return
	case "IfStatement":
		if nodeContains(node.TrueBody, "RevertStatement") || nodeContains(node.FalseBody, "RevertStatement") {
			evidence := n.addEvidence("derived", "partial", "La rama contiene un revert sintácticamente visible.", span, node.ID, []string{"No demuestra cobertura sobre todas las rutas."})
			n.program.Guards = append(n.program.Guards, model.Guard{
				ID: stableID("guard", function.ID, strconv.Itoa(node.ID)), FunctionID: function.ID, Function: function.Symbol,
				Kind: "if_revert", Category: guardCategory(expressionText(node.Condition, 0)), Expression: expressionText(node.Condition, 0),
				Coverage: "branch_local", Dependencies: expressionDependencies(node.Condition), Resolution: "partial", Span: span, EvidenceIDs: []string{evidence},
			})
		}
	case "InlineAssembly":
		n.analyzeAssembly(function, node)
	case "VariableDeclarationStatement":
		if n.recordStorageAlias(node, function, context) {
			return
		}
	}
	for _, child := range node.children() {
		n.walkFunction(child, function, access, context)
	}
}

func (n *normalizer) recordStorageAlias(node *Node, function model.Function, context *functionContext) bool {
	hasStorageAlias := false
	for _, declaration := range node.Declarations {
		if declaration == nil || declaration.StorageLocation != "storage" {
			continue
		}
		hasStorageAlias = true
		variable, exists := n.variableByNode[declaration.ID]
		if !exists {
			continue
		}
		location, resolved := n.resolveStorageLocation(node.InitialValue, context)
		if resolved {
			location.viaAlias = true
			location.aliasID = variable.ID
			context.aliases[declaration.ID] = location
			evidence := n.addEvidence("derived", location.resolution, "Alias local storage resuelto hasta su raíz mediante referencias tipadas del AST.", n.span(node.Src), node.ID, nil)
			n.program.StorageAliases = append(n.program.StorageAliases, model.StorageAlias{
				ID:         stableID("storage-alias", function.ID, variable.ID, location.variable.ID, location.path),
				FunctionID: function.ID, Function: function.Symbol, AliasID: variable.ID, Alias: variable.Name,
				RootKind: location.rootKind, TargetVariableID: location.variable.ID, TargetVariable: location.variable.Name,
				Path: location.path, Resolution: location.resolution, Span: n.span(node.Src), EvidenceIDs: []string{evidence},
			})
			n.walkAddressingExpressions(node.InitialValue, function, context)
			continue
		}
		evidence := n.addEvidence("derived", "unknown", "Alias local storage sin raíz resoluble.", n.span(node.Src), node.ID, []string{"Los efectos dependientes se conservan como desconocidos."})
		n.program.StorageAliases = append(n.program.StorageAliases, model.StorageAlias{
			ID: stableID("storage-alias", function.ID, variable.ID, "unknown"), FunctionID: function.ID,
			Function: function.Symbol, AliasID: variable.ID, Alias: variable.Name, RootKind: "unknown",
			Resolution: "unknown", Span: n.span(node.Src), EvidenceIDs: []string{evidence},
		})
		n.program.Observations = append(n.program.Observations, model.Observation{
			ID: stableID("observation", function.ID, "storage-alias", strconv.Itoa(node.ID)), Rule: "storage-alias-unresolved", Category: "data_flow",
			Title: "Alias de storage no resuelto", Description: "La referencia local existe, pero su raíz no puede atribuirse con la evidencia disponible.",
			FunctionID: function.ID, Function: function.Symbol, Resolution: "unknown", Span: n.span(node.Src),
			EvidenceIDs: []string{evidence}, Limitations: []string{"Unknown no significa ausencia de efectos."},
		})
	}
	return hasStorageAlias
}

func (n *normalizer) walkLocation(node *Node, function model.Function, access string, context *functionContext) {
	if node == nil {
		return
	}
	if location, ok := n.resolveStorageLocation(node, context); ok {
		n.addStorageAccess(function, location, access, n.span(node.Src), node.ID)
		n.walkAddressingExpressions(node, function, context)
		return
	}
	n.walkFunction(node, function, access, context)
}

func (n *normalizer) walkAddressingExpressions(node *Node, function model.Function, context *functionContext) {
	if node == nil {
		return
	}
	switch node.NodeType {
	case "IndexAccess":
		n.walkAddressingExpressions(node.BaseExpression, function, context)
		n.walkFunction(node.IndexExpression, function, "read", context)
	case "MemberAccess":
		n.walkAddressingExpressions(node.Expression, function, context)
	}
}

func (n *normalizer) resolveStorageLocation(node *Node, context *functionContext) (storageLocation, bool) {
	if node == nil {
		return storageLocation{}, false
	}
	switch node.NodeType {
	case "Identifier":
		if context != nil {
			if location, ok := context.aliases[node.ReferencedDeclaration]; ok {
				return location, true
			}
		}
		variable, ok := n.variableByNode[node.ReferencedDeclaration]
		if !ok {
			return storageLocation{}, false
		}
		if variable.Scope == "state" {
			return storageLocation{rootKind: "state", variable: variable, path: variable.Name, resolution: "resolved"}, true
		}
		if variable.StorageLocation == "storage" {
			return storageLocation{rootKind: "storage_parameter", variable: variable, path: variable.Name, resolution: "resolved"}, true
		}
	case "MemberAccess":
		if _, field := n.variableByNode[node.ReferencedDeclaration]; !field && node.MemberName != "length" {
			return storageLocation{}, false
		}
		location, ok := n.resolveStorageLocation(node.Expression, context)
		if !ok {
			return storageLocation{}, false
		}
		location.path = joinStoragePath(location.path, "."+node.MemberName)
		return location, true
	case "IndexAccess":
		location, ok := n.resolveStorageLocation(node.BaseExpression, context)
		if !ok {
			return storageLocation{}, false
		}
		location.path = joinStoragePath(location.path, "["+expressionText(node.IndexExpression, 0)+"]")
		return location, true
	}
	return storageLocation{}, false
}

func (n *normalizer) addBuiltinStorageMutation(function model.Function, call *Node, context *functionContext) bool {
	if call == nil || call.Expression == nil || call.Expression.NodeType != "MemberAccess" {
		return false
	}
	member := call.Expression.MemberName
	if member != "push" && member != "pop" {
		return false
	}
	location, ok := n.resolveStorageLocation(call.Expression.Expression, context)
	if !ok {
		return false
	}
	n.addStorageAccess(function, location, "read_write", n.span(call.Src), call.ID)
	n.walkAddressingExpressions(call.Expression.Expression, function, context)
	return true
}

func joinStoragePath(base, suffix string) string {
	if base == "" {
		return strings.TrimPrefix(suffix, ".")
	}
	return base + suffix
}

func (n *normalizer) addStorageAccess(function model.Function, location storageLocation, access string, span *model.Span, nodeID int) {
	key := strings.Join([]string{function.ID, location.rootKind, location.variable.ID, location.path, access, spanKey(span)}, "|")
	if _, ok := n.accessSeen[key]; ok {
		return
	}
	n.accessSeen[key] = struct{}{}
	kind, description := "compiler_fact", "La referencia del AST apunta a una ubicación de estado."
	if location.viaAlias {
		kind, description = "derived", "Acceso atribuido a la raíz de estado mediante un alias storage resuelto."
	} else if location.rootKind == "storage_parameter" {
		description = "Acceso simbólico mediante un parámetro formal storage."
	}
	evidence := n.addEvidence(kind, location.resolution, description, span, nodeID, nil)
	n.program.StateAccesses = append(n.program.StateAccesses, model.StateAccess{
		ID: stableID("state", key), FunctionID: function.ID, Function: function.Symbol, VariableID: location.variable.ID,
		Variable: location.variable.Name, Access: access, Direct: true, RootKind: location.rootKind,
		Path: location.path, ViaAlias: location.viaAlias, AliasID: location.aliasID,
		Resolution: location.resolution, Span: span, EvidenceIDs: []string{evidence},
	})
}

func (n *normalizer) addCall(function model.Function, call *Node, context *functionContext) map[int]bool {
	storageArguments := make(map[int]bool)
	if call.Kind == "typeConversion" || call.Kind == "structConstructorCall" {
		return storageArguments
	}
	expression := call.Expression
	if expression == nil {
		return storageArguments
	}
	if event, ok := n.eventByNode[expression.ReferencedDeclaration]; ok {
		n.addResolvedSpecialCall(function, call, "emit", event.ID, event.Signature)
		return storageArguments
	}
	if customError, ok := n.errorByNode[expression.ReferencedDeclaration]; ok {
		n.addResolvedSpecialCall(function, call, "revert", customError.ID, customError.Signature)
		return storageArguments
	}
	kind, callee, receiverType, resolution := "dynamic", expressionText(expression, 0), "", "unknown"
	calleeID := ""
	if target, ok := n.functionByNode[expression.ReferencedDeclaration]; ok {
		kind, callee, calleeID, resolution = "internal", target.Symbol, target.ID, "resolved"
	}
	if expression.NodeType == "Identifier" && expression.Name != "" {
		callee = expression.Name
		if expression.Name == "require" || expression.Name == "assert" || expression.Name == "revert" {
			return storageArguments
		}
	}
	if expression.NodeType == "MemberAccess" {
		callee = expression.MemberName
		receiverType = typeOf(expression.Expression)
		lowLevel := expression.MemberName == "call" || expression.MemberName == "delegatecall" || expression.MemberName == "staticcall"
		lowLevel = lowLevel || ((expression.MemberName == "send" || expression.MemberName == "transfer") && strings.Contains(strings.ToLower(receiverType), "address"))
		if lowLevel {
			kind, resolution = "low_level", "partial"
		} else if target, ok := n.functionByNode[expression.ReferencedDeclaration]; ok {
			callee, calleeID = target.Symbol, target.ID
			if internalMemberCall(expression, target) {
				kind, resolution = "internal", "resolved"
			} else {
				kind, resolution = "external", "partial"
			}
		} else {
			kind, resolution = "external", "partial"
		}
	}
	if expression.NodeType == "FunctionCallOptions" && expression.Expression != nil {
		inner := expression.Expression
		if inner.NodeType == "MemberAccess" {
			callee, receiverType, kind, resolution = inner.MemberName, typeOf(inner.Expression), "low_level", "partial"
		}
	}
	span := n.span(call.Src)
	evidence := n.addEvidence("derived", resolution, "Sitio de llamada derivado de referencias y tipos del AST.", span, call.ID, []string{"Un destino externo tipado no demuestra la implementación desplegada."})
	callSite := model.CallSite{
		ID: stableID("call", function.ID, strconv.Itoa(call.ID)), CallerID: function.ID, CallerSymbol: function.Symbol,
		Kind: kind, CalleeID: calleeID, Callee: callee, ReceiverType: receiverType, Resolution: resolution,
		Span: span, EvidenceIDs: []string{evidence},
	}
	if kind == "internal" && calleeID != "" {
		if targetNode := n.functionNodeByNode[expression.ReferencedDeclaration]; targetNode != nil && targetNode.Parameters != nil {
			for index := range targetNode.Parameters.Parameters {
				parameterNode := &targetNode.Parameters.Parameters[index]
				if parameterNode.StorageLocation != "storage" || index >= len(call.Arguments) {
					continue
				}
				storageArguments[index] = true
				parameter := n.variableByNode[parameterNode.ID]
				binding := model.StorageBinding{ParameterID: parameter.ID, Parameter: parameter.Name, ArgumentRootKind: "unknown", Resolution: "unknown"}
				if location, ok := n.resolveStorageLocation(&call.Arguments[index], context); ok {
					binding.ArgumentRootKind = location.rootKind
					binding.ArgumentVariableID = location.variable.ID
					binding.ArgumentVariable = location.variable.Name
					binding.ArgumentPath = location.path
					binding.ViaAlias = location.viaAlias
					binding.Resolution = location.resolution
				}
				callSite.StorageBindings = append(callSite.StorageBindings, binding)
			}
		}
	}
	if expression.NodeType == "FunctionCallOptions" {
		for index, name := range expression.Names {
			if name == "value" && index < len(expression.Options) {
				callSite.ValueExpression = expressionText(&expression.Options[index], 0)
			}
		}
	}
	n.program.Calls = append(n.program.Calls, callSite)
	n.program.Edges = append(n.program.Edges, model.Edge{
		ID: stableID("edge", "call", function.ID, calleeID, callee), Kind: "call", From: function.ID,
		To: firstNonEmpty(calleeID, callee), Resolution: resolution, EvidenceIDs: []string{evidence},
	})
	n.addCallReviewSurface(function, callSite, call)
	return storageArguments
}

func internalMemberCall(expression *Node, target model.Function) bool {
	if expression == nil || expression.Expression == nil {
		return false
	}
	receiver := expression.Expression
	if receiver.Name == "super" || strings.HasPrefix(strings.ToLower(typeOf(receiver)), "type(contract ") {
		return true
	}
	return target.Visibility == "internal" || target.Visibility == "private"
}

func (n *normalizer) addResolvedSpecialCall(function model.Function, call *Node, kind, calleeID, callee string) {
	span := n.span(call.Src)
	evidence := n.addEvidence("compiler_fact", "resolved", "La referencia del AST resuelve el evento o error declarado.", span, call.ID, nil)
	callSite := model.CallSite{
		ID: stableID("call", function.ID, kind, strconv.Itoa(call.ID)), CallerID: function.ID,
		CallerSymbol: function.Symbol, Kind: kind, CalleeID: calleeID, Callee: callee,
		Resolution: "resolved", Span: span, EvidenceIDs: []string{evidence},
	}
	n.program.Calls = append(n.program.Calls, callSite)
	n.program.Edges = append(n.program.Edges, model.Edge{
		ID: stableID("edge", kind, function.ID, calleeID), Kind: kind, From: function.ID,
		To: calleeID, Resolution: "resolved", EvidenceIDs: []string{evidence},
	})
}

func (n *normalizer) addGuard(function model.Function, call *Node) {
	if call.Expression == nil || call.Expression.NodeType != "Identifier" {
		return
	}
	name := call.Expression.Name
	if name != "require" && name != "assert" {
		return
	}
	expression := ""
	if len(call.Arguments) > 0 {
		expression = expressionText(&call.Arguments[0], 0)
	}
	span := n.span(call.Src)
	evidence := n.addEvidence("compiler_fact", "partial", "Comprobación explícita presente en el AST.", span, call.ID, []string{"La presencia de una comprobación no demuestra cobertura de todas las rutas."})
	n.program.Guards = append(n.program.Guards, model.Guard{
		ID: stableID("guard", function.ID, strconv.Itoa(call.ID)), FunctionID: function.ID, Function: function.Symbol,
		Kind: name, Category: guardCategory(expression), Expression: expression, Coverage: "path_local",
		Dependencies: expressionDependencies(firstArgument(call)), Resolution: "partial", Span: span, EvidenceIDs: []string{evidence},
	})
}

func (n *normalizer) addAssetOperation(function model.Function, call *Node, context *functionContext) {
	expression := call.Expression
	if expression == nil {
		return
	}
	member := ""
	receiver := (*Node)(nil)
	options := (*Node)(nil)
	if expression.NodeType == "MemberAccess" {
		member, receiver = expression.MemberName, expression.Expression
	} else if expression.NodeType == "FunctionCallOptions" && expression.Expression != nil && expression.Expression.NodeType == "MemberAccess" {
		options, member, receiver = expression, expression.Expression.MemberName, expression.Expression.Expression
	} else if expression.NodeType == "FunctionCallOptions" && expression.Expression != nil && expression.Expression.NodeType == "NewExpression" {
		options = expression
		if valueIndex := optionIndex(options, "value"); valueIndex >= 0 {
			n.emitAsset(function, call, context, "movement", "native", "create_with_value", "address(this)", expressionText(expression.Expression, 0), expressionText(&options.Options[valueIndex], 0), "partial", "wei", false,
				[]string{"new_expression", "call_option=value"}, "La creación y el constructor de destino pueden revertir; el tipo no implica una dirección desplegada.")
		}
		return
	}
	if expression.NodeType == "Identifier" && expression.Name == "selfdestruct" {
		n.emitAsset(function, call, context, "movement", "native", "selfdestruct", "address(this)", argText(call, 0), "address(this).balance", "partial", "wei", false,
			[]string{"builtin=selfdestruct"}, "Las reglas efectivas dependen del EVM objetivo (incluido EIP-6780).")
		return
	}
	if member == "" {
		return
	}
	receiverType := strings.ToLower(typeOf(receiver))
	contractReceiver := strings.Contains(receiverType, "contract ") || strings.Contains(receiverType, "interface ") || strings.Contains(receiverType, "ierc") || strings.Contains(receiverType, "erc")
	addressReceiver := strings.Contains(receiverType, "address")
	erc20Receiver := strings.Contains(receiverType, "erc20")
	tokenReceiver := erc20Receiver || strings.Contains(receiverType, "721") || strings.Contains(receiverType, "1155") || strings.Contains(receiverType, "token")
	erc4626Receiver := strings.Contains(receiverType, "4626")
	wrappedNativeReceiver := strings.Contains(receiverType, "weth") || strings.Contains(receiverType, "wrapped")
	basis := []string{"receiver_type=" + firstNonEmpty(receiverType, "unknown"), "member=" + member, "argument_count=" + strconv.Itoa(len(call.Arguments))}
	switch {
	case (member == "transfer" || member == "send") && len(call.Arguments) == 1 && addressReceiver:
		n.emitAsset(function, call, context, "movement", "native", "native_"+member, "address(this)", expressionText(receiver, 0), argText(call, 0), "resolved", "wei", false, basis,
			"La operación es potencial; una ejecución puede revertir o no haberse producido.")
	case member == "call" && options != nil && optionIndex(options, "value") >= 0:
		amount := expressionText(&options.Options[optionIndex(options, "value")], 0)
		n.emitAsset(function, call, context, "movement", "native", "call_with_value", "address(this)", expressionText(receiver, 0), amount, "partial", "wei", true, basis,
			"El destino y el resultado efectivo de la llamada pueden ser dinámicos.")
	case member == "transfer" && len(call.Arguments) == 2 && erc20Receiver:
		n.emitAsset(function, call, context, "movement", "erc20", "transfer", "address(this)", argText(call, 0), argText(call, 1), "partial", "token_units_unknown", false, basis,
			"La interfaz tipada apoya la clasificación, pero no prueba el código desplegado en la dirección.")
	case member == "transferFrom" && len(call.Arguments) == 3 && contractReceiver:
		assetClass := "token"
		if strings.Contains(receiverType, "721") {
			assetClass = "erc721"
		} else if strings.Contains(receiverType, "20") {
			assetClass = "erc20"
		}
		unit := "token_units_unknown"
		if assetClass == "erc721" {
			unit = "token_id"
		}
		n.emitAsset(function, call, context, "movement", assetClass, "transferFrom", argText(call, 0), argText(call, 1), argText(call, 2), "partial", unit, assetClass == "erc721", basis,
			"La firma puede ser ambigua entre estándares; se conserva el tipo receptor como evidencia.")
	case (member == "safeTransferFrom" || member == "safeBatchTransferFrom") && contractReceiver:
		assetClass := "token"
		if strings.Contains(receiverType, "1155") || member == "safeBatchTransferFrom" {
			assetClass = "erc1155"
		} else if strings.Contains(receiverType, "721") {
			assetClass = "erc721"
		}
		unit, cardinality := "token_id", "single"
		if assetClass == "erc1155" {
			unit, cardinality = "token_ids_and_amounts", "batch_unknown"
		}
		n.emitAsset(function, call, context, "movement", assetClass, member, argText(call, 0), argText(call, 1), argText(call, 2), "partial", unit, true, append(basis, "cardinality="+cardinality),
			"Callbacks y efectos del receptor no se resuelven por completo.")
	case (member == "approve" || member == "increaseAllowance" || member == "decreaseAllowance" || member == "permit" || member == "setApprovalForAll") && tokenReceiver:
		assetClass := "token"
		if erc20Receiver {
			assetClass = "erc20"
		} else if strings.Contains(receiverType, "721") {
			assetClass = "erc721"
		}
		n.emitAsset(function, call, context, "authorization", assetClass, member, expressionText(receiver, 0), argText(call, 0), argText(call, 1), "partial", "token_units_or_approval", false, basis,
			"Autorizar no equivale a transferir activos.")
	case erc4626Receiver && (member == "deposit" || member == "mint" || member == "withdraw" || member == "redeem"):
		unit := "assets"
		if member == "mint" || member == "redeem" {
			unit = "shares"
		}
		n.emitAsset(function, call, context, "movement", "erc4626", member, "symbolic", argText(call, 1), argText(call, 0), "partial", unit, false, basis,
			"Assets y shares se mantienen separados; previews, límites y redondeo requieren revisión adicional.")
	case wrappedNativeReceiver && ((member == "deposit" && options != nil && optionIndex(options, "value") >= 0) || member == "wrap"):
		amount := "msg.value"
		if options != nil && optionIndex(options, "value") >= 0 {
			amount = expressionText(&options.Options[optionIndex(options, "value")], 0)
		}
		n.emitAsset(function, call, context, "movement", "wrapped_native", "wrap", "address(this)", expressionText(receiver, 0), amount, "partial", "wei", false, basis,
			"La equivalencia entre activo nativo y wrapper depende del contrato de destino.")
	case wrappedNativeReceiver && (member == "withdraw" || member == "unwrap"):
		n.emitAsset(function, call, context, "movement", "wrapped_native", "unwrap", expressionText(receiver, 0), "address(this)", argText(call, 0), "partial", "wei", true, basis,
			"La recepción de valor puede ejecutar código del receptor o revertir.")
	case (member == "mint" || member == "burn") && tokenReceiver:
		n.emitAsset(function, call, context, "accounting", "token_supply", member, "symbolic", "symbolic", argText(call, len(call.Arguments)-1), "partial", "token_units_unknown", false, basis,
			"La clasificación requiere un receptor de tipo contrato; no demuestra una variación on-chain.")
	case (member == "safeTransfer" || member == "safeTransferFrom") && erc20Receiver:
		if _, resolvedWrapper := n.functionByNode[expression.ReferencedDeclaration]; !resolvedWrapper {
			n.emitAsset(function, call, context, "movement", "erc20", member, "address(this)", argText(call, 0), argText(call, 1), "partial", "token_units_unknown", false, append(basis, "wrapper=unresolved"),
				"El wrapper no se resolvió a una implementación local; no se contabiliza una operación subyacente adicional.")
		}
	}
}

func (n *normalizer) emitAsset(function model.Function, call *Node, context *functionContext, category, assetClass, operation, from, to, amount, resolution, unit string, callback bool, basis []string, limitation string) {
	span := n.span(call.Src)
	evidence := n.addEvidence("derived", resolution, "Operación de activos clasificada usando el tipo y la forma de la llamada en el AST.", span, call.ID, []string{limitation})
	handling := "unchecked_or_unknown"
	if context != nil && context.returnHandling[call.ID] != "" {
		handling = context.returnHandling[call.ID]
	}
	if operation == "native_transfer" {
		handling = "reverts_on_failure"
	} else if operation == "selfdestruct" || operation == "create_with_value" {
		handling = "not_applicable"
	} else if operation == "safeTransferFrom" || operation == "safeBatchTransferFrom" || operation == "setApprovalForAll" || operation == "permit" {
		handling = "not_applicable"
	} else if assetClass == "erc4626" {
		handling = "return_value_not_a_success_flag"
	}
	cardinality := "single"
	if operation == "safeBatchTransferFrom" {
		cardinality = "batch_unknown"
	} else if context != nil && context.loopDepth > 0 {
		cardinality = "loop_dependent_unknown"
	}
	callID := stableID("call", function.ID, strconv.Itoa(call.ID))
	n.program.AssetOperations = append(n.program.AssetOperations, model.AssetOperation{
		ID: stableID("asset", function.ID, operation, strconv.Itoa(call.ID)), FunctionID: function.ID, Function: function.Symbol,
		Category: category, AssetClass: assetClass, Operation: operation, AssetExpression: expressionText(call.Expression, 0),
		FromExpression: from, ToExpression: to, AmountExpression: amount, Unit: unit,
		ClassificationBasis: uniqueStrings(basis), Dependencies: expressionDependencies(call.Expression, argumentsNode(call)),
		ReturnHandling: handling, SuccessCondition: successCondition(handling), Cardinality: cardinality,
		CallbackPotential: callback, RelatedCallIDs: []string{callID}, Resolution: resolution, Span: span,
		EvidenceIDs: []string{evidence}, Limitations: []string{limitation},
	})
}

func (n *normalizer) addImportAndInheritanceEdges(roots []*Node) {
	contractsByCompilerID := n.contractByNode
	for _, root := range roots {
		walk(root, func(node *Node) {
			switch node.NodeType {
			case "ImportDirective":
				span := n.span(node.Src)
				resolution := "partial"
				for _, source := range n.snapshot.Sources {
					if source.LogicalPath == node.AbsolutePath {
						resolution = "resolved"
						break
					}
				}
				evidence := n.addEvidence("compiler_fact", resolution, "Directiva import proporcionada por solc.", span, node.ID, nil)
				n.program.Edges = append(n.program.Edges, model.Edge{ID: stableID("edge", "import", logicalOf(span), node.AbsolutePath), Kind: "import", From: logicalOf(span), To: node.AbsolutePath, Resolution: resolution, EvidenceIDs: []string{evidence}})
			case "ContractDefinition":
				derived, ok := contractsByCompilerID[node.ID]
				if !ok {
					return
				}
				for _, base := range node.BaseContracts {
					if base.BaseName == nil {
						continue
					}
					target := base.BaseName.Name
					resolution := "partial"
					if resolved, ok := contractsByCompilerID[base.BaseName.ReferencedDeclaration]; ok {
						target, resolution = resolved.ID, "resolved"
					}
					evidence := n.addEvidence("compiler_fact", resolution, "Relación de herencia extraída del AST.", n.span(base.Src), base.ID, nil)
					n.program.Edges = append(n.program.Edges, model.Edge{ID: stableID("edge", "inheritance", derived.ID, target), Kind: "inheritance", From: derived.ID, To: target, Resolution: resolution, EvidenceIDs: []string{evidence}})
				}
			}
		})
	}
}

func (n *normalizer) addEvidence(kind, resolution, description string, span *model.Span, nodeID int, limitations []string) string {
	id := stableID("evidence", kind, resolution, description, spanKey(span), strconv.Itoa(nodeID))
	n.program.Evidence = append(n.program.Evidence, model.Evidence{
		ID: id, Kind: kind, Resolution: resolution, Description: description, Span: span,
		NodeID: nodeID, Limitations: limitations,
	})
	return id
}

func (n *normalizer) span(src string) *model.Span {
	parts := strings.Split(src, ":")
	if len(parts) != 3 {
		return nil
	}
	offset, errOffset := strconv.Atoi(parts[0])
	length, errLength := strconv.Atoi(parts[1])
	sourceID, errSource := strconv.Atoi(parts[2])
	if errOffset != nil || errLength != nil || errSource != nil || offset < 0 || length < 0 {
		return nil
	}
	source, ok := n.sourceByID[sourceID]
	if !ok {
		return &model.Span{SourceID: sourceID, Offset: offset, Length: length}
	}
	if offset > len(source.Content) {
		offset = len(source.Content)
	}
	prefix := source.Content[:offset]
	line := bytes.Count(prefix, []byte{'\n'}) + 1
	last := bytes.LastIndexByte(prefix, '\n')
	column := offset + 1
	if last >= 0 {
		column = offset - last
	}
	return &model.Span{SourceID: sourceID, LogicalPath: source.LogicalPath, Offset: offset, Length: length, Line: line, Column: column}
}

func (n *normalizer) sortProgram() {
	sort.Slice(n.program.Sources, func(i, j int) bool { return n.program.Sources[i].LogicalPath < n.program.Sources[j].LogicalPath })
	sort.Slice(n.program.Contracts, func(i, j int) bool { return n.program.Contracts[i].ID < n.program.Contracts[j].ID })
	sort.Slice(n.program.Functions, func(i, j int) bool { return n.program.Functions[i].Symbol < n.program.Functions[j].Symbol })
	sort.Slice(n.program.Modifiers, func(i, j int) bool { return n.program.Modifiers[i].ID < n.program.Modifiers[j].ID })
	sort.Slice(n.program.Variables, func(i, j int) bool { return n.program.Variables[i].ID < n.program.Variables[j].ID })
	sort.Slice(n.program.Events, func(i, j int) bool { return n.program.Events[i].ID < n.program.Events[j].ID })
	sort.Slice(n.program.Errors, func(i, j int) bool { return n.program.Errors[i].ID < n.program.Errors[j].ID })
	sort.Slice(n.program.Calls, func(i, j int) bool { return n.program.Calls[i].ID < n.program.Calls[j].ID })
	sort.Slice(n.program.StorageAliases, func(i, j int) bool { return n.program.StorageAliases[i].ID < n.program.StorageAliases[j].ID })
	sort.Slice(n.program.StateAccesses, func(i, j int) bool { return n.program.StateAccesses[i].ID < n.program.StateAccesses[j].ID })
	sort.Slice(n.program.Guards, func(i, j int) bool { return n.program.Guards[i].ID < n.program.Guards[j].ID })
	sort.Slice(n.program.AssetOperations, func(i, j int) bool { return n.program.AssetOperations[i].ID < n.program.AssetOperations[j].ID })
	sort.Slice(n.program.ConfigurationChanges, func(i, j int) bool {
		return n.program.ConfigurationChanges[i].ID < n.program.ConfigurationChanges[j].ID
	})
	sort.Slice(n.program.AssemblyOperations, func(i, j int) bool { return n.program.AssemblyOperations[i].ID < n.program.AssemblyOperations[j].ID })
	sort.Slice(n.program.Observations, func(i, j int) bool { return n.program.Observations[i].ID < n.program.Observations[j].ID })
	sort.Slice(n.program.Evidence, func(i, j int) bool { return n.program.Evidence[i].ID < n.program.Evidence[j].ID })
	sort.Slice(n.program.Edges, func(i, j int) bool { return n.program.Edges[i].ID < n.program.Edges[j].ID })
}

func parameters(node *ParameterList) []model.Parameter {
	if node == nil {
		return nil
	}
	result := make([]model.Parameter, 0, len(node.Parameters))
	for _, parameter := range node.Parameters {
		result = append(result, model.Parameter{Name: parameter.Name, Type: cleanType(parameter.TypeDescriptions.TypeString), StorageLocation: parameter.StorageLocation, Indexed: parameter.Indexed})
	}
	return result
}

func abiParameters(values []compiler.ABIParameter) []model.Parameter {
	result := make([]model.Parameter, 0, len(values))
	for _, value := range values {
		result = append(result, model.Parameter{Name: value.Name, Type: value.Type})
	}
	return result
}

func signature(name string, params []model.Parameter) string {
	types := make([]string, 0, len(params))
	for _, parameter := range params {
		types = append(types, cleanType(parameter.Type))
	}
	return name + "(" + strings.Join(types, ",") + ")"
}

func cleanType(value string) string {
	value = strings.TrimSpace(value)
	for _, suffix := range []string{" storage ref", " storage pointer", " memory", " calldata"} {
		value = strings.ReplaceAll(value, suffix, "")
	}
	return value
}

func symbol(logical, contractName, sig string) string {
	if contractName == "" {
		return logical + ":" + sig
	}
	return logical + ":" + contractName + "." + sig
}

func stableID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}

func prefixSelector(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "0x") {
		return value
	}
	return "0x" + value
}

func logicalOf(span *model.Span) string {
	if span == nil {
		return ""
	}
	return span.LogicalPath
}

func spanKey(span *model.Span) string {
	if span == nil {
		return "generated"
	}
	return fmt.Sprintf("%s:%d:%d", span.LogicalPath, span.Offset, span.Length)
}

func documentationText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var object struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.Text
	}
	return ""
}

func expressionText(node *Node, depth int) string {
	if node == nil {
		return ""
	}
	if depth > 8 {
		return "<expresión>"
	}
	switch node.NodeType {
	case "Identifier", "IdentifierPath":
		return node.Name
	case "Literal":
		if node.Value != "" {
			return node.Value
		}
		return node.HexValue
	case "MemberAccess":
		base := expressionText(node.Expression, depth+1)
		if base == "" {
			return node.MemberName
		}
		return base + "." + node.MemberName
	case "IndexAccess":
		return expressionText(node.BaseExpression, depth+1) + "[" + expressionText(node.IndexExpression, depth+1) + "]"
	case "BinaryOperation", "Assignment":
		left, right := node.LeftExpression, node.RightExpression
		if node.NodeType == "Assignment" {
			left, right = node.LeftHandSide, node.RightHandSide
		}
		return expressionText(left, depth+1) + " " + node.Operator + " " + expressionText(right, depth+1)
	case "UnaryOperation":
		return node.Operator + expressionText(node.SubExpression, depth+1)
	case "FunctionCall":
		args := make([]string, 0, len(node.Arguments))
		for index := range node.Arguments {
			args = append(args, expressionText(&node.Arguments[index], depth+1))
		}
		return expressionText(node.Expression, depth+1) + "(" + strings.Join(args, ", ") + ")"
	case "FunctionCallOptions":
		return expressionText(node.Expression, depth+1) + "{...}"
	case "Conditional":
		return expressionText(node.Condition, depth+1) + " ? " + expressionText(node.TrueExpression, depth+1) + " : " + expressionText(node.FalseExpression, depth+1)
	default:
		if node.Name != "" {
			return node.Name
		}
		return "<" + node.NodeType + ">"
	}
}

func typeOf(node *Node) string {
	if node == nil {
		return ""
	}
	return node.TypeDescriptions.TypeString
}

func argText(call *Node, index int) string {
	if call == nil || index < 0 || index >= len(call.Arguments) {
		return "unknown"
	}
	return expressionText(&call.Arguments[index], 0)
}

func optionIndex(node *Node, name string) int {
	for index, candidate := range node.Names {
		if candidate == name && index < len(node.Options) {
			return index
		}
	}
	return -1
}

func nodeContains(node *Node, nodeType string) bool {
	found := false
	walk(node, func(candidate *Node) {
		if candidate.NodeType == nodeType {
			found = true
		}
	})
	return found
}

func walk(node *Node, visit func(*Node)) {
	if node == nil {
		return
	}
	visit(node)
	for _, child := range node.children() {
		walk(child, visit)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}
