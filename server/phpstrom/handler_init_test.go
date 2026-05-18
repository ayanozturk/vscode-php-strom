package phpstrom

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestInitializeAppliesDiagnosticsOverridesToProvider(t *testing.T) {
	srv := &Server{out: io.Discard}
	h := NewHandler(srv)

	raw, err := json.Marshal(lsp.InitializeParams{
		InitializationOptions: map[string]interface{}{
			"settings": map[string]interface{}{
				"diagnostics": map[string]interface{}{
					"overrides": map[string]interface{}{
						"PSR1.Classes.ClassDeclaration.PascalCase": map[string]interface{}{
							"classes": []interface{}{`/^RG_.*/`},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal initialize: %v", err)
	}

	if _, respErr := h.initialize(raw); respErr != nil {
		t.Fatalf("initialize returned error: %+v", respErr)
	}

	diags := h.prov.Diagnostics.Analyse("file:///test.php", `<?php
class RG_Session_SessionTest {}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR1.Classes.ClassDeclaration.PascalCase" {
			t.Fatalf("expected PascalCase diagnostic to be suppressed after initialize, got %+v", diag)
		}
	}
}

func TestInitializeIgnoresGitignoredDiagnosticsByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("generated/\nignored.php\n!generated/keep.php\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	srv := &Server{out: io.Discard}
	h := NewHandler(srv)

	raw, err := json.Marshal(lsp.InitializeParams{
		WorkspaceFolders: []lsp.WorkspaceFolder{{
			URI:  "file://" + filepath.ToSlash(tmpDir),
			Name: "tmp",
		}},
	})
	if err != nil {
		t.Fatalf("marshal initialize: %v", err)
	}

	if _, respErr := h.initialize(raw); respErr != nil {
		t.Fatalf("initialize returned error: %+v", respErr)
	}

	badText := "<?php\nclass Bad_Class {}\n"
	ignoredURI := "file://" + filepath.ToSlash(filepath.Join(tmpDir, "ignored.php"))
	if diags := h.prov.Diagnostics.Analyse(ignoredURI, badText); len(diags) != 0 {
		t.Fatalf("expected file ignored by .gitignore to suppress diagnostics, got %+v", diags)
	}

	generatedURI := "file://" + filepath.ToSlash(filepath.Join(tmpDir, "generated", "BadClass.php"))
	if diags := h.prov.Diagnostics.Analyse(generatedURI, badText); len(diags) != 0 {
		t.Fatalf("expected directory ignored by .gitignore to suppress diagnostics, got %+v", diags)
	}

	keepURI := "file://" + filepath.ToSlash(filepath.Join(tmpDir, "generated", "keep.php"))
	if diags := h.prov.Diagnostics.Analyse(keepURI, badText); len(diags) == 0 {
		t.Fatal("expected negated .gitignore rule to allow diagnostics")
	}
}
