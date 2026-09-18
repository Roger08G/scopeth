package model

type Span struct {
	SourceID    int    `json:"source_id"`
	LogicalPath string `json:"logical_path"`
	Offset      int    `json:"offset_bytes"`
	Length      int    `json:"length_bytes"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
}

type Evidence struct {
	ID          string   `json:"id"`
	Kind        string   `json:"evidence_kind"`
	Resolution  string   `json:"resolution"`
	Description string   `json:"description"`
	Span        *Span    `json:"span,omitempty"`
	NodeID      int      `json:"compiler_node_id,omitempty"`
	Assumptions []string `json:"assumptions,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
}

type SourceUnit struct {
	ID           string `json:"id"`
	LogicalPath  string `json:"logical_path"`
	PhysicalPath string `json:"physical_path,omitempty"`
	SHA256       string `json:"sha256"`
	SizeBytes    int    `json:"size_bytes"`
	CompilerID   int    `json:"compiler_source_id"`
}

type Contract struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Abstract      bool     `json:"abstract"`
	LogicalPath   string   `json:"logical_path"`
	CompilerID    int      `json:"compiler_node_id"`
	Bases         []string `json:"bases,omitempty"`
	Linearized    []int    `json:"linearized_base_ids,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
	Span          *Span    `json:"span,omitempty"`
}

type Parameter struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	StorageLocation string `json:"storage_location,omitempty"`
	Indexed         bool   `json:"indexed,omitempty"`
}

type Function struct {
	ID              string      `json:"id"`
	Symbol          string      `json:"symbol"`
	Name            string      `json:"name"`
	Signature       string      `json:"signature"`
	Kind            string      `json:"kind"`
	ContractID      string      `json:"contract_id,omitempty"`
	ContractName    string      `json:"contract_name,omitempty"`
	LogicalPath     string      `json:"logical_path"`
	Visibility      string      `json:"visibility"`
	StateMutability string      `json:"state_mutability"`
	Payable         bool        `json:"payable"`
	Virtual         bool        `json:"virtual"`
	Implemented     bool        `json:"implemented"`
	Generated       bool        `json:"generated"`
	GeneratedFrom   string      `json:"generated_from,omitempty"`
	Selector        string      `json:"selector,omitempty"`
	Parameters      []Parameter `json:"parameters,omitempty"`
	Returns         []Parameter `json:"returns,omitempty"`
	Modifiers       []string    `json:"modifiers,omitempty"`
	ModifierIDs     []string    `json:"modifier_ids,omitempty"`
	CompilerID      int         `json:"compiler_node_id,omitempty"`
	Span            *Span       `json:"span,omitempty"`
	EvidenceIDs     []string    `json:"evidence_ids,omitempty"`
}

type Modifier struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	ContractID   string      `json:"contract_id,omitempty"`
	ContractName string      `json:"contract_name,omitempty"`
	LogicalPath  string      `json:"logical_path"`
	Parameters   []Parameter `json:"parameters,omitempty"`
	CompilerID   int         `json:"compiler_node_id"`
	Span         *Span       `json:"span,omitempty"`
}

type Variable struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Scope           string `json:"scope"`
	Visibility      string `json:"visibility,omitempty"`
	StorageLocation string `json:"storage_location,omitempty"`
	Constant        bool   `json:"constant"`
	Immutable       bool   `json:"immutable"`
	ContractID      string `json:"contract_id,omitempty"`
	ContractName    string `json:"contract_name,omitempty"`
	LogicalPath     string `json:"logical_path"`
	Slot            string `json:"slot,omitempty"`
	Offset          int    `json:"offset,omitempty"`
	CompilerID      int    `json:"compiler_node_id"`
	Span            *Span  `json:"span,omitempty"`
}

type Event struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Signature    string      `json:"signature"`
	Topic0       string      `json:"topic0,omitempty"`
	Anonymous    bool        `json:"anonymous"`
	ContractID   string      `json:"contract_id,omitempty"`
	ContractName string      `json:"contract_name,omitempty"`
	LogicalPath  string      `json:"logical_path"`
	Parameters   []Parameter `json:"parameters,omitempty"`
	CompilerID   int         `json:"compiler_node_id"`
	Span         *Span       `json:"span,omitempty"`
}

type CustomError struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Signature    string      `json:"signature"`
	Selector     string      `json:"selector,omitempty"`
	ContractID   string      `json:"contract_id,omitempty"`
	ContractName string      `json:"contract_name,omitempty"`
	LogicalPath  string      `json:"logical_path"`
	Parameters   []Parameter `json:"parameters,omitempty"`
	CompilerID   int         `json:"compiler_node_id"`
	Span         *Span       `json:"span,omitempty"`
}

type CallSite struct {
	ID              string           `json:"id"`
	CallerID        string           `json:"caller_id"`
	CallerSymbol    string           `json:"caller_symbol"`
	Kind            string           `json:"kind"`
	CalleeID        string           `json:"callee_id,omitempty"`
	Callee          string           `json:"callee,omitempty"`
	ReceiverType    string           `json:"receiver_type,omitempty"`
	Resolution      string           `json:"resolution"`
	ValueExpression string           `json:"value_expression,omitempty"`
	Span            *Span            `json:"span,omitempty"`
	EvidenceIDs     []string         `json:"evidence_ids,omitempty"`
	StorageBindings []StorageBinding `json:"storage_bindings,omitempty"`
}

type StorageBinding struct {
	ParameterID        string `json:"parameter_id"`
	Parameter          string `json:"parameter"`
	ArgumentRootKind   string `json:"argument_root_kind"`
	ArgumentVariableID string `json:"argument_variable_id,omitempty"`
	ArgumentVariable   string `json:"argument_variable,omitempty"`
	ArgumentPath       string `json:"argument_path,omitempty"`
	ViaAlias           bool   `json:"via_alias"`
	Resolution         string `json:"resolution"`
}

type StorageAlias struct {
	ID               string   `json:"id"`
	FunctionID       string   `json:"function_id"`
	Function         string   `json:"function"`
	AliasID          string   `json:"alias_id"`
	Alias            string   `json:"alias"`
	RootKind         string   `json:"root_kind"`
	TargetVariableID string   `json:"target_variable_id,omitempty"`
	TargetVariable   string   `json:"target_variable,omitempty"`
	Path             string   `json:"path,omitempty"`
	Resolution       string   `json:"resolution"`
	Span             *Span    `json:"span,omitempty"`
	EvidenceIDs      []string `json:"evidence_ids,omitempty"`
}

type StateAccess struct {
	ID          string   `json:"id"`
	FunctionID  string   `json:"function_id"`
	Function    string   `json:"function"`
	VariableID  string   `json:"variable_id"`
	Variable    string   `json:"variable"`
	Access      string   `json:"access"`
	Direct      bool     `json:"direct"`
	RootKind    string   `json:"root_kind"`
	Path        string   `json:"path,omitempty"`
	ViaAlias    bool     `json:"via_alias"`
	AliasID     string   `json:"alias_id,omitempty"`
	Resolution  string   `json:"resolution"`
	Span        *Span    `json:"span,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type StorageEffect struct {
	RootKind   string `json:"root_kind"`
	VariableID string `json:"variable_id"`
	Variable   string `json:"variable"`
	Path       string `json:"path,omitempty"`
	Access     string `json:"access"`
	Resolution string `json:"resolution"`
	ViaAlias   bool   `json:"via_alias"`
}

