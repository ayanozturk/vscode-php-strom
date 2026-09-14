package providers

import (
	"testing"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestIdentityFormatEditsEqual(t *testing.T) {
	edits := identityFormatEdits("<?php\n", "<?php\n")
	if edits != nil {
		t.Fatalf("expected nil edits when printed == text, got %v", edits)
	}
}

func TestIdentityFormatEditsMismatchFailClosed(t *testing.T) {
	edits := identityFormatEdits("<?php\n", "<?php\n\n")
	if edits != nil {
		t.Fatalf("expected nil edits on identity mismatch (fail closed), got %v", edits)
	}
}

func TestFormattingProviderFailClosed(t *testing.T) {
	provider := &FormattingProvider{}
	text := "<?php\nclass Foo {}\n"
	edits := provider.Format("file:///workspace/Foo.php", text, lsp.FormattingOptions{})
	if len(edits) != 0 {
		t.Fatalf("expected no edits for identity-holding PHP, got %d: %v", len(edits), edits)
	}
	for _, edit := range edits {
		if edit.NewText != text {
			t.Fatalf("Format must never return NewText different from input; got %q want %q", edit.NewText, text)
		}
	}
}
