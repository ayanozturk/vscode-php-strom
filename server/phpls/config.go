package phpls

import (
	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/providers"
)

// Config holds the server configuration derived from VS Code settings.
type Config struct {
	Environment struct {
		PHPVersion   string
		IncludePaths []string
		DocumentRoot string
	}
	Files struct {
		Associations []string
		Exclude      []string
		MaxSize      int64
	}
	Diagnostics struct {
		Enable                   bool
		Run                      string // "onType" | "onSave"
		UndefinedSymbols         bool
		UndefinedVariables       bool
		TypeErrors               bool
		StrictTypes              bool
		RelaxedTypeCheck         bool
		NoMixedTypeCheck         bool
		TypeCheckDocumentedTypes bool
		Exclude                  map[string][]string
	}
	Completion struct {
		InsertUseDeclaration      bool
		FullyQualifyGlobalSymbols bool
		MaxItems                  int
	}
	Format struct {
		BraceStyle   string
		InsertSpaces bool
		TabSize      int
	}
	CodeLens struct {
		References      bool
		Implementations bool
		Overrides       bool
		Parent          bool
		Usages          bool
	}
	InlayHints struct {
		ParameterNames bool
		ParameterTypes bool
		ReturnTypes    bool
	}
	Compatibility struct {
		PreferPsalmPhpstanPrefixedAnnotations bool
	}
	StoragePath       string
	GlobalStoragePath string
	ClearCache        bool
}

func DefaultConfig() *Config {
	c := &Config{}
	c.Environment.PHPVersion = "8.3"
	c.Files.Associations = []string{"**/*.php", "**/*.phtml", "**/*.phar"}
	c.Files.Exclude = []string{"**/.git/**", "**/node_modules/**"}
	c.Files.MaxSize = 1_000_000
	c.Diagnostics.Enable = true
	c.Diagnostics.Run = "onType"
	c.Diagnostics.UndefinedSymbols = true
	c.Diagnostics.UndefinedVariables = true
	c.Diagnostics.TypeErrors = true
	c.Diagnostics.RelaxedTypeCheck = true
	c.Diagnostics.NoMixedTypeCheck = true
	c.Diagnostics.Exclude = map[string][]string{}
	c.Completion.InsertUseDeclaration = true
	c.Completion.MaxItems = 100
	c.Format.BraceStyle = "per"
	c.Format.InsertSpaces = true
	c.Format.TabSize = 4
	c.InlayHints.ParameterNames = true
	c.InlayHints.ParameterTypes = true
	c.InlayHints.ReturnTypes = true
	return c
}

func (c *Config) ApplyInitOptions(opts map[string]interface{}) {
	if v, ok := opts["storagePath"].(string); ok {
		c.StoragePath = v
	}
	if v, ok := opts["globalStoragePath"].(string); ok {
		c.GlobalStoragePath = v
	}
	if v, ok := opts["clearCache"].(bool); ok {
		c.ClearCache = v
	}
}

func (c *Config) Update(settings map[string]interface{}) {
	inner, ok := settings["phpls"].(map[string]interface{})
	if !ok {
		inner = settings
	}
	_ = inner
}

func (c *Config) toIndexerConfig() indexer.Config {
	return indexer.Config{
		Associations: c.Files.Associations,
		Exclude:      c.Files.Exclude,
		MaxSize:      c.Files.MaxSize,
	}
}

func (c *Config) toProviderConfig() providers.Config {
	return providers.Config{
		PHPVersion:              c.Environment.PHPVersion,
		InsertUseDeclaration:    c.Completion.InsertUseDeclaration,
		MaxCompletionItems:      c.Completion.MaxItems,
		DocumentRoot:            c.Environment.DocumentRoot,
		BraceStyle:              c.Format.BraceStyle,
		InsertSpaces:            c.Format.InsertSpaces,
		TabSize:                 c.Format.TabSize,
		CodeLensReferences:      c.CodeLens.References,
		CodeLensImplementations: c.CodeLens.Implementations,
		CodeLensOverrides:       c.CodeLens.Overrides,
		CodeLensParent:          c.CodeLens.Parent,
		InlayHintsParamNames:    c.InlayHints.ParameterNames,
		InlayHintsParamTypes:    c.InlayHints.ParameterTypes,
		InlayHintsReturnTypes:   c.InlayHints.ReturnTypes,
	}
}
