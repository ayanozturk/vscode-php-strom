package indexer

import (
	"context"
	"strings"
	"testing"
)

const maxFuzzSourceBytes = 64 << 10

func FuzzParseSourceProductionPaths(f *testing.F) {
	seeds := []string{
		"",
		"<?php",
		"<?php if () {",
		"<?php function broken(] { return (((; }",
		"<?phpA[0(00",
		"<?php $value = <<<'TXT'\nunterminated",
		"<?php /* unterminated comment",
		"<?php class Example { public function run(): never { throw new \\RuntimeException(); }",
		string([]byte("<?php \xff\xfe\x00")),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > maxFuzzSourceBytes {
			t.Skip()
		}

		const uri = "file:///security-fuzz.php"
		parsePaths := []struct {
			name  string
			parse func(context.Context, string, string) ParsedFile
		}{
			{name: "diagnostics", parse: ParseSourceWithContext},
			{name: "index", parse: ParseSourceForIndexWithContext},
		}

		for _, path := range parsePaths {
			parsed := path.parse(context.Background(), uri, source)
			assertSafeFuzzResult(t, path.name, parsed, uri, source)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			cancelledSource := "<?php\n$sentinel = 1;\n" + source
			cancelled := path.parse(ctx, uri, cancelledSource)
			assertSafeFuzzResult(t, path.name+" cancelled", cancelled, uri, cancelledSource)
			assertCancellationReported(t, path.name, cancelled.Errors)
		}
	})
}

func assertSafeFuzzResult(t *testing.T, path string, parsed ParsedFile, uri, source string) {
	t.Helper()
	if parsed.URI != uri || parsed.Text != source || parsed.Bytes != len(source) {
		t.Fatalf("%s path corrupted source metadata", path)
	}
	for _, parseErr := range parsed.Errors {
		if strings.HasPrefix(parseErr, "Parser panic:") {
			t.Fatalf("%s path recovered an internal parser panic: %s", path, parseErr)
		}
	}
}

func assertCancellationReported(t *testing.T, path string, errors []string) {
	t.Helper()
	for _, parseErr := range errors {
		if strings.Contains(parseErr, "parser context cancelled: context canceled") {
			return
		}
	}
	t.Fatalf("%s path did not report pre-cancelled parsing: %v", path, errors)
}
