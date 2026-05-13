// Package lsp implements the Language Server Protocol types for phpls.
package lsp

import "encoding/json"

// ─── JSON-RPC 2.0 ────────────────────────────────────────────────────────────

type RequestMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"` // string | number | null
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type ResponseMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  interface{}      `json:"result"`
	Error   *ResponseError   `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type NotificationMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// ─── Core types ──────────────────────────────────────────────────────────────

type URI = string

type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   URI   `json:"uri"`
	Range Range `json:"range"`
}

type LocationLink struct {
	OriginSelectionRange *Range `json:"originSelectionRange,omitempty"`
	TargetURI            URI    `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type TextDocumentIdentifier struct {
	URI URI `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     URI `json:"uri"`
	Version int `json:"version"`
}

type TextDocumentItem struct {
	URI        URI    `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" | "markdown"
	Value string `json:"value"`
}

// ─── Initialize ──────────────────────────────────────────────────────────────

type InitializeParams struct {
	ProcessID             *int                   `json:"processId"`
	RootURI               *URI                   `json:"rootUri"`
	WorkspaceFolders      []WorkspaceFolder      `json:"workspaceFolders"`
	InitializationOptions map[string]interface{} `json:"initializationOptions"`
	Capabilities          ClientCapabilities     `json:"capabilities"`
}

type WorkspaceFolder struct {
	URI  URI    `json:"uri"`
	Name string `json:"name"`
}

type ClientCapabilities struct {
	Workspace    *WorkspaceClientCapabilities    `json:"workspace,omitempty"`
	TextDocument *TextDocumentClientCapabilities `json:"textDocument,omitempty"`
}

type WorkspaceClientCapabilities struct {
	WorkspaceFolders       bool `json:"workspaceFolders"`
	Configuration          bool `json:"configuration"`
	ApplyEdit              bool `json:"applyEdit"`
	DidChangeConfiguration *struct {
		DynamicRegistration bool `json:"dynamicRegistration"`
	} `json:"didChangeConfiguration,omitempty"`
}

type TextDocumentClientCapabilities struct {
	Synchronization *struct {
		DidSave bool `json:"didSave"`
	} `json:"synchronization,omitempty"`
	Completion *struct {
		CompletionItem *struct {
			SnippetSupport      bool                           `json:"snippetSupport"`
			DocumentationFormat []string                       `json:"documentationFormat"`
			ResolveSupport      *struct{ Properties []string } `json:"resolveSupport,omitempty"`
		} `json:"completionItem,omitempty"`
	} `json:"completion,omitempty"`
	Hover *struct {
		ContentFormat []string `json:"contentFormat"`
	} `json:"hover,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ServerCapabilities struct {
	TextDocumentSync                *TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
	CompletionProvider              *CompletionOptions       `json:"completionProvider,omitempty"`
	HoverProvider                   bool                     `json:"hoverProvider,omitempty"`
	SignatureHelpProvider           *SignatureHelpOptions    `json:"signatureHelpProvider,omitempty"`
	DefinitionProvider              bool                     `json:"definitionProvider,omitempty"`
	TypeDefinitionProvider          bool                     `json:"typeDefinitionProvider,omitempty"`
	ImplementationProvider          bool                     `json:"implementationProvider,omitempty"`
	ReferencesProvider              bool                     `json:"referencesProvider,omitempty"`
	DocumentHighlightProvider       bool                     `json:"documentHighlightProvider,omitempty"`
	DocumentSymbolProvider          bool                     `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider         bool                     `json:"workspaceSymbolProvider,omitempty"`
	CodeActionProvider              *CodeActionOptions       `json:"codeActionProvider,omitempty"`
	CodeLensProvider                *CodeLensOptions         `json:"codeLensProvider,omitempty"`
	DocumentLinkProvider            *DocumentLinkOptions     `json:"documentLinkProvider,omitempty"`
	DocumentFormattingProvider      bool                     `json:"documentFormattingProvider,omitempty"`
	DocumentRangeFormattingProvider bool                     `json:"documentRangeFormattingProvider,omitempty"`
	RenameProvider                  *RenameOptions           `json:"renameProvider,omitempty"`
	FoldingRangeProvider            bool                     `json:"foldingRangeProvider,omitempty"`
	SelectionRangeProvider          bool                     `json:"selectionRangeProvider,omitempty"`
	InlayHintProvider               *InlayHintOptions        `json:"inlayHintProvider,omitempty"`
	DeclarationProvider             bool                     `json:"declarationProvider,omitempty"`
	TypeHierarchyProvider           bool                     `json:"typeHierarchyProvider,omitempty"`
	InlineValueProvider             bool                     `json:"inlineValueProvider,omitempty"`
	DiagnosticProvider              *DiagnosticOptions       `json:"diagnosticProvider,omitempty"`
	Workspace                       *WorkspaceCapabilities   `json:"workspace,omitempty"`
}

type TextDocumentSyncOptions struct {
	OpenClose bool `json:"openClose"`
	Change    int  `json:"change"` // 0=none 1=full 2=incremental
	Save      *struct {
		IncludeText bool `json:"includeText"`
	} `json:"save,omitempty"`
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider"`
}

