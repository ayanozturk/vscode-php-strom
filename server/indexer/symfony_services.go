package indexer

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type symfonyServiceXML struct {
	ID     string `xml:"id,attr"`
	Class  string `xml:"class,attr"`
	Alias  string `xml:"alias,attr"`
	Public string `xml:"public,attr"`
}

type symfonyServiceDefinition struct {
	class  string
	alias  string
	public bool
}

func loadSymfonyServiceTypes(folders []WorkspaceFolder) map[string]string {
	candidates := make(map[string]map[string]struct{})
	for _, folder := range folders {
		root := uriToPath(folder.URI)
		if root == "" {
			continue
		}
		paths, _ := filepath.Glob(filepath.Join(root, "var", "cache", "*", "*DebugContainer.xml"))
		sort.Strings(paths)
		for _, path := range paths {
			for id, className := range readSymfonyServiceTypes(path) {
				if candidates[id] == nil {
					candidates[id] = make(map[string]struct{})
				}
				candidates[id][className] = struct{}{}
			}
		}
	}

	resolved := make(map[string]string, len(candidates))
	for id, classes := range candidates {
		if len(classes) != 1 {
			continue
		}
		for className := range classes {
			resolved[id] = className
		}
	}
	return resolved
}

func readSymfonyServiceTypes(path string) map[string]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	definitions := make(map[string]symfonyServiceDefinition)
	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "service" {
			continue
		}
		var service symfonyServiceXML
		if err := decoder.DecodeElement(&service, &start); err != nil || service.ID == "" {
			continue
		}
		definitions[service.ID] = symfonyServiceDefinition{
			class:  strings.TrimPrefix(service.Class, `\`),
			alias:  service.Alias,
			public: service.Public == "true" || service.Public == "1",
		}
	}

	resolved := make(map[string]string)
	for id, definition := range definitions {
		if !definition.public {
			continue
		}
		if className := resolveSymfonyServiceClass(id, definitions, make(map[string]struct{})); className != "" {
			resolved[id] = className
		}
	}
	return resolved
}

func resolveSymfonyServiceClass(id string, definitions map[string]symfonyServiceDefinition, seen map[string]struct{}) string {
	if _, duplicate := seen[id]; duplicate {
		return ""
	}
	seen[id] = struct{}{}
	definition, ok := definitions[id]
	if !ok {
		return ""
	}
	if definition.class != "" {
		return definition.class
	}
	if definition.alias != "" {
		return resolveSymfonyServiceClass(definition.alias, definitions, seen)
	}
	return ""
}
