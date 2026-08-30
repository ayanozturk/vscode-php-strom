package indexer

import (
	"testing"

	"github.com/ayanozturk/go-php-parser/analyse"
)

func TestProjectIndexSnapshotRevisionChangesOnlyWhenExportedSemanticsChange(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(Dependency $dependency): void { $dependency->run(); }\n"
		otherURI = "file:///workspace/Dependency.php"
	)

	wi := New(Config{})
	parsed := ParseSource(uri, text)
	wi.IndexDocument(uri, text)

	firstProject, firstRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	secondProject, secondRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if firstProject == nil || secondProject == nil {
		t.Fatal("expected project-index views")
	}
	if firstRevision != secondRevision {
		t.Fatalf("expected unchanged reads to keep revision %d, got %d", firstRevision, secondRevision)
	}
	if firstProject != secondProject {
		t.Fatal("expected unchanged reads to reuse the project index")
	}

	wi.IndexDocument(otherURI, "<?php\nclass Dependency {}\n")
	thirdProject, thirdRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if thirdProject == nil {
		t.Fatal("expected project-index view after workspace rebuild")
	}
	if thirdRevision <= secondRevision {
		t.Fatalf("expected project revision to increase after rebuild, remained %d", thirdRevision)
	}

	fourthProject, fourthRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if fourthRevision != thirdRevision {
		t.Fatalf("expected unchanged reads after rebuild to keep revision %d, got %d", thirdRevision, fourthRevision)
	}
	if fourthProject != thirdProject {
		t.Fatal("expected unchanged reads after rebuild to reuse the project index")
	}

	wi.IndexDocument(otherURI, "<?php\nclass Dependency { public function run(): void { echo 'changed'; } }\n")
	bodyProject, bodyRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if bodyRevision <= fourthRevision {
		t.Fatalf("expected adding an exported method to advance revision beyond %d, got %d", fourthRevision, bodyRevision)
	}

	wi.IndexDocument(otherURI, "<?php\nclass Dependency { public function run(): void { echo 'updated body'; } }\n")
	updatedBodyProject, updatedBodyRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if updatedBodyRevision != bodyRevision {
		t.Fatalf("expected body-only edit to retain semantic revision %d, got %d", bodyRevision, updatedBodyRevision)
	}
	if updatedBodyProject == bodyProject {
		t.Fatal("expected body-only edit to publish a new immutable project index")
	}

	wi.IndexDocument(otherURI, "<?php\nclass Dependency { public function run(int $value): void { echo $value; } }\n")
	_, signatureRevision := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if signatureRevision <= updatedBodyRevision {
		t.Fatalf("expected exported signature edit to advance revision beyond %d, got %d", updatedBodyRevision, signatureRevision)
	}
}

func TestProjectIndexSnapshotRevisionIgnoresUnreferencedExportChanges(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(): void { echo 'independent'; }\n"
		otherURI = "file:///workspace/Dependency.php"
	)

	wi := New(Config{})
	wi.IndexDocument(uri, text)
	wi.IndexDocument(otherURI, "<?php\nclass Dependency {}\n")
	parsed := ParseSource(uri, text)
	_, before := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)

	wi.IndexDocument(otherURI, "<?php\nclass Dependency { public function changed(int $value): void {} }\n")
	_, after := wi.ProjectIndexSnapshotForFile(filename, text, parsed.Nodes)
	if after != before {
		t.Fatalf("expected unrelated export change to retain document revision %d, got %d", before, after)
	}
}

func TestSemanticChangeEventOverflowFallsBackToGlobalRevision(t *testing.T) {
	wi := New(Config{})
	wi.semanticBase = 1
	for revision := uint64(2); revision <= uint64(maxSemanticChangeEvents)+2; revision++ {
		wi.semanticRevision = revision - 1
		wi.recordSemanticChangesLocked(analyse.ProjectIndexChanges{
			Complete:        true,
			Symbols:         []analyse.ExportedSymbolChange{{Kind: "class", Name: "Dependency"}},
			DependencyNames: []string{"Dependency"},
		})
	}
	if len(wi.semanticChanges) != 0 {
		t.Fatalf("expected overflow to compact semantic events, retained %d", len(wi.semanticChanges))
	}
	if wi.semanticBase != wi.semanticRevision {
		t.Fatalf("expected compacted base revision %d, got %d", wi.semanticRevision, wi.semanticBase)
	}
	if got := wi.documentSemanticRevisionLocked("<?php class Independent {}"); got != wi.semanticBase {
		t.Fatalf("expected compacted history to invalidate unrelated document at %d, got %d", wi.semanticBase, got)
	}
}

func TestSemanticChangeDependencyOverflowFallsBackToGlobalRevision(t *testing.T) {
	wi := New(Config{})
	names := make([]string, maxSemanticChangeDependencyNames+1)
	for index := range names {
		names[index] = "Dependency"
	}
	wi.recordSemanticChangesLocked(analyse.ProjectIndexChanges{
		Complete:        true,
		Symbols:         []analyse.ExportedSymbolChange{{Kind: "class", Name: "Dependency"}},
		DependencyNames: names,
	})
	if len(wi.semanticChanges) != 0 || wi.semanticBase != wi.semanticRevision {
		t.Fatalf("expected oversized dependency event to compact globally: base=%d revision=%d events=%d", wi.semanticBase, wi.semanticRevision, len(wi.semanticChanges))
	}
}

func TestSemanticChangeAffectsPHPIdentifiersCaseInsensitively(t *testing.T) {
	text := "<?php\nuse App\\Domain\\BaseRecord;\n$record->SAVE();\n"
	if !semanticChangeAffectsText(text, []string{"app\\domain\\baserecord"}) {
		t.Fatal("expected qualified class dependency to match case-insensitively")
	}
	if !semanticChangeAffectsText(text, []string{"save"}) {
		t.Fatal("expected member dependency to match case-insensitively")
	}
	if semanticChangeAffectsText(text, []string{"base"}) {
		t.Fatal("did not expect partial identifier substring to match")
	}
}