type SignatureHelpOptions struct {
	TriggerCharacters   []string `json:"triggerCharacters,omitempty"`
	RetriggerCharacters []string `json:"retriggerCharacters,omitempty"`
}

type CodeActionOptions struct {
	CodeActionKinds []string `json:"codeActionKinds,omitempty"`
	ResolveProvider bool     `json:"resolveProvider"`
}

type CodeLensOptions struct {
	ResolveProvider bool `json:"resolveProvider"`
}

type DocumentLinkOptions struct {
	ResolveProvider bool `json:"resolveProvider"`
}

type RenameOptions struct {
	PrepareProvider bool `json:"prepareProvider"`
}

type InlayHintOptions struct {
	ResolveProvider bool `json:"resolveProvider"`
}

type DiagnosticOptions struct {
	InterFileDependencies bool `json:"interFileDependencies"`
	WorkspaceDiagnostics  bool `json:"workspaceDiagnostics"`
}

type WorkspaceCapabilities struct {
	WorkspaceFolders *WorkspaceFoldersServerCapabilities `json:"workspaceFolders,omitempty"`
}

type WorkspaceFoldersServerCapabilities struct {
	Supported           bool `json:"supported"`
	ChangeNotifications bool `json:"changeNotifications"`
}

// ─── Text document sync ──────────────────────────────────────────────────────

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"` // nil = full content replace
	Text  string `json:"text"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// ─── Diagnostics ─────────────────────────────────────────────────────────────

type PublishDiagnosticsParams struct {
	URI         URI          `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Diagnostic struct {
	Range    Range       `json:"range"`
	Severity *int        `json:"severity,omitempty"` // 1=Error 2=Warning 3=Info 4=Hint
	Code     interface{} `json:"code,omitempty"`
	Source   string      `json:"source,omitempty"`
	Message  string      `json:"message"`
	Tags     []int       `json:"tags,omitempty"`
}

const (
	DiagSeverityError   = 1
	DiagSeverityWarning = 2
	DiagSeverityInfo    = 3
	DiagSeverityHint    = 4
)

// ─── Completion ───────────────────────────────────────────────────────────────

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *CompletionContext     `json:"context,omitempty"`
}

