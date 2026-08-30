package providers

import (
	"testing"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestSourcePositionMapper_ASCIIHalfOpenSpan(t *testing.T) {
	mapper := newSourcePositionMapper("abcdef")

	rangeValue := mapper.spanRange(1, 2, 1, 5)
	want := lsp.Range{
		Start: lsp.Position{Line: 0, Character: 1},
		End:   lsp.Position{Line: 0, Character: 4},
	}
	if rangeValue != want {
		t.Fatalf("expected ASCII range %+v, got %+v", want, rangeValue)
	}
}

func TestSourcePositionMapper_BMPRuneUsesOneUTF16Unit(t *testing.T) {
	mapper := newSourcePositionMapper("aébc")

	tests := []struct {
		name   string
		column int
		char   uint32
	}{
		{name: "before", column: 1, char: 0},
		{name: "start", column: 2, char: 1},
		{name: "end", column: 3, char: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			position, valid := mapper.position(1, tt.column)
			if !valid {
				t.Fatalf("expected column %d to be valid", tt.column)
			}
			if position != (lsp.Position{Line: 0, Character: tt.char}) {
				t.Fatalf("column %d: expected UTF-16 character %d, got %+v", tt.column, tt.char, position)
			}
		})
	}

	rangeValue := mapper.spanRange(1, 2, 1, 3)
	want := lsp.Range{
		Start: lsp.Position{Line: 0, Character: 1},
		End:   lsp.Position{Line: 0, Character: 2},
	}
	if rangeValue != want {
		t.Fatalf("expected BMP span %+v, got %+v", want, rangeValue)
	}
}

func TestSourcePositionMapper_AstralRuneUsesSurrogatePair(t *testing.T) {
	mapper := newSourcePositionMapper("a🙂b")

	tests := []struct {
		name   string
		column int
		char   uint32
	}{
		{name: "before", column: 1, char: 0},
		{name: "start", column: 2, char: 1},
		{name: "end", column: 3, char: 3},
		{name: "after", column: 4, char: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			position, valid := mapper.position(1, tt.column)
			if !valid {
				t.Fatalf("expected column %d to be valid", tt.column)
			}
			if position != (lsp.Position{Line: 0, Character: tt.char}) {
				t.Fatalf("column %d: expected UTF-16 character %d, got %+v", tt.column, tt.char, position)
			}
		})
	}

	rangeValue := mapper.spanRange(1, 2, 1, 3)
	want := lsp.Range{
		Start: lsp.Position{Line: 0, Character: 1},
		End:   lsp.Position{Line: 0, Character: 3},
	}
	if rangeValue != want {
		t.Fatalf("expected astral span %+v, got %+v", want, rangeValue)
	}
}

func TestSourcePositionMapper_MultilineSpan(t *testing.T) {
	mapper := newSourcePositionMapper("one\né🙂x\nlast")

	rangeValue := mapper.spanRange(1, 3, 2, 3)
	want := lsp.Range{
		Start: lsp.Position{Line: 0, Character: 2},
		End:   lsp.Position{Line: 1, Character: 3},
	}
	if rangeValue != want {
		t.Fatalf("expected multiline range %+v, got %+v", want, rangeValue)
	}
}

func TestSourcePositionMapper_CRLFDoesNotCountCarriageReturn(t *testing.T) {
	mapper := newSourcePositionMapper("ab\r\nc🙂\r\nd")

	position, valid := mapper.position(2, 3)
	if !valid || position != (lsp.Position{Line: 1, Character: 3}) {
		t.Fatalf("expected column after emoji on CRLF line to be (1,3), got %+v (valid=%t)", position, valid)
	}

	position, valid = mapper.position(3, 1)
	if !valid || position != (lsp.Position{Line: 2, Character: 0}) {
		t.Fatalf("expected third CRLF line start to be (2,0), got %+v (valid=%t)", position, valid)
	}
}

func TestSourcePositionMapper_MalformedSpanFallsBackToStart(t *testing.T) {
	mapper := newSourcePositionMapper("abc\ndef")
	start := lsp.Position{Line: 0, Character: 1}
	want := lsp.Range{Start: start, End: start}

	tests := []struct {
		name                   string
		startLine, startColumn int
		endLine, endColumn     int
	}{
		{name: "missing end line", startLine: 1, startColumn: 2, endLine: 0, endColumn: 4},
		{name: "missing end column", startLine: 1, startColumn: 2, endLine: 1, endColumn: 0},
		{name: "reversed", startLine: 1, startColumn: 3, endLine: 1, endColumn: 2},
		{name: "end line out of bounds", startLine: 1, startColumn: 2, endLine: 3, endColumn: 1},
		{name: "end column out of bounds", startLine: 1, startColumn: 2, endLine: 1, endColumn: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rangeValue := mapper.spanRange(tt.startLine, tt.startColumn, tt.endLine, tt.endColumn)
			if rangeValue != want && tt.name != "reversed" {
				t.Fatalf("expected malformed span to fall back to %+v, got %+v", want, rangeValue)
			}
			if tt.name == "reversed" {
				if rangeValue.Start != (lsp.Position{Line: 0, Character: 2}) || rangeValue.End != rangeValue.Start {
					t.Fatalf("expected reversed span to fall back to start (0,2), got %+v", rangeValue)
				}
			}
		})
	}
}

func TestSourcePositionMapper_InvalidStartIsSafe(t *testing.T) {
	mapper := newSourcePositionMapper("abc\ndef")

	tests := []struct {
		name         string
		line, column int
	}{
		{name: "zero line", line: 0, column: 1},
		{name: "line beyond source", line: 3, column: 1},
		{name: "zero column", line: 1, column: 0},
		{name: "column beyond source", line: 1, column: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			position, valid := mapper.position(tt.line, tt.column)
			if valid {
				t.Fatalf("expected malformed start to be invalid, got %+v", position)
			}
			rangeValue := mapper.spanRange(tt.line, tt.column, 2, 1)
			if rangeValue.Start != position || rangeValue.End != position {
				t.Fatalf("expected malformed span fallback to %+v, got %+v", position, rangeValue)
			}
		})
	}
}
