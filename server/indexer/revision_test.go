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

func TestSemanticTraceAccountsIncrementalUpdatesAndDependencyMatches(t *testing.T) {
	const (
		consumerURI    = "file:///workspace/Consumer.php"
		consumerFile   = "/workspace/Consumer.php"
		consumerText   = "<?php\nfunction run(Dependency $dependency): void { $dependency->render(); }\n"
		dependencyURI  = "file:///workspace/Dependency.php"
		dependencyBody = "<?php\nclass Dependency { public function render(): string { return 'before'; } }\n"
	)

	wi := New(Config{})
	wi.IndexDocument(consumerURI, consumerText)
	wi.IndexDocument(dependencyURI, dependencyBody)
	parsed := ParseSource(consumerURI, consumerText)

	matchedBefore := wi.SemanticTrace()
	wi.ProjectIndexSnapshotForFile(consumerFile, consumerText, parsed.Nodes)
	matchedAfter := wi.SemanticTrace()
	if matchedAfter.RevisionChecks <= matchedBefore.RevisionChecks {
		t.Fatalf("expected a semantic revision check, before=%#v after=%#v", matchedBefore, matchedAfter)
	}
	if matchedAfter.DependencyMatches <= matchedBefore.DependencyMatches {
		t.Fatalf("expected the consumer to match the dependency change, before=%#v after=%#v", matchedBefore, matchedAfter)
	}

	bodyBefore := wi.SemanticTrace()
	wi.IndexDocument(dependencyURI, "<?php\nclass Dependency { public function render(): string { return 'after'; } }\n")
	bodyAfter := wi.SemanticTrace()
	if bodyAfter.BodyOnlyUpdates <= bodyBefore.BodyOnlyUpdates {
		t.Fatalf("expected a body-only incremental update, before=%#v after=%#v", bodyBefore, bodyAfter)
	}
	if bodyAfter.FullFallbacks != bodyBefore.FullFallbacks {
		t.Fatalf("did not expect body-only update to fall back to a full build, before=%#v after=%#v", bodyBefore, bodyAfter)
	}

	exportedBefore := wi.SemanticTrace()
	wi.IndexDocument(dependencyURI, "<?php\nclass Dependency { public function render(int $mode): string { return (string) $mode; } }\n")
	exportedAfter := wi.SemanticTrace()
	if exportedAfter.ExportedChanges <= exportedBefore.ExportedChanges {
		t.Fatalf("expected an exported semantic change, before=%#v after=%#v", exportedBefore, exportedAfter)
	}
	if exportedAfter.IncrementalBuilds <= exportedBefore.IncrementalBuilds {
		t.Fatalf("expected exported update to use the incremental path, before=%#v after=%#v", exportedBefore, exportedAfter)
	}

	matchedBefore = wi.SemanticTrace()
	wi.ProjectIndexSnapshotForFile(consumerFile, consumerText, parsed.Nodes)
	matchedAfter = wi.SemanticTrace()
	if matchedAfter.DependencyMatches <= matchedBefore.DependencyMatches {
		t.Fatalf("expected the consumer to match the exported dependency update, before=%#v after=%#v", matchedBefore, matchedAfter)
	}
}

func TestSemanticTraceAccountsFullFallbackAndGlobalCompaction(t *testing.T) {
	wi := New(Config{})
	before := wi.SemanticTrace()

	wi.IndexDocument("file:///workspace/Initial.php", "<?php\nclass Initial {}\n")

	after := wi.SemanticTrace()
	if after.FullBuilds <= before.FullBuilds {
		t.Fatalf("expected the untracked initial project to require a full build, before=%#v after=%#v", before, after)
	}
	if after.FullFallbacks <= before.FullFallbacks {
		t.Fatalf("expected the untracked initial project to record a full fallback, before=%#v after=%#v", before, after)
	}
	if after.GlobalCompactions <= before.GlobalCompactions {
		t.Fatalf("expected the incomplete initial change metadata to compact globally, before=%#v after=%#v", before, after)
	}
}
