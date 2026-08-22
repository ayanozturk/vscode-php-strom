//go:build darwin || linux

package indexer

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestCollectWorkspaceFilePathsRejectsSymlinksAndSpecialFiles(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()

	regularPath := filepath.Join(workspace, "Regular.php")
	if err := os.WriteFile(regularPath, []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write regular PHP file: %v", err)
	}
	outsidePath := filepath.Join(outside, "Outside.php")
	if err := os.WriteFile(outsidePath, []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write outside PHP file: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(workspace, "Linked.php")); err != nil {
		t.Fatalf("create PHP file symlink: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "linked-directory")); err != nil {
		t.Fatalf("create directory symlink: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(workspace, "Blocking.php"), 0o600); err != nil {
		t.Fatalf("create PHP named pipe: %v", err)
	}

	wi := New(Config{Associations: []string{"**/*.php"}})
	folders := []WorkspaceFolder{{URI: pathToURI(workspace), Name: "workspace"}}
	wi.SetWorkspaceFolders(folders)

	paths := wi.collectWorkspaceFilePaths(folders, wi.gitignores)
	if len(paths) != 1 || paths[0] != regularPath {
		t.Fatalf("expected only the regular PHP file, got %#v", paths)
	}
}

func TestReadFileWithinLimitRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Target.php")
	if err := os.WriteFile(target, []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	link := filepath.Join(dir, "Linked.php")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, _, _, err := ReadFileWithinLimit(link, 1_000); err == nil {
		t.Fatal("expected symlink read to be rejected")
	}
}

func TestReadFileWithinLimitRejectsNamedPipeWithoutBlocking(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "Blocking.php")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatalf("create PHP named pipe: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, _, err := ReadFileWithinLimit(pipe, 1_000)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected named pipe read to be rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("named pipe read blocked")
	}
}
