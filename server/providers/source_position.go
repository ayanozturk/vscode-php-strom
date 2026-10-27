package providers

import (
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	goparser "github.com/ayanozturk/tusk/diag"
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
	// Same contract as goparser.UTF16PositionFromByteOffset, but resolved
	// through the precomputed line table: no []byte copy of the document and
	// no scan from the start of the file for every diagnostic endpoint.
	if offset < 0 {
		offset = 0
	}
	if offset > len(m.source) {
		offset = len(m.source)
	}
	line := sort.Search(len(m.lineStarts), func(i int) bool { return m.lineStarts[i] > offset }) - 1
	column := 0
	for i := m.lineStarts[line]; i < offset; {
		if m.source[i] == '\r' && i+1 == offset && offset < len(m.source) && m.source[offset] == '\n' {
			break
		}
		r, size := utf8.DecodeRuneInString(m.source[i:])
		if size < 1 || i+size > offset {
			break
		}
		column += utf16.RuneLen(r)
		i += size
	}
	return lsp.Position{Line: uint32(line), Character: uint32(column)}
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

// byteSpanFromRunePositions converts the parser/style packages' one-based
// rune coordinates into the shared diagnostic's half-open byte span, with the
// same contract as goparser.ByteSpanFromRunePositions but resolved through the
// line table instead of rescanning the document from the start per call.
func (m sourcePositionMapper) byteSpanFromRunePositions(startLine, startColumn, endLine, endColumn int) goparser.ByteSpan {
	start, ok := m.runePositionToByte(startLine, startColumn)
	if !ok {
		return goparser.ByteSpan{}
	}
	end := start
	if endLine > 0 && endColumn > 0 {
		if candidate, valid := m.runePositionToByte(endLine, endColumn); valid && candidate >= start {
			end = candidate
		}
	}
	return goparser.ByteSpan{Start: start, End: end}
}

func (m sourcePositionMapper) runePositionToByte(line, column int) (int, bool) {
	if line < 1 || column < 1 || line > len(m.lineStarts) {
		return 0, false
	}
	lineStart := m.lineStarts[line-1]
	lineEnd := len(m.source)
	if line < len(m.lineStarts) {
		lineEnd = m.lineStarts[line] - 1
	}
	if lineEnd > lineStart && m.source[lineEnd-1] == '\r' {
		lineEnd--
	}
	target := column - 1
	for offset, count := lineStart, 0; ; {
		if count == target {
			return offset, true
		}
		if offset >= lineEnd {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(m.source[offset:lineEnd])
		if size < 1 {
			return 0, false
		}
		offset += size
		count++
	}
}

func (m sourcePositionMapper) byteOffsetFromRunePosition(line, column int) int {
	if line < 1 {
		return 0
	}
	lineIndex := line - 1
	if lineIndex >= len(m.lineStarts) {
		return len(m.source)
	}
	start := m.lineStarts[lineIndex]
	end := len(m.source)
	if lineIndex+1 < len(m.lineStarts) {
		end = m.lineStarts[lineIndex+1] - 1
	}
	lineText := strings.TrimSuffix(m.source[start:end], "\r")
	targetRunes := column - 1
	if targetRunes < 0 {
		targetRunes = 0
	}
	offset := start
	for count := 0; count < targetRunes && offset < start+len(lineText); count++ {
		_, size := utf8.DecodeRuneInString(lineText[offset-start:])
		if size < 1 {
			break
		}
		offset += size
	}
	return offset
}

func (m sourcePositionMapper) diagnosticRange(value goparser.Diagnostic) lsp.Range {
	return lsp.Range{
		Start: m.positionFromByteOffset(value.Span.Start),
		End:   m.positionFromByteOffset(value.Span.End),
	}
}

func (m sourcePositionMapper) toLSPDiagnostic(value goparser.Diagnostic) lsp.Diagnostic {
	severity := lsp.DiagSeverityWarning
	switch value.Severity {
	case goparser.SeverityError:
		severity = lsp.DiagSeverityError
	case goparser.SeverityInfo:
		severity = lsp.DiagSeverityInfo
	case goparser.SeverityHint:
		severity = lsp.DiagSeverityHint
	}
	return lsp.Diagnostic{
		Range:    m.diagnosticRange(value),
		Severity: &severity,
		Code:     value.Code,
		Source:   value.Source,
		Message:  value.Message,
	}
}

func utf16CodeUnits(s string) int {
	units := 0
	for _, r := range s {
		units += utf16.RuneLen(r)
	}
	return units
}