type Guard struct {
	ID           string   `json:"id"`
	FunctionID   string   `json:"function_id"`
	Function     string   `json:"function"`
	Kind         string   `json:"kind"`
	Category     string   `json:"category"`
	Expression   string   `json:"expression,omitempty"`
	Coverage     string   `json:"coverage"`
	Dependencies []string `json:"dependencies,omitempty"`
	Resolution   string   `json:"resolution"`
	Span         *Span    `json:"span,omitempty"`
	EvidenceIDs  []string `json:"evidence_ids,omitempty"`
}

type AssetOperation struct {
	ID                  string   `json:"id"`
	FunctionID          string   `json:"function_id"`
	Function            string   `json:"function"`
	Category            string   `json:"category"`
	AssetClass          string   `json:"asset_class"`
	Operation           string   `json:"operation"`
	AssetExpression     string   `json:"asset_expression,omitempty"`
	FromExpression      string   `json:"from_expression,omitempty"`
	ToExpression        string   `json:"to_expression,omitempty"`
	AmountExpression    string   `json:"amount_expression,omitempty"`
	Unit                string   `json:"unit"`
	ClassificationBasis []string `json:"classification_basis"`
	Dependencies        []string `json:"dependencies,omitempty"`
	ReturnHandling      string   `json:"return_handling"`
	SuccessCondition    string   `json:"success_condition,omitempty"`
	Cardinality         string   `json:"cardinality"`
	CallbackPotential   bool     `json:"callback_potential"`
	RelatedCallIDs      []string `json:"related_call_ids,omitempty"`
	Resolution          string   `json:"resolution"`
	Span                *Span    `json:"span,omitempty"`
	EvidenceIDs         []string `json:"evidence_ids,omitempty"`
	Limitations         []string `json:"limitations,omitempty"`
	ConditionedBy       []string `json:"conditioned_by_guard_ids,omitempty"`
	RelatedState        []string `json:"related_state_access_ids,omitempty"`
	ReachableFrom       []string `json:"reachable_from,omitempty"`
}

type Observation struct {
	ID                    string   `json:"id"`
	Rule                  string   `json:"rule"`
	Category              string   `json:"category"`
	Title                 string   `json:"title"`
	Description           string   `json:"description"`
	FunctionID            string   `json:"function_id,omitempty"`
	Function              string   `json:"function,omitempty"`
	Resolution            string   `json:"resolution"`
	Span                  *Span    `json:"span,omitempty"`
	EvidenceIDs           []string `json:"evidence_ids,omitempty"`
	Assumptions           []string `json:"assumptions,omitempty"`
	Limitations           []string `json:"limitations,omitempty"`
	RelatedCallIDs        []string `json:"related_call_ids,omitempty"`
	RelatedStateAccessIDs []string `json:"related_state_access_ids,omitempty"`
	ReachableFrom         []string `json:"reachable_from,omitempty"`
}

