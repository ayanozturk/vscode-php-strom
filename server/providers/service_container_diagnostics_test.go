package providers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func TestDiagnosticsUsesSymfonyServiceMetadataForContainerGet(t *testing.T) {
	root := t.TempDir()
	writeProviderFixture(t, root, "vendor/psr/container/ContainerInterface.php", `<?php
namespace Psr\Container;
interface ContainerInterface { public function get(string $id): ?object; }
`)
	writeProviderFixture(t, root, "vendor/symfony/dependency-injection/ContainerInterface.php", `<?php
namespace Symfony\Component\DependencyInjection;
use Psr\Container\ContainerInterface as PsrContainerInterface;
interface ContainerInterface extends PsrContainerInterface {}
`)
	writeProviderFixture(t, root, "src/Container.php", `<?php
namespace Framework;
class Container implements \Psr\Container\ContainerInterface { public function get(string $id): ?object { return null; } }
class RoutingEngine { public function routes(): array { return []; } }
class Kernel { public function getContainer(): Container { return new Container(); } }
class ExternalGateway { public function result(): ?object { return null; } }
`)
	consumer := `<?php
$kernel = new \Framework\Kernel();
$container = $kernel->getContainer();
$engine = $container->get('routing.engine');
$routes = array_keys($engine->routes());
$gateway = new \Framework\ExternalGateway();
$result = $gateway->result();
$unrelated = $result->routes();
`
	consumerPath := writeProviderFixture(t, root, "scripts/check.php", consumer)

	cacheDir := filepath.Join(root, "var", "cache", "dev")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("create cache directory: %v", err)
	}
	metadata := `<?xml version="1.0"?><container><services><service id="routing.engine" class="Framework\RoutingEngine" public="true"/></services></container>`
	if err := os.WriteFile(filepath.Join(cacheDir, "App_KernelDebugContainer.xml"), []byte(metadata), 0o644); err != nil {
		t.Fatalf("write container metadata: %v", err)
	}

	folders := []indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(root), Name: "tmp"}}
	idx := indexer.New(indexer.Config{Associations: []string{"**/*.php"}, MaxSize: 1_000_000})
	idx.SetWorkspaceFolders(folders)
	idx.IndexWorkspace()
	if serviceType, ok := idx.ResolveServiceType("routing.engine"); !ok || serviceType != `Framework\RoutingEngine` {
		t.Fatalf("expected indexed Symfony service metadata, got %q, %v", serviceType, ok)
	}
	if idx.GetIndex().GetByFQN(`\Framework\RoutingEngine`) == nil {
		t.Fatal("expected mapped service class to be present in the workspace index")
	}
	level := 9
	provider := NewRegistry(idx, Config{AnalysisLevel: &level, DisabledAnalysis: DisabledAnalysis{Style: true, SideEffects: true}}).Diagnostics
	snapshot := parseSemanticSnapshot(consumer)
	ctx := provider.cache.analysisContextForFile(idx, "", consumerPath, consumer, snapshot.nodes)
	if !isContainerResolverType(`Symfony\Component\DependencyInjection\ContainerInterface`, ctx, make(map[string]struct{})) {
		t.Fatal("expected Symfony's dependency injection container contract to be recognized")
	}
	if serviceType, ok := resolvedServiceVariableTypeAtLine(snapshot.nodes, 5, "engine", ctx); !ok || serviceType != `Framework\RoutingEngine` {
		kernelType, _ := inferVariableFlowHoverType(snapshot.nodes, 3, "kernel", ctx)
		containerType, _ := inferVariableFlowHoverType(snapshot.nodes, 4, "container", ctx)
		engineType, _ := inferVariableFlowHoverType(snapshot.nodes, 5, "engine", ctx)
		isContainer := isContainerResolverType(containerType, ctx, make(map[string]struct{}))
		t.Fatalf("expected service-provenance flow to resolve service receiver, got %q, %v (kernel=%q container=%q isContainer=%v engine=%q)", serviceType, ok, kernelType, containerType, isContainer, engineType)
	}
	diagnostics := provider.AnalyseTransient("file://"+filepath.ToSlash(consumerPath), consumer)
	keptUnrelated := false
	for _, diagnostic := range diagnostics {
		code, _ := diagnostic.Code.(string)
		if code != "Level8.MethodNonObject" && code != "Level2.MethodNonObject" {
			continue
		}
		if diagnostic.Range.Start.Line == 7 {
			keptUnrelated = true
		} else {
			t.Fatalf("expected authoritative service metadata to resolve the concrete receiver, got %#v", diagnostics)
		}
	}
	if !keptUnrelated {
		t.Fatalf("expected unrelated nullable result diagnostic to remain, got %#v", diagnostics)
	}
}

func TestDiagnosticsDoesNotAssumeArbitraryGetIsAServiceContainer(t *testing.T) {
	root := t.TempDir()
	writeProviderFixture(t, root, "src/Registry.php", `<?php
namespace Framework;
class Registry { public function get(string $id): ?object { return null; } }
class RoutingEngine { public function routes(): array { return []; } }
`)
	consumer := `<?php
$registry = new \Framework\Registry();
$engine = $registry->get('routing.engine');
$routes = $engine->routes();
`
	consumerPath := writeProviderFixture(t, root, "scripts/check.php", consumer)
	cacheDir := filepath.Join(root, "var", "cache", "dev")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := `<?xml version="1.0"?><container><services><service id="routing.engine" class="Framework\RoutingEngine" public="true"/></services></container>`
	if err := os.WriteFile(filepath.Join(cacheDir, "App_KernelDebugContainer.xml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	folders := []indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(root), Name: "tmp"}}
	idx := indexer.New(indexer.Config{Associations: []string{"**/*.php"}, MaxSize: 1_000_000})
	idx.SetWorkspaceFolders(folders)
	idx.IndexWorkspace()
	level := 9
	diagnostics := NewRegistry(idx, Config{AnalysisLevel: &level, DisabledAnalysis: DisabledAnalysis{Style: true, SideEffects: true}}).Diagnostics.AnalyseTransient("file://"+filepath.ToSlash(consumerPath), consumer)
	for _, diagnostic := range diagnostics {
		if code, _ := diagnostic.Code.(string); code == "Level8.MethodNonObject" || code == "Level2.MethodNonObject" {
			return
		}
	}
	t.Fatalf("expected non-container get() result to remain nullable, got %#v", diagnostics)
}

func writeProviderFixture(t *testing.T, root, relative, contents string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
