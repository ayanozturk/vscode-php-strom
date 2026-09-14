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

func (m sourcePositionMapper) positionFromByteOffset(offset int) lsp.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(m.source) {
		offset = len(m.source)
	}
	line := 0
	for i, start := range m.lineStarts {
		if start > offset {
			break
		}
		line = i
	}
	lineStart := m.lineStarts[line]
	col := utf16CodeUnits(m.source[lineStart:offset])
	return lsp.Position{Line: uint32(line), Character: uint32(col)}
}

func (m sourcePositionMapper) byteOffsetFromPosition(pos lsp.Position) int {
	line := int(pos.Line)
	if line < 0 {
		return 0
	}
	if line >= len(m.lineStarts) {
		return len(m.source)
	}
	lineStart := m.lineStarts[line]
	lineEnd := len(m.source)
	if line+1 < len(m.lineStarts) {
		lineEnd = m.lineStarts[line+1] - 1
		if lineEnd < lineStart {
			lineEnd = lineStart
		}
	}
	lineText := strings.TrimSuffix(m.source[lineStart:lineEnd], "\r")
	target := int(pos.Character)
	utf16Column := 0
	bytePos := lineStart
	for _, value := range lineText {
		if utf16Column >= target {
			return bytePos
		}
		size := len(string(value))
		utf16Column += utf16.RuneLen(value)
		bytePos += size
		if utf16Column >= target {
			return bytePos
		}
	}
	return lineStart + len(lineText)
}

func utf16CodeUnits(s string) int {
	units := 0
	for _, r := range s {
		units += utf16.RuneLen(r)
	}
	return units
}

