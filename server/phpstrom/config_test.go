package phpstrom

import "testing"

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
