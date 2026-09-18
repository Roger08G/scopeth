package ast

import "encoding/json"

type TypeDescriptions struct {
	TypeIdentifier string `json:"typeIdentifier"`
	TypeString     string `json:"typeString"`
}

type ParameterList struct {
	ID         int    `json:"id"`
	NodeType   string `json:"nodeType"`
	Src        string `json:"src"`
	Parameters []Node `json:"parameters"`
}

type Node struct {
	ID                      int              `json:"id"`
	NodeType                string           `json:"nodeType"`
	Src                     string           `json:"src"`
	Name                    string           `json:"name"`
	AbsolutePath            string           `json:"absolutePath"`
	ContractKind            string           `json:"contractKind"`
	Kind                    string           `json:"kind"`
	Visibility              string           `json:"visibility"`
	StateMutability         string           `json:"stateMutability"`
	StorageLocation         string           `json:"storageLocation"`
	Mutability              string           `json:"mutability"`
	Operator                string           `json:"operator"`
	MemberName              string           `json:"memberName"`
	Value                   string           `json:"value"`
	HexValue                string           `json:"hexValue"`
	FunctionSelector        string           `json:"functionSelector"`
	EventSelector           string           `json:"eventSelector"`
	ErrorSelector           string           `json:"errorSelector"`
	ReferencedDeclaration   int              `json:"referencedDeclaration"`
	Scope                   int              `json:"scope"`
	SourceUnit              int              `json:"sourceUnit"`
	Abstract                bool             `json:"abstract"`
	Anonymous               bool             `json:"anonymous"`
	Constant                bool             `json:"constant"`
	StateVariable           bool             `json:"stateVariable"`
	Virtual                 bool             `json:"virtual"`
	Implemented             bool             `json:"implemented"`
	Indexed                 bool             `json:"indexed"`
	Prefix                  bool             `json:"prefix"`
	TryCall                 bool             `json:"tryCall"`
	TypeDescriptions        TypeDescriptions `json:"typeDescriptions"`
	Documentation           json.RawMessage  `json:"documentation"`
	LinearizedBaseContracts []int            `json:"linearizedBaseContracts"`

	Nodes         []Node   `json:"nodes"`
	Statements    []Node   `json:"statements"`
	Declarations  []*Node  `json:"declarations"`
	Components    []*Node  `json:"components"`
	Arguments     []Node   `json:"arguments"`
	Modifiers     []Node   `json:"modifiers"`
	BaseContracts []Node   `json:"baseContracts"`
	Members       []Node   `json:"members"`
	Clauses       []Node   `json:"clauses"`
	Options       []Node   `json:"options"`
	Names         []string `json:"names"`
	Assignments   []any    `json:"assignments"`

	Body                     *Node           `json:"body"`
	Parameters               *ParameterList  `json:"parameters"`
	ReturnParameters         *ParameterList  `json:"returnParameters"`
	Expression               *Node           `json:"expression"`
	LeftHandSide             *Node           `json:"leftHandSide"`
	RightHandSide            *Node           `json:"rightHandSide"`
	LeftExpression           *Node           `json:"leftExpression"`
	RightExpression          *Node           `json:"rightExpression"`
	SubExpression            *Node           `json:"subExpression"`
	BaseExpression           *Node           `json:"baseExpression"`
	IndexExpression          *Node           `json:"indexExpression"`
	Condition                *Node           `json:"condition"`
	TrueBody                 *Node           `json:"trueBody"`
	FalseBody                *Node           `json:"falseBody"`
	TrueExpression           *Node           `json:"trueExpression"`
	FalseExpression          *Node           `json:"falseExpression"`
	InitializationExpression *Node           `json:"initializationExpression"`
	LoopExpression           *Node           `json:"loopExpression"`
	InitialValue             *Node           `json:"initialValue"`
	EventCall                *Node           `json:"eventCall"`
	ErrorCall                *Node           `json:"errorCall"`
	ExternalCall             *Node           `json:"externalCall"`
	ModifierName             *Node           `json:"modifierName"`
	BaseName                 *Node           `json:"baseName"`
	AST                      json.RawMessage `json:"AST"`
}

func (n *Node) children() []*Node {
	var result []*Node
	for index := range n.Nodes {
		result = append(result, &n.Nodes[index])
	}
	for index := range n.Statements {
		result = append(result, &n.Statements[index])
	}
	for _, child := range n.Declarations {
		if child != nil {
			result = append(result, child)
		}
	}
	for _, child := range n.Components {
		if child != nil {
			result = append(result, child)
		}
	}
	for index := range n.Arguments {
		result = append(result, &n.Arguments[index])
	}
	for index := range n.Modifiers {
		result = append(result, &n.Modifiers[index])
	}
	for index := range n.BaseContracts {
		result = append(result, &n.BaseContracts[index])
	}
	for index := range n.Members {
		result = append(result, &n.Members[index])
	}
	for index := range n.Clauses {
		result = append(result, &n.Clauses[index])
	}
	for index := range n.Options {
		result = append(result, &n.Options[index])
	}
	for index := range parameterNodes(n.Parameters) {
		result = append(result, &n.Parameters.Parameters[index])
	}
	for index := range parameterNodes(n.ReturnParameters) {
		result = append(result, &n.ReturnParameters.Parameters[index])
	}
	pointers := []*Node{
		n.Body, n.Expression, n.LeftHandSide,
		n.RightHandSide, n.LeftExpression, n.RightExpression, n.SubExpression,
		n.BaseExpression, n.IndexExpression, n.Condition, n.TrueBody, n.FalseBody,
		n.TrueExpression, n.FalseExpression, n.InitializationExpression,
		n.LoopExpression, n.InitialValue, n.EventCall, n.ErrorCall, n.ExternalCall,
		n.ModifierName, n.BaseName,
	}
	for _, child := range pointers {
		if child != nil {
			result = append(result, child)
		}
	}
	return result
}

func parameterNodes(list *ParameterList) []Node {
	if list == nil {
		return nil
	}
	return list.Parameters
}
