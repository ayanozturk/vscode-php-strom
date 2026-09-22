package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymfonyServiceMetadataResolvesPublicAliasesAndRejectsAmbiguity(t *testing.T) {
	root := t.TempDir()
	writeSymfonyContainerFixture(t, root, "dev", `<?xml version="1.0"?>
<container><services>
  <service id="routing.engine.default" class="Framework\RoutingEngine"/>
  <service id="routing.engine" alias="routing.engine.default" public="true"/>
  <service id="private.engine" class="Framework\PrivateEngine"/>
</services></container>`)

	folders := []WorkspaceFolder{{URI: "file://" + filepath.ToSlash(root), Name: "tmp"}}
	types := loadSymfonyServiceTypes(folders)
	if got := types["routing.engine"]; got != `Framework\RoutingEngine` {
		t.Fatalf("expected public alias to resolve to its concrete class, got %q", got)
	}
	if _, ok := types["private.engine"]; ok {
		t.Fatal("expected private service metadata not to be exposed to container get() inference")
	}

	writeSymfonyContainerFixture(t, root, "test", `<?xml version="1.0"?>
<container><services><service id="routing.engine" class="Framework\DifferentEngine" public="true"/></services></container>`)
	if _, ok := loadSymfonyServiceTypes(folders)["routing.engine"]; ok {
		t.Fatal("expected service IDs with conflicting environment types to remain unresolved")
	}
}

func writeSymfonyContainerFixture(t *testing.T, root, environment, contents string) {
	t.Helper()
	directory := filepath.Join(root, "var", "cache", environment)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create cache directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "App_KernelDebugContainer.xml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write container metadata: %v", err)
	}
}