type ConfigurationChange struct {
	ID            string   `json:"id"`
	FunctionID    string   `json:"function_id"`
	Function      string   `json:"function"`
	VariableID    string   `json:"variable_id"`
	Variable      string   `json:"variable"`
	Path          string   `json:"path"`
	Category      string   `json:"category"`
	ActorModel    string   `json:"actor_model"`
	GuardIDs      []string `json:"guard_ids,omitempty"`
	ReachableFrom []string `json:"reachable_from,omitempty"`
	Resolution    string   `json:"resolution"`
	Span          *Span    `json:"span,omitempty"`
	EvidenceIDs   []string `json:"evidence_ids,omitempty"`
	Limitations   []string `json:"limitations,omitempty"`
}

type AssemblyOperation struct {
	ID          string   `json:"id"`
	FunctionID  string   `json:"function_id"`
	Function    string   `json:"function"`
	Opcode      string   `json:"opcode"`
	Category    string   `json:"category"`
	Arguments   []string `json:"arguments,omitempty"`
	Resolution  string   `json:"resolution"`
	Span        *Span    `json:"span,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
}

type Edge struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Resolution  string   `json:"resolution"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type FunctionSummary struct {
	FunctionID                string          `json:"function_id"`
	Function                  string          `json:"function"`
	DirectReadVariables       []string        `json:"direct_read_variable_ids,omitempty"`
	DirectWriteVariables      []string        `json:"direct_write_variable_ids,omitempty"`
	TransitiveReadVariables   []string        `json:"transitive_read_variable_ids,omitempty"`
	TransitiveWriteVariables  []string        `json:"transitive_write_variable_ids,omitempty"`
	DirectCallIDs             []string        `json:"direct_call_ids,omitempty"`
	TransitiveCallIDs         []string        `json:"transitive_call_ids,omitempty"`
	InternalCallees           []string        `json:"internal_callee_ids,omitempty"`
	DirectAssetOperations     []string        `json:"direct_asset_operation_ids,omitempty"`
	TransitiveAssetOperations []string        `json:"transitive_asset_operation_ids,omitempty"`
	DirectGuardIDs            []string        `json:"direct_guard_ids,omitempty"`
	GuardIDs                  []string        `json:"guard_ids,omitempty"`
	DirectStorageEffects      []StorageEffect `json:"direct_storage_effects,omitempty"`
	TransitiveStorageEffects  []StorageEffect `json:"transitive_storage_effects,omitempty"`
	UnknownEffects            bool            `json:"unknown_effects"`
	Resolution                string          `json:"resolution"`
	Iterations                int             `json:"iterations"`
	SCCID                     string          `json:"scc_id"`
	Recursive                 bool            `json:"recursive"`
	Converged                 bool            `json:"converged"`
}

type IRInstruction struct {
	ID          string   `json:"id"`
	FunctionID  string   `json:"function_id"`
	Function    string   `json:"function"`
	Kind        string   `json:"kind"`
	Target      string   `json:"target,omitempty"`
	Resolution  string   `json:"resolution"`
	Span        *Span    `json:"span,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type Program struct {
	Sources              []SourceUnit          `json:"sources"`
	Contracts            []Contract            `json:"contracts"`
	Functions            []Function            `json:"functions"`
	Modifiers            []Modifier            `json:"modifiers"`
	Variables            []Variable            `json:"variables"`
	Events               []Event               `json:"events"`
	Errors               []CustomError         `json:"errors"`
	Calls                []CallSite            `json:"calls"`
	StorageAliases       []StorageAlias        `json:"storage_aliases"`
	StateAccesses        []StateAccess         `json:"state_accesses"`
	Guards               []Guard               `json:"guards"`
	AssetOperations      []AssetOperation      `json:"asset_operations"`
	ConfigurationChanges []ConfigurationChange `json:"configuration_changes"`
	AssemblyOperations   []AssemblyOperation   `json:"assembly_operations"`
	Observations         []Observation         `json:"observations"`
	Evidence             []Evidence            `json:"evidence"`
	Edges                []Edge                `json:"edges"`
	Summaries            []FunctionSummary     `json:"summaries"`
	IR                   []IRInstruction       `json:"ir"`
}

type SymbolRecord struct {
	SymbolType string       `json:"symbol_type"`
	Contract   *Contract    `json:"contract,omitempty"`
	Function   *Function    `json:"function,omitempty"`
	Modifier   *Modifier    `json:"modifier,omitempty"`
	Variable   *Variable    `json:"variable,omitempty"`
	Event      *Event       `json:"event,omitempty"`
	Error      *CustomError `json:"error,omitempty"`
}
