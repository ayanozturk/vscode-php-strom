package phpstrom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func TestApplyInitOptionsLoadsDiagnosticsOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyInitOptions(map[string]interface{}{
		"settings": map[string]interface{}{
			"diagnostics": map[string]interface{}{
				"overrides": map[string]interface{}{
					"PSR1.Classes.ClassDeclaration.PascalCase": map[string]interface{}{
						"classes": []interface{}{"/^Legacy_/", "SpecialClass"},
					},
				},
			},
		},
	})

	override, ok := cfg.Diagnostics.Overrides["PSR1.Classes.ClassDeclaration.PascalCase"]
	if !ok {
		t.Fatal("expected diagnostics override to be loaded from init options")
	}
	if len(override.Classes) != 2 {
		t.Fatalf("expected 2 class override patterns, got %d", len(override.Classes))
	}
}

func TestUpdateLoadsFlattenedDiagnosticsOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Update(map[string]interface{}{
		"diagnostics.overrides": map[string]interface{}{
			"PSR1.Classes.ClassDeclaration.PascalCase": map[string]interface{}{
				"classes": []interface{}{`/^RG_.*/`},
			},
		},
	})

	override, ok := cfg.Diagnostics.Overrides["PSR1.Classes.ClassDeclaration.PascalCase"]
	if !ok {
		t.Fatal("expected flattened diagnostics override to be loaded")
	}
	if len(override.Classes) != 1 || override.Classes[0] != `/^RG_.*/` {
		t.Fatalf("unexpected override classes: %#v", override.Classes)
	}
}

func TestUpdateLoadsDiagnosticsExclude(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Update(map[string]interface{}{
		"diagnostics.exclude": map[string]interface{}{
			"**/generated/**": []interface{}{"PSR1.Classes.ClassDeclaration.PascalCase"},
			"**/cache/**":     []interface{}{},
		},
	})

	if len(cfg.Diagnostics.Exclude) != 2 {
		t.Fatalf("expected 2 diagnostics exclusions, got %#v", cfg.Diagnostics.Exclude)
	}
	if got := cfg.Diagnostics.Exclude["**/generated/**"]; len(got) != 1 || got[0] != "PSR1.Classes.ClassDeclaration.PascalCase" {
		t.Fatalf("unexpected diagnostics exclusion codes: %#v", cfg.Diagnostics.Exclude)
	}
	if got := cfg.Diagnostics.Exclude["**/cache/**"]; len(got) != 0 {
		t.Fatalf("expected empty exclusion codes slice for ignore-all rule, got %#v", got)
	}
}

func TestUpdateLoadsDiagnosticToggles(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Update(map[string]interface{}{
		"diagnostics": map[string]interface{}{
			"undefinedSymbols":   false,
			"undefinedVariables": false,
			"typeErrors":         false,
		},
	})

	if cfg.Diagnostics.UndefinedSymbols || cfg.Diagnostics.UndefinedVariables || cfg.Diagnostics.TypeErrors {
		t.Fatalf("expected nested diagnostic toggles to be disabled, got %#v", cfg.Diagnostics)
	}
	providerCfg := cfg.toProviderConfig(nil)
	if !providerCfg.DisableUndefinedSymbols || !providerCfg.DisableUndefinedVariables || !providerCfg.DisableTypeErrors {
		t.Fatalf("expected disabled toggles to reach providers, got %#v", providerCfg)
	}

	cfg.Update(map[string]interface{}{
		"diagnostics.undefinedSymbols":   true,
		"diagnostics.undefinedVariables": true,
		"diagnostics.typeErrors":         true,
	})
	if !cfg.Diagnostics.UndefinedSymbols || !cfg.Diagnostics.UndefinedVariables || !cfg.Diagnostics.TypeErrors {
		t.Fatalf("expected flattened diagnostic toggles to be enabled, got %#v", cfg.Diagnostics)
	}
}

func TestDefaultConfigIncludesVendorTestsExclude(t *testing.T) {
	cfg := DefaultConfig()
	want := "**/vendor/**/{Tests,tests}/**"
	for _, pattern := range cfg.Files.Exclude {
		if pattern == want {
			return
		}
	}
	t.Fatalf("expected default files exclude to contain %q, got %#v", want, cfg.Files.Exclude)
}

func TestUpdateLoadsFilesSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Update(map[string]interface{}{
		"files": map[string]interface{}{
			"exclude": []interface{}{"**/vendor/**", "**/.cache/**"},
			"maxSize": float64(2048),
		},
	})

	if len(cfg.Files.Exclude) != 2 || cfg.Files.Exclude[0] != "**/vendor/**" || cfg.Files.Exclude[1] != "**/.cache/**" {
		t.Fatalf("unexpected files.exclude: %#v", cfg.Files.Exclude)
	}
	if cfg.Files.MaxSize != 2048 {
		t.Fatalf("expected files.maxSize=2048, got %d", cfg.Files.MaxSize)
	}
}

func TestResolvePHPVersionDetectsComposerRequirement(t *testing.T) {
	tmpDir := t.TempDir()
	composer := []byte(`{"require":{"php":">=8.4 <9.0"}}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "composer.json"), composer, 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}

	cfg := DefaultConfig()
	got := cfg.resolvePHPVersion([]indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(tmpDir), Name: "tmp"}})
	if got != "8.4" {
		t.Fatalf("expected composer-detected PHP 8.4, got %q", got)
	}
}

func TestResolvePHPVersionOverrideWinsOverComposer(t *testing.T) {
	tmpDir := t.TempDir()
	composer := []byte(`{"require":{"php":">=8.2 <9.0"}}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "composer.json"), composer, 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Update(map[string]interface{}{
		"environment": map[string]interface{}{
			"phpVersionOverride": "8.5",
		},
	})
	got := cfg.resolvePHPVersion([]indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(tmpDir), Name: "tmp"}})
	if got != "8.5" {
		t.Fatalf("expected override PHP 8.5, got %q", got)
	}
}

func TestResolvePHPVersionUsesComposerPlatformPHP(t *testing.T) {
	tmpDir := t.TempDir()
	composer := []byte(`{"require":{"php":">=8.2"},"config":{"platform":{"php":"8.3.12"}}}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "composer.json"), composer, 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}

	cfg := DefaultConfig()
	got := cfg.resolvePHPVersion([]indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(tmpDir), Name: "tmp"}})
	if got != "8.3" {
		t.Fatalf("expected platform PHP 8.3, got %q", got)
	}
}
