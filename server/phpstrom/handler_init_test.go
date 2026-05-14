package phpstrom

import (
	"encoding/json"
	"io"
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
