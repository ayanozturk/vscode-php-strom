package indexer

import "testing"

func TestBuildLineOffsets(t *testing.T) {
	src := "<?php\nclass Foo\n{\n}\n"
	offsets := buildLineOffsets(src)
	// Line 0 starts at 0, line 1 starts after first \n, etc.
	expected := []int{0, 6, 16, 18, 20}
	if len(offsets) != len(expected) {
		t.Fatalf("expected %d offsets, got %d: %v", len(expected), len(offsets), offsets)
	}
	for i, want := range expected {
		if offsets[i] != want {
			t.Errorf("offsets[%d]: want %d, got %d", i, want, offsets[i])
		}
	}
}

func TestOffsetToLineChar(t *testing.T) {
	src := "<?php\nclass Foo\n{\n}\n"
	offsets := buildLineOffsets(src)

	tests := []struct {
		offset   int
		wantLine uint32
		wantChar uint32
	}{
		{0, 0, 0},  // start of file
		{5, 0, 5},  // end of first line before \n
		{6, 1, 0},  // start of "class Foo"
		{11, 1, 5}, // "F" in "Foo"
		{16, 2, 0}, // "{"
	}
	for _, tc := range tests {
		line, char := offsetToLineChar(offsets, tc.offset)
		if line != tc.wantLine || char != tc.wantChar {
			t.Errorf("offset %d: want (%d,%d), got (%d,%d)", tc.offset, tc.wantLine, tc.wantChar, line, char)
		}
	}
}

func TestExtractSymbolsHasLSPPositions(t *testing.T) {
	src := "<?php\nclass MyClass {}\n"
	syms := extractSymbols("file:///test.php", src)
	if len(syms) == 0 {
		t.Fatal("expected at least one symbol")
	}
	cls := syms[0]
	if cls.Name != "MyClass" {
		t.Fatalf("expected MyClass, got %s", cls.Name)
	}
	// "class" keyword is on line 1 (0-based), so StartLine should be 1
	if cls.StartLine != 1 {
		t.Errorf("expected StartLine=1 (0-based), got %d", cls.StartLine)
	}
}