type CompletionContext struct {
	TriggerKind      int     `json:"triggerKind"`
	TriggerCharacter *string `json:"triggerCharacter,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionItem struct {
	Label               string      `json:"label"`
	Kind                int         `json:"kind,omitempty"`
	Detail              string      `json:"detail,omitempty"`
	Documentation       interface{} `json:"documentation,omitempty"` // string | MarkupContent
	Deprecated          bool        `json:"deprecated,omitempty"`
	Preselect           bool        `json:"preselect,omitempty"`
	SortText            string      `json:"sortText,omitempty"`
	FilterText          string      `json:"filterText,omitempty"`
	InsertText          string      `json:"insertText,omitempty"`
	InsertTextFormat    *int        `json:"insertTextFormat,omitempty"` // 1=plain 2=snippet
	TextEdit            *TextEdit   `json:"textEdit,omitempty"`
	AdditionalTextEdits []TextEdit  `json:"additionalTextEdits,omitempty"`
	Command             *Command    `json:"command,omitempty"`
	Data                interface{} `json:"data,omitempty"`
}

type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

const (
	CompletionItemKindText        = 1
	CompletionItemKindMethod      = 2
	CompletionItemKindFunction    = 3
	CompletionItemKindConstructor = 4
	CompletionItemKindField       = 5
	CompletionItemKindVariable    = 6
	CompletionItemKindClass       = 7
	CompletionItemKindInterface   = 8
	CompletionItemKindModule      = 9
	CompletionItemKindProperty    = 10
	CompletionItemKindUnit        = 11
	CompletionItemKindValue       = 12
	CompletionItemKindEnum        = 13
	CompletionItemKindKeyword     = 14
	CompletionItemKindSnippet     = 15
	CompletionItemKindConstant    = 21
	CompletionItemKindEnumMember  = 20
)

// ─── Hover ───────────────────────────────────────────────────────────────────

type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// ─── Definition / Declaration / TypeDefinition / Implementation ──────────────

type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ─── References ──────────────────────────────────────────────────────────────

type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// ─── Document symbols ────────────────────────────────────────────────────────

type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// ─── Workspace symbols ───────────────────────────────────────────────────────

type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

type WorkspaceSymbol struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	ContainerName string   `json:"containerName,omitempty"`
	Location      Location `json:"location"`
}

// SymbolKind is a named int type for LSP symbol kinds.
type SymbolKind = int

// SymbolInformation represents a symbol found during workspace search.
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	ContainerName string     `json:"containerName,omitempty"`
	Location      Location   `json:"location"`
}

const (
	SymbolKindFile        = 1
	SymbolKindModule      = 2
	SymbolKindNamespace   = 3
	SymbolKindPackage     = 4
	SymbolKindClass       = 5
	SymbolKindMethod      = 6
	SymbolKindProperty    = 7
	SymbolKindField       = 8
	SymbolKindConstructor = 9
	SymbolKindEnum        = 10
	SymbolKindInterface   = 11
	SymbolKindFunction    = 12
	SymbolKindVariable    = 13
	SymbolKindConstant    = 14
	SymbolKindString      = 15
	SymbolKindEnumMember  = 22
	SymbolKindStruct      = 23
)

// ─── Signature help ──────────────────────────────────────────────────────────

type SignatureHelpParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *SignatureHelpContext  `json:"context,omitempty"`
}

type SignatureHelpContext struct {
	TriggerKind      int     `json:"triggerKind"`
	TriggerCharacter *string `json:"triggerCharacter,omitempty"`
	IsRetrigger      bool    `json:"isRetrigger"`
}

type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature *int                   `json:"activeSignature,omitempty"`
	ActiveParameter *int                   `json:"activeParameter,omitempty"`
}

type SignatureInformation struct {
	Label           string                 `json:"label"`
	Documentation   *MarkupContent         `json:"documentation,omitempty"`
	Parameters      []ParameterInformation `json:"parameters,omitempty"`
	ActiveParameter *int                   `json:"activeParameter,omitempty"`
}

type ParameterInformation struct {
	Label         interface{}    `json:"label"` // string | [uint32, uint32]
	Documentation *MarkupContent `json:"documentation,omitempty"`
}

// ─── Document highlight ──────────────────────────────────────────────────────

type DocumentHighlightParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type DocumentHighlight struct {
	Range Range `json:"range"`
	Kind  *int  `json:"kind,omitempty"` // 1=text 2=read 3=write
}

// ─── Rename ──────────────────────────────────────────────────────────────────

type PrepareRenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

type WorkspaceEdit struct {
	Changes         map[URI][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []TextDocumentEdit `json:"documentChanges,omitempty"`
}

type TextDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

// ─── Code action ─────────────────────────────────────────────────────────────

type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
	Data        interface{}    `json:"data,omitempty"`
}

// ─── Code lens ───────────────────────────────────────────────────────────────

type CodeLensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type CodeLens struct {
	Range   Range       `json:"range"`
	Command *Command    `json:"command,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ─── Folding range ───────────────────────────────────────────────────────────

type FoldingRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type FoldingRange struct {
	StartLine      uint32  `json:"startLine"`
	StartCharacter *uint32 `json:"startCharacter,omitempty"`
	EndLine        uint32  `json:"endLine"`
	EndCharacter   *uint32 `json:"endCharacter,omitempty"`
	Kind           string  `json:"kind,omitempty"` // "comment" | "imports" | "region"
}

// ─── Selection range ─────────────────────────────────────────────────────────

type SelectionRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Positions    []Position             `json:"positions"`
}

type SelectionRange struct {
	Range  Range           `json:"range"`
	Parent *SelectionRange `json:"parent,omitempty"`
}

// ─── Inlay hints ─────────────────────────────────────────────────────────────

type InlayHintParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
}

type InlayHint struct {
	Position     Position    `json:"position"`
	Label        interface{} `json:"label"`          // string | InlayHintLabelPart[]
	Kind         *int        `json:"kind,omitempty"` // 1=Type 2=Parameter
	PaddingLeft  bool        `json:"paddingLeft,omitempty"`
	PaddingRight bool        `json:"paddingRight,omitempty"`
}

// ─── Document links ──────────────────────────────────────────────────────────

type DocumentLinkParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DocumentLink struct {
	Range  Range   `json:"range"`
	Target *string `json:"target,omitempty"`
}

// ─── Formatting ──────────────────────────────────────────────────────────────

type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Options      FormattingOptions      `json:"options"`
}

type DocumentRangeFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Options      FormattingOptions      `json:"options"`
}

type FormattingOptions struct {
	TabSize                uint32 `json:"tabSize"`
	InsertSpaces           bool   `json:"insertSpaces"`
	TrimTrailingWhitespace bool   `json:"trimTrailingWhitespace,omitempty"`
	InsertFinalNewline     bool   `json:"insertFinalNewline,omitempty"`
}

// ─── Type hierarchy ──────────────────────────────────────────────────────────

type TypeHierarchyPrepareParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type TypeHierarchyItem struct {
	Name           string      `json:"name"`
	Kind           int         `json:"kind"`
	URI            URI         `json:"uri"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Data           interface{} `json:"data,omitempty"`
}

type TypeHierarchySupertypesParams struct {
	Item TypeHierarchyItem `json:"item"`
}

type TypeHierarchySubtypesParams struct {
	Item TypeHierarchyItem `json:"item"`
}

// ─── Inline values ───────────────────────────────────────────────────────────

type InlineValueParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
}

type InlineValueVariableLookup struct {
	Range               Range   `json:"range"`
	VariableName        *string `json:"variableName,omitempty"`
	CaseSensitiveLookup bool    `json:"caseSensitiveLookup"`
}
