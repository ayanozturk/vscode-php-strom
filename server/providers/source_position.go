package providers

import (
	"strings"
	"unicode/utf16"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// sourcePositionMapper converts the parser's one-based rune coordinates into
// the zero-based UTF-16 code-unit coordinates required by LSP.
type sourcePositionMapper struct {
	source     string
	lineStarts []int
}

func newSourcePositionMapper(source string) sourcePositionMapper {
	starts := []int{0}
	for index := 0; index < len(source); index++ {
		if source[index] == '\n' {
			starts = append(starts, index+1)
		}
	}
	return sourcePositionMapper{source: source, lineStarts: starts}
}

func (m sourcePositionMapper) pointRange(line, column int) lsp.Range {
	position, _ := m.position(line, column)
	return lsp.Range{Start: position, End: position}
}

func (m sourcePositionMapper) spanRange(startLine, startColumn, endLine, endColumn int) lsp.Range {
	start, startValid := m.position(startLine, startColumn)
	if endLine < 1 || endColumn < 1 {
		return lsp.Range{Start: start, End: start}
	}
	end, endValid := m.position(endLine, endColumn)
	if !startValid || !endValid || positionBefore(end, start) {
		return lsp.Range{Start: start, End: start}
	}
	return lsp.Range{Start: start, End: end}
}

func (m sourcePositionMapper) position(line, column int) (lsp.Position, bool) {
	if line < 1 || line > len(m.lineStarts) {
		return lsp.Position{}, false
	}
	lineIndex := line - 1
	start := m.lineStarts[lineIndex]
	end := len(m.source)
	if lineIndex+1 < len(m.lineStarts) {
		end = m.lineStarts[lineIndex+1] - 1
	}
	lineText := strings.TrimSuffix(m.source[start:end], "\r")
	if column < 1 {
		return lsp.Position{Line: uint32(lineIndex)}, false
	}

	targetRune := column - 1
	runeIndex := 0
	utf16Column := 0
	for _, value := range lineText {
		if runeIndex == targetRune {
			return lsp.Position{Line: uint32(lineIndex), Character: uint32(utf16Column)}, true
		}
		utf16Column += utf16.RuneLen(value)
		runeIndex++
	}
	position := lsp.Position{Line: uint32(lineIndex), Character: uint32(utf16Column)}
	return position, runeIndex == targetRune
}

func positionBefore(left, right lsp.Position) bool {
	return left.Line < right.Line || (left.Line == right.Line && left.Character < right.Character)
}
