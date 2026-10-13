package indexer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type composerAutoload struct {
	PSR4     map[string]json.RawMessage `json:"psr-4"`
	PSR0     map[string]json.RawMessage `json:"psr-0"`
	Classmap []string                   `json:"classmap"`
}

type composerJSON struct {
	Autoload    composerAutoload `json:"autoload"`
	AutoloadDev composerAutoload `json:"autoload-dev"`
}

// composerAutoloadDirs walks upward from root looking for the nearest
// composer.json and returns every directory referenced by its autoload /
// autoload-dev psr-4, psr-0, and classmap entries, resolved relative to the
// composer.json location. This lets legacy monorepo layouts (e.g. a sibling
// tree holding PSR-0/classmap-style underscore class names outside the open
// workspace folder) get indexed for symbol resolution, even though those
// files are never part of the diagnosed workspace file set.
func composerAutoloadDirs(root string) []string {
	composerDir, raw := findComposerJSON(root)
	if raw == nil {
		return nil
	}

	var parsed composerJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}

	var dirs []string
	collect := func(al composerAutoload) {
		for _, targets := range al.PSR4 {
			dirs = append(dirs, resolveAutoloadTargets(composerDir, targets)...)
		}
		for _, targets := range al.PSR0 {
			dirs = append(dirs, resolveAutoloadTargets(composerDir, targets)...)
		}
		for _, dir := range al.Classmap {
			dirs = append(dirs, filepath.Join(composerDir, dir))
		}
	}
	collect(parsed.Autoload)
	collect(parsed.AutoloadDev)
	return dirs
}

// resolveAutoloadTargets decodes a psr-4/psr-0 autoload map value, which may
// be a single path string or an array of path strings.
func resolveAutoloadTargets(composerDir string, raw json.RawMessage) []string {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{filepath.Join(composerDir, single)}
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err == nil {
		dirs := make([]string, 0, len(multiple))
		for _, dir := range multiple {
			dirs = append(dirs, filepath.Join(composerDir, dir))
		}
		return dirs
	}
	return nil
}

// findComposerJSON walks upward from dir looking for composer.json, stopping
// at the filesystem root. It returns the directory containing composer.json
// and its raw bytes, or ("", nil) if none is found.
// composerAutoloadWorkspaceFolders resolves composer-autoload-only index
// roots for the given workspace folders, skipping any resolved directory
// that already sits inside one of the folders (it is already covered by the
// host walk) and deduplicating repeats across folders/entries.
func composerAutoloadWorkspaceFolders(folders []WorkspaceFolder) []WorkspaceFolder {
	hostRoots := make([]string, 0, len(folders))
	for _, folder := range folders {
		if root := uriToPath(folder.URI); root != "" {
			hostRoots = append(hostRoots, filepath.Clean(root))
		}
	}

	seen := make(map[string]struct{})
	var extra []WorkspaceFolder
	for _, root := range hostRoots {
		for _, dir := range composerAutoloadDirs(root) {
			dir = filepath.Clean(dir)
			if _, ok := seen[dir]; ok {
				continue
			}
			if isUnderAnyRoot(dir, hostRoots) {
				continue
			}
			seen[dir] = struct{}{}
			extra = append(extra, WorkspaceFolder{URI: pathToURI(dir), Name: filepath.Base(dir)})
		}
	}
	return extra
}

func isUnderAnyRoot(dir string, roots []string) bool {
	for _, root := range roots {
		if dir == root {
			return true
		}
		if strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func findComposerJSON(dir string) (string, []byte) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	} else {
		dir = filepath.Clean(dir)
	}
	for {
		candidate := filepath.Join(dir, "composer.json")
		if data, err := os.ReadFile(candidate); err == nil {
			return dir, data
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
