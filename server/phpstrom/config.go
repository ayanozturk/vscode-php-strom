package phpstrom

import (
	"log"

	"github.com/ayanozturk/go-php-parser/overrides"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/providers"
)

// Config holds the server configuration derived from VS Code settings.
type Config struct {
	Environment struct {
		PHPVersion          string
		PHPVersionOverride  string
		EffectivePHPVersion string
		IncludePaths        []string
		DocumentRoot        string
	}
	Files struct {
		Associations []string
		Exclude      []string
		MaxSize      int64
	}
	Stubs       []string
	Diagnostics struct {
		Enable               bool
		Run                  string // "onType" | "onSave"
		WorkspaceScanOnStart bool
		Analysis             AnalysisToggles
		Exclude              map[string][]string
		Overrides            overrides.RuleOverrides
	}
	Completion struct {
		InsertUseDeclaration      bool
		FullyQualifyGlobalSymbols bool
		TriggerParameterHints     bool
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
	ExtensionPath     string
	ClearCache        bool
}

func DefaultConfig() *Config {
	c := &Config{}
	c.Environment.PHPVersion = "auto"
	c.Environment.EffectivePHPVersion = fallbackPHPVersion
	c.Files.Associations = []string{"**/*.php", "**/*.phtml", "**/*.phar"}
	c.Files.Exclude = []string{"**/.git/**", "**/node_modules/**", "**/vendor/**/{Tests,tests}/**"}
	c.Files.MaxSize = 1_000_000
	c.Stubs = []string{"Core", "SPL", "Reflection"}
	c.Diagnostics.Enable = true
	c.Diagnostics.Run = "onType"
	c.Diagnostics.WorkspaceScanOnStart = false
	c.Diagnostics.Analysis = DefaultAnalysisToggles()
	c.Diagnostics.Exclude = map[string][]string{}
	c.Diagnostics.Overrides = overrides.RuleOverrides{}
	c.Completion.InsertUseDeclaration = true
	c.Completion.TriggerParameterHints = true
	c.Completion.MaxItems = 100
	c.Format.BraceStyle = "per"
	c.Format.InsertSpaces = true
	c.Format.TabSize = 4
	c.InlayHints.ParameterNames = true
	return c
}

func (c *Config) ApplyInitOptions(opts map[string]interface{}) {
	if v, ok := opts["storagePath"].(string); ok {
		c.StoragePath = v
	}
	if v, ok := opts["globalStoragePath"].(string); ok {
		c.GlobalStoragePath = v
	}
	if v, ok := opts["extensionPath"].(string); ok {
		c.ExtensionPath = v
	}
	if v, ok := opts["clearCache"].(bool); ok {
		c.ClearCache = v
	}
	if v, ok := opts["settings"].(map[string]interface{}); ok {
		c.Update(v)
	}
}

func (c *Config) Update(settings map[string]interface{}) {
	inner, ok := settings["phpstrom"].(map[string]interface{})
	if !ok {
		inner = settings
	}
	if diagnostics, ok := diagnosticsSection(inner); ok {
		applyBool(&c.Diagnostics.Enable, diagnostics["enable"])
		if v, ok := diagnostics["run"].(string); ok {
			c.Diagnostics.Run = v
		}
		applyBool(&c.Diagnostics.WorkspaceScanOnStart, diagnostics["workspaceScanOnStart"])
		applyAnalysisToggles(&c.Diagnostics.Analysis, diagnostics)
		if overridesMap, ok := parseRuleOverrides(diagnostics["overrides"]); ok {
			c.Diagnostics.Overrides = overridesMap
		}
		if excludeMap, ok := parseDiagnosticsExclude(diagnostics["exclude"]); ok {
			c.Diagnostics.Exclude = excludeMap
		}
	}
	if environment, ok := inner["environment"].(map[string]interface{}); ok {
		applyEnvironmentConfig(&c.Environment, environment)
	}
	if v, ok := inner["environment.phpVersion"].(string); ok {
		c.Environment.PHPVersion = v
	}
	if v, ok := inner["environment.phpVersionOverride"].(string); ok {
		c.Environment.PHPVersionOverride = v
	}
	if files, ok := inner["files"].(map[string]interface{}); ok {
		applyFilesConfig(&c.Files, files)
	}
	if associations, ok := toStringSliceSetting(inner["files.associations"]); ok {
		c.Files.Associations = associations
	}
	if exclude, ok := toStringSliceSetting(inner["files.exclude"]); ok {
		c.Files.Exclude = exclude
	}
	if maxSize, ok := toInt64(inner["files.maxSize"]); ok {
		c.Files.MaxSize = maxSize
	}
	if stubs, ok := toStringSliceSetting(inner["stubs"]); ok {
		c.Stubs = stubs
	}
	if overridesMap, ok := parseRuleOverrides(inner["diagnostics.overrides"]); ok {
		c.Diagnostics.Overrides = overridesMap
	}
	if excludeMap, ok := parseDiagnosticsExclude(inner["diagnostics.exclude"]); ok {
		c.Diagnostics.Exclude = excludeMap
	}
	applyBool(&c.Diagnostics.Enable, inner["diagnostics.enable"])
	if v, ok := inner["diagnostics.run"].(string); ok {
		c.Diagnostics.Run = v
	}
	applyBool(&c.Diagnostics.WorkspaceScanOnStart, inner["diagnostics.workspaceScanOnStart"])
	applyFlattenedAnalysisToggles(&c.Diagnostics.Analysis, inner)
	if completion, ok := inner["completion"].(map[string]interface{}); ok {
		applyBool(&c.Completion.InsertUseDeclaration, completion["insertUseDeclaration"])
		applyBool(&c.Completion.FullyQualifyGlobalSymbols, completion["fullyQualifyGlobalSymbols"])
		applyBool(&c.Completion.TriggerParameterHints, completion["triggerParameterHints"])
		if maxItems, ok := toInt64(completion["maxItems"]); ok {
			c.Completion.MaxItems = int(maxItems)
		}
	}
	applyBool(&c.Completion.InsertUseDeclaration, inner["completion.insertUseDeclaration"])
	applyBool(&c.Completion.FullyQualifyGlobalSymbols, inner["completion.fullyQualifyGlobalSymbols"])
	applyBool(&c.Completion.TriggerParameterHints, inner["completion.triggerParameterHints"])
	if maxItems, ok := toInt64(inner["completion.maxItems"]); ok {
		c.Completion.MaxItems = int(maxItems)
	}
	if format, ok := inner["format"].(map[string]interface{}); ok {
		if v, ok := format["braceStyle"].(string); ok {
			c.Format.BraceStyle = v
		}
		applyBool(&c.Format.InsertSpaces, format["insertSpaces"])
		if tabSize, ok := toInt64(format["tabSize"]); ok {
			c.Format.TabSize = int(tabSize)
		}
	}
	if v, ok := inner["format.braceStyle"].(string); ok {
		c.Format.BraceStyle = v
	}
	applyBool(&c.Format.InsertSpaces, inner["format.insertSpaces"])
	if tabSize, ok := toInt64(inner["format.tabSize"]); ok {
		c.Format.TabSize = int(tabSize)
	}
	applyNestedEnable(&c.CodeLens.References, inner, "codeLens", "references")
	applyNestedEnable(&c.CodeLens.Implementations, inner, "codeLens", "implementations")
	applyNestedEnable(&c.CodeLens.Overrides, inner, "codeLens", "overrides")
	applyNestedEnable(&c.CodeLens.Parent, inner, "codeLens", "parent")
	applyNestedEnable(&c.CodeLens.Usages, inner, "codeLens", "usages")
	applyNestedEnable(&c.InlayHints.ParameterNames, inner, "inlayHints", "parameterNames")
	applyNestedEnable(&c.InlayHints.ParameterTypes, inner, "inlayHints", "parameterTypes")
	applyNestedEnable(&c.InlayHints.ReturnTypes, inner, "inlayHints", "returnTypes")
	if compatibility, ok := inner["compatibility"].(map[string]interface{}); ok {
		applyBool(&c.Compatibility.PreferPsalmPhpstanPrefixedAnnotations, compatibility["preferPsalmPhpstanPrefixedAnnotations"])
	}
	applyBool(&c.Compatibility.PreferPsalmPhpstanPrefixedAnnotations, inner["compatibility.preferPsalmPhpstanPrefixedAnnotations"])
}

func applyEnvironmentConfig(dst *struct {
	PHPVersion          string
	PHPVersionOverride  string
	EffectivePHPVersion string
	IncludePaths        []string
	DocumentRoot        string
}, settings map[string]interface{}) {
	if v, ok := settings["phpVersion"].(string); ok {
		dst.PHPVersion = v
	}
	if v, ok := settings["phpVersionOverride"].(string); ok {
		dst.PHPVersionOverride = v
	}
	if includePaths, ok := toStringSliceSetting(settings["includePaths"]); ok {
		dst.IncludePaths = includePaths
	}
	if v, ok := settings["documentRoot"].(string); ok {
		dst.DocumentRoot = v
	}
}

func applyFilesConfig(dst *struct {
	Associations []string
	Exclude      []string
	MaxSize      int64
}, settings map[string]interface{}) {
	if associations, ok := toStringSliceSetting(settings["associations"]); ok {
		dst.Associations = associations
	}
	if exclude, ok := toStringSliceSetting(settings["exclude"]); ok {
		dst.Exclude = exclude
	}
	if maxSize, ok := toInt64(settings["maxSize"]); ok {
		dst.MaxSize = maxSize
	}
}

func toStringSliceSetting(raw interface{}) ([]string, bool) {
	values, ok := raw.([]interface{})
	if !ok {
		strings, ok := raw.([]string)
		if !ok {
			return nil, false
		}
		return append([]string(nil), strings...), true
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func toInt64(raw interface{}) (int64, bool) {
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		return int64(value), true
	default:
		return 0, false
	}
}

func diagnosticsSection(settings map[string]interface{}) (map[string]interface{}, bool) {
	diagnostics, ok := settings["diagnostics"].(map[string]interface{})
	if ok {
		return diagnostics, true
	}
	return nil, false
}

func (c *Config) toIndexerConfig() indexer.Config {
	return indexer.Config{
		Associations: c.Files.Associations,
		Exclude:      c.Files.Exclude,
		MaxSize:      c.Files.MaxSize,
		Stubs:        c.Stubs,
		PHPVersion:   c.Environment.EffectivePHPVersion,
	}
}

func (c *Config) resolvePHPVersion(folders []indexer.WorkspaceFolder) string {
	c.Environment.EffectivePHPVersion = effectivePHPVersion(
		c.Environment.PHPVersion,
		c.Environment.PHPVersionOverride,
		folders,
	)
	return c.Environment.EffectivePHPVersion
}

func (c *Config) toProviderConfig(folders []indexer.WorkspaceFolder) providers.Config {
	matcher, err := overrides.Compile(c.Diagnostics.Overrides)
	if err != nil {
		log.Printf("[phpstrom] ignoring invalid diagnostics overrides: %v", err)
		matcher = nil
	}

	return providers.Config{
		PHPVersion:              c.Environment.EffectivePHPVersion,
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
		DisabledAnalysis:        c.Diagnostics.Analysis.toProviderDisables(),
		AnalysisLevel:           c.Diagnostics.Analysis.Level,
		DiagnosticsExclusions:   providers.BuildDiagnosticsPathExclusions(c.Diagnostics.Exclude, folders),
		DiagnosticsOverrides:    matcher,
	}
}

func parseDiagnosticsExclude(raw interface{}) (map[string][]string, bool) {
	exclusions, ok := raw.(map[string]interface{})
	if !ok {
		if typed, ok := raw.(map[string][]string); ok {
			cloned := make(map[string][]string, len(typed))
			for pattern, codes := range typed {
				cloned[pattern] = append([]string(nil), codes...)
			}
			return cloned, true
		}
		return nil, false
	}

	parsed := make(map[string][]string, len(exclusions))
	for pattern, rawCodes := range exclusions {
		codes, ok := toStringSliceSetting(rawCodes)
		if !ok {
			continue
		}
		parsed[pattern] = codes
	}
	return parsed, true
}

func parseRuleOverrides(raw interface{}) (overrides.RuleOverrides, bool) {
	overridesMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, false
	}

	parsed := make(overrides.RuleOverrides, len(overridesMap))
	for code, rawOverride := range overridesMap {
		overrideMap, ok := rawOverride.(map[string]interface{})
		if !ok {
			continue
		}
		parsed[code] = overrides.RuleOverride{
			Classes: toStringSlice(overrideMap["classes"]),
		}
	}
	return parsed, true
}

func toStringSlice(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		if strings, ok := raw.([]string); ok {
			return append([]string(nil), strings...)
		}
		return nil
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
}
