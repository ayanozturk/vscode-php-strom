package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/phpstrom"
	"github.com/ayanozturk/vscode-php-strom/providers"
)

func TestProductionBenchmarkUsesDefaultDiagnosticConfiguration(t *testing.T) {
	level := phpstrom.DefaultProviderConfig(nil).AnalysisLevel
	if level == nil || *level != 9 {
		t.Fatalf("expected production analysis level 9, got %#v", level)
	}

	disabled := phpstrom.DefaultProviderConfig(nil).DisabledAnalysis
	if !disabled.Style || !disabled.SideEffects {
		t.Fatalf("expected production defaults to disable style and side-effects, got %#v", disabled)
	}
	if disabled.AssignmentInCondition || disabled.TypeErrors || disabled.Deprecated {
		t.Fatalf("expected production correctness diagnostics to remain enabled, got %#v", disabled)
	}
}

func TestReportableWorkspaceURIsRetainsVendorIndexButSkipsIgnoredDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/vendor/\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}

	folders := []indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(root), Name: "tmp"}}
	idx := indexer.New(indexer.Config{})
	projectURI := "file://" + filepath.ToSlash(filepath.Join(root, "src", "Project.php"))
	vendorURI := "file://" + filepath.ToSlash(filepath.Join(root, "vendor", "pkg", "Library.php"))
	idx.IndexDocument(projectURI, "<?php class Project {}")
	idx.IndexDocument(vendorURI, "<?php class Library {}")

	if idx.GetIndex().GetByFQN(`\Library`) == nil {
		t.Fatal("expected vendor symbol to remain indexed")
	}

	diagnostics := providers.NewRegistry(idx, phpstrom.DefaultProviderConfig(folders)).Diagnostics
	got := reportableWorkspaceURIs([]string{projectURI, vendorURI}, diagnostics)
	if len(got) != 1 || got[0] != projectURI {
		t.Fatalf("expected only the project URI to be reportable, got %#v", got)
	}
}
