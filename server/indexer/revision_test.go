package indexer

import "testing"

func TestProjectIndexSnapshotRevisionChangesOnlyWhenProjectRebuilds(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(): void {}\n"
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
}
