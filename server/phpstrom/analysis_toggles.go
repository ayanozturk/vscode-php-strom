package phpstrom

import "github.com/ayanozturk/vscode-php-strom/providers"

// AnalysisToggles controls which diagnostic families run. All correctness
// checks default on; style and PSR-1 side-effects default off.
type AnalysisToggles struct {
	SyntaxErrors          bool
	UndefinedSymbols      bool
	UndefinedVariables    bool
	ClassModel            bool
	InvalidCalls          bool
	Language              bool
	TypeErrors            bool
	MethodVisibility      bool
	ThrowTypes            bool
	Deprecated            bool
	UnreachableCode       bool
	EmptyStatements       bool
	AssignmentInCondition bool
	SideEffects           bool
	Style                 bool
}

func DefaultAnalysisToggles() AnalysisToggles {
	return AnalysisToggles{
		SyntaxErrors:          true,
		UndefinedSymbols:      true,
		UndefinedVariables:    true,
		ClassModel:            true,
		InvalidCalls:          true,
		Language:              true,
		TypeErrors:            true,
		MethodVisibility:      true,
		ThrowTypes:            true,
		Deprecated:            true,
		UnreachableCode:       true,
		EmptyStatements:       true,
		AssignmentInCondition: true,
		SideEffects:           false,
		Style:                 false,
	}
}

func (a AnalysisToggles) toProviderDisables() providers.DisabledAnalysis {
	return providers.DisabledAnalysis{
		SyntaxErrors:          !a.SyntaxErrors,
		UndefinedSymbols:      !a.UndefinedSymbols,
		UndefinedVariables:    !a.UndefinedVariables,
		ClassModel:            !a.ClassModel,
		InvalidCalls:          !a.InvalidCalls,
		Language:              !a.Language,
		TypeErrors:            !a.TypeErrors,
		MethodVisibility:      !a.MethodVisibility,
		ThrowTypes:            !a.ThrowTypes,
		Deprecated:            !a.Deprecated,
		UnreachableCode:       !a.UnreachableCode,
		EmptyStatements:       !a.EmptyStatements,
		AssignmentInCondition: !a.AssignmentInCondition,
		SideEffects:           !a.SideEffects,
		Style:                 !a.Style,
	}
}

func applyAnalysisToggles(dst *AnalysisToggles, diagnostics map[string]interface{}) {
	applyLegacyAnalysisToggles(dst, diagnostics)
	if analysis, ok := diagnostics["analysis"].(map[string]interface{}); ok {
		applyAnalysisMap(dst, analysis)
	}
}

func applyFlattenedAnalysisToggles(dst *AnalysisToggles, settings map[string]interface{}) {
	applyBool(&dst.UndefinedSymbols, settings["diagnostics.undefinedSymbols"])
	applyBool(&dst.UndefinedVariables, settings["diagnostics.undefinedVariables"])
	applyBool(&dst.TypeErrors, settings["diagnostics.typeErrors"])
	applyBool(&dst.SyntaxErrors, settings["diagnostics.analysis.syntaxErrors"])
	applyBool(&dst.UndefinedSymbols, settings["diagnostics.analysis.undefinedSymbols"])
	applyBool(&dst.UndefinedVariables, settings["diagnostics.analysis.undefinedVariables"])
	applyBool(&dst.ClassModel, settings["diagnostics.analysis.classModel"])
	applyBool(&dst.InvalidCalls, settings["diagnostics.analysis.invalidCalls"])
	applyBool(&dst.Language, settings["diagnostics.analysis.language"])
	applyBool(&dst.TypeErrors, settings["diagnostics.analysis.typeErrors"])
	applyBool(&dst.MethodVisibility, settings["diagnostics.analysis.methodVisibility"])
	applyBool(&dst.ThrowTypes, settings["diagnostics.analysis.throwTypes"])
	applyBool(&dst.Deprecated, settings["diagnostics.analysis.deprecated"])
	applyBool(&dst.UnreachableCode, settings["diagnostics.analysis.unreachableCode"])
	applyBool(&dst.EmptyStatements, settings["diagnostics.analysis.emptyStatements"])
	applyBool(&dst.AssignmentInCondition, settings["diagnostics.analysis.assignmentInCondition"])
	applyBool(&dst.SideEffects, settings["diagnostics.analysis.sideEffects"])
	applyBool(&dst.Style, settings["diagnostics.analysis.style"])
}

func applyLegacyAnalysisToggles(dst *AnalysisToggles, diagnostics map[string]interface{}) {
	applyBool(&dst.UndefinedSymbols, diagnostics["undefinedSymbols"])
	applyBool(&dst.UndefinedVariables, diagnostics["undefinedVariables"])
	applyBool(&dst.TypeErrors, diagnostics["typeErrors"])
}

func applyAnalysisMap(dst *AnalysisToggles, analysis map[string]interface{}) {
	applyBool(&dst.SyntaxErrors, analysis["syntaxErrors"])
	applyBool(&dst.UndefinedSymbols, analysis["undefinedSymbols"])
	applyBool(&dst.UndefinedVariables, analysis["undefinedVariables"])
	applyBool(&dst.ClassModel, analysis["classModel"])
	applyBool(&dst.InvalidCalls, analysis["invalidCalls"])
	applyBool(&dst.Language, analysis["language"])
	applyBool(&dst.TypeErrors, analysis["typeErrors"])
	applyBool(&dst.MethodVisibility, analysis["methodVisibility"])
	applyBool(&dst.ThrowTypes, analysis["throwTypes"])
	applyBool(&dst.Deprecated, analysis["deprecated"])
	applyBool(&dst.UnreachableCode, analysis["unreachableCode"])
	applyBool(&dst.EmptyStatements, analysis["emptyStatements"])
	applyBool(&dst.AssignmentInCondition, analysis["assignmentInCondition"])
	applyBool(&dst.SideEffects, analysis["sideEffects"])
	applyBool(&dst.Style, analysis["style"])
}

func applyBool(dst *bool, raw interface{}) {
	if v, ok := raw.(bool); ok {
		*dst = v
	}
}

func applyNestedEnable(dst *bool, settings map[string]interface{}, section, key string) {
	if nested, ok := settings[section].(map[string]interface{}); ok {
		if item, ok := nested[key].(map[string]interface{}); ok {
			applyBool(dst, item["enable"])
		}
		applyBool(dst, nested[key+".enable"])
	}
	applyBool(dst, settings[section+"."+key+".enable"])
}
