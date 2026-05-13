package providers

import (
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestTypeDefinitionProviderResolvesImportedClass(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/src/Domain/Notification.php", "<?php\nnamespace App\\Domain;\nclass Notification {}\n")

	provider := &TypeDefinitionProvider{idx: idx}
	text := "<?php\nnamespace App\\Service;\nuse App\\Domain\\Notification;\nclass Mailer { public function add(Notification $notification): void {} }\n"
	locs := provider.Provide("file:///workspace/src/Service/Mailer.php", text, lsp.Position{Line: 3, Character: 40})

	if len(locs) != 1 {
		t.Fatalf("expected 1 type definition, got %d", len(locs))
	}
	if locs[0].URI != "file:///workspace/src/Domain/Notification.php" {
		t.Fatalf("expected Notification.php, got %s", locs[0].URI)
	}
}

func TestDefinitionProviderResolvesSameNamespaceClassType(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/src/Domain/Notification.php", "<?php\nnamespace App\\Domain;\nclass Notification {}\n")

	provider := &DefinitionProvider{idx: idx}
	text := "<?php\nnamespace App\\Domain;\nclass Mailer { public function add(Notification $notification): void {} }\n"
	locs := provider.Provide("file:///workspace/src/Domain/Mailer.php", text, lsp.Position{Line: 2, Character: 40})

	if len(locs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(locs))
	}
	if locs[0].URI != "file:///workspace/src/Domain/Notification.php" {
		t.Fatalf("expected Notification.php, got %s", locs[0].URI)
	}
}
