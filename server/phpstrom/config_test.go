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
