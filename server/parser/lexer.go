package parser

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lexer tokenises a PHP source string.
// It is designed for zero-copy scanning: the src string is never copied,
// tokens carry byte offsets into it.
type Lexer struct {
	src   string
	pos   int  // current byte position
	line  int  // current 1-based line
	col   int  // current 0-based column
	inPHP bool // false = inline HTML mode
	inStr bool // inside a double-quoted interpolated string

	peeked *Token
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src, line: 1, col: 0}
}

// Next returns the next token, advancing the lexer.
func (l *Lexer) Next() Token {
	if l.peeked != nil {
		t := *l.peeked
		l.peeked = nil
		return t
	}
	return l.scan()
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() Token {
	if l.peeked == nil {
		t := l.scan()
		l.peeked = &t
	}
	return *l.peeked
}

// ─── Internal scanner ────────────────────────────────────────────────────────

func (l *Lexer) scan() Token {
	if l.pos >= len(l.src) {
		return l.tok(T_EOF, "")
	}

	if !l.inPHP {
		return l.scanInlineHTML()
	}

	l.skipWhitespace()

	if l.pos >= len(l.src) {
		return l.tok(T_EOF, "")
	}

	start := l.pos
	startLine := l.line
	startCol := l.col

	ch := l.src[l.pos]

	// ── Comments ─────────────────────────────────────────────────────────────
	if ch == '/' {
		if l.pos+1 < len(l.src) {
			next := l.src[l.pos+1]
			if next == '/' {
				return l.scanLineComment()
			}
			if next == '*' {
				return l.scanBlockComment()
			}
		}
	}
	if ch == '#' {
		// Could be #[ (attribute) or # comment
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '[' {
			l.advance()
			l.advance()
			return Token{Type: T_ATTRIBUTE, Value: "#[", StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
		}
		return l.scanLineComment()
	}

	// ── Strings ───────────────────────────────────────────────────────────────
	if ch == '\'' {
		return l.scanSingleQuotedString()
	}
	if ch == '"' {
		return l.scanDoubleQuotedString()
	}
	if ch == '`' {
		return l.scanBacktickString()
	}
	// Heredoc / Nowdoc: <<<
	if ch == '<' && l.pos+2 < len(l.src) && l.src[l.pos+1] == '<' && l.src[l.pos+2] == '<' {
		return l.scanHeredoc()
	}

	// ── Variables ─────────────────────────────────────────────────────────────
	if ch == '$' {
		return l.scanVariable()
	}

	// ── Numbers ───────────────────────────────────────────────────────────────
	if isDigit(ch) || (ch == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1])) {
		return l.scanNumber()
	}

	// ── Identifiers & keywords ────────────────────────────────────────────────
	if isIdentStart(ch) {
		return l.scanIdentOrKeyword()
	}

	// ── Close tag ─────────────────────────────────────────────────────────────
	if ch == '?' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '>' {
		l.advance()
		l.advance()
		l.inPHP = false
		return Token{Type: T_CLOSE_TAG, Value: "?>", StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}

	// ── Multi-character operators ─────────────────────────────────────────────
	return l.scanOperator()
}

func (l *Lexer) scanInlineHTML() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	for l.pos < len(l.src) {
		if l.src[l.pos] == '<' {
			if strings.HasPrefix(l.src[l.pos:], "<?php") && (l.pos+5 >= len(l.src) || !isIdentChar(l.src[l.pos+5])) {
				if l.pos > start {
					return Token{Type: T_INLINE_HTML, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
				}
				end := l.pos + 5
				for i := 0; i < 5; i++ {
					l.advance()
				}
				l.inPHP = true
				return Token{Type: T_OPEN_TAG, Value: "<?php", StartPos: start, EndPos: end, Line: startLine, Col: startCol}
			}
			if strings.HasPrefix(l.src[l.pos:], "<?=") {
				if l.pos > start {
					return Token{Type: T_INLINE_HTML, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
				}
				end := l.pos + 3
				l.advance()
				l.advance()
				l.advance()
				l.inPHP = true
				return Token{Type: T_OPEN_TAG_WITH_ECHO, Value: "<?=", StartPos: start, EndPos: end, Line: startLine, Col: startCol}
			}
		}
		l.advance()
	}
	if l.pos > start {
		return Token{Type: T_INLINE_HTML, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}
	return l.tok(T_EOF, "")
}

func (l *Lexer) scanLineComment() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advance()
	}
	return Token{Type: T_COMMENT, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanBlockComment() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	isDoc := l.pos+2 < len(l.src) && l.src[l.pos+2] == '*'
	l.advance()
	l.advance() // consume /*
	for l.pos < len(l.src) {
		if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			l.advance()
			l.advance()
			break
		}
		l.advance()
	}
	tt := T_BLOCK_COMMENT
	if isDoc {
		tt = T_DOC_COMMENT
	}
	return Token{Type: tt, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanVariable() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	l.advance() // $
	for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
		l.advance()
	}
	return Token{Type: T_VARIABLE, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanIdentOrKeyword() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
		l.advance()
	}
	value := l.src[start:l.pos]
	lower := strings.ToLower(value)
	if tt, ok := keywords[lower]; ok {
		return Token{Type: tt, Value: value, StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}
	return Token{Type: T_STRING, Value: value, StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanNumber() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	isFloat := false

	// Hex
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) && (l.src[l.pos+1] == 'x' || l.src[l.pos+1] == 'X') {
		l.advance()
		l.advance()
		for l.pos < len(l.src) && isHexDigit(l.src[l.pos]) {
			l.advance()
		}
		return Token{Type: T_LNUMBER, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}
	// Binary
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) && (l.src[l.pos+1] == 'b' || l.src[l.pos+1] == 'B') {
		l.advance()
		l.advance()
		for l.pos < len(l.src) && (l.src[l.pos] == '0' || l.src[l.pos] == '1' || l.src[l.pos] == '_') {
			l.advance()
		}
		return Token{Type: T_LNUMBER, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}
	// Octal
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) && (l.src[l.pos+1] == 'o' || l.src[l.pos+1] == 'O') {
		l.advance()
		l.advance()
		for l.pos < len(l.src) && (l.src[l.pos] >= '0' && l.src[l.pos] <= '7' || l.src[l.pos] == '_') {
			l.advance()
		}
		return Token{Type: T_LNUMBER, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}

	for l.pos < len(l.src) && (isDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
		l.advance()
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' && (l.pos+1 >= len(l.src) || l.src[l.pos+1] != '.') {
		isFloat = true
		l.advance()
		for l.pos < len(l.src) && (isDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
			l.advance()
		}
	}
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		isFloat = true
		l.advance()
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.advance()
		}
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.advance()
		}
	}
	tt := T_LNUMBER
	if isFloat {
		tt = T_DNUMBER
	}
	return Token{Type: tt, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanSingleQuotedString() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	l.advance() // '
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\\' && l.pos+1 < len(l.src) {
			l.advance()
			l.advance()
			continue
		}
		if ch == '\'' {
			l.advance()
			break
		}
		l.advance()
	}
	return Token{Type: T_CONSTANT_ENCAPSED_STRING, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanDoubleQuotedString() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	l.advance() // "
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '\\' && l.pos+1 < len(l.src) {
			l.advance()
			l.advance()
			continue
		}
		if ch == '"' {
			l.advance()
			break
		}
		l.advance()
	}
	return Token{Type: T_ENCAPSED_AND_WHITESPACE, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanBacktickString() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	l.advance() // `
	for l.pos < len(l.src) {
		if l.src[l.pos] == '`' {
			l.advance()
			break
		}
		l.advance()
	}
	return Token{Type: T_ENCAPSED_AND_WHITESPACE, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanHeredoc() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	l.advance()
	l.advance()
	l.advance() // <<<

	// Optional whitespace
	for l.pos < len(l.src) && l.src[l.pos] == ' ' {
		l.advance()
	}

	isNowdoc := false
	var label string

	if l.pos < len(l.src) && l.src[l.pos] == '\'' {
		isNowdoc = true
		l.advance()
		lStart := l.pos
		for l.pos < len(l.src) && l.src[l.pos] != '\'' {
			l.advance()
		}
		label = l.src[lStart:l.pos]
		if l.pos < len(l.src) {
			l.advance()
		}
	} else if l.pos < len(l.src) && l.src[l.pos] == '"' {
		l.advance()
		lStart := l.pos
		for l.pos < len(l.src) && l.src[l.pos] != '"' {
			l.advance()
		}
		label = l.src[lStart:l.pos]
		if l.pos < len(l.src) {
			l.advance()
		}
	} else {
		lStart := l.pos
		for l.pos < len(l.src) && isIdentChar(l.src[l.pos]) {
			l.advance()
		}
		label = l.src[lStart:l.pos]
	}

	// Skip to end of opening line
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advance()
	}
	if l.pos < len(l.src) {
		l.advance() // \n
	}

	// Scan body until closing label at start of line
	for l.pos < len(l.src) {
		lineStart := l.pos
		// Skip optional leading whitespace (flexible heredoc PHP 7.3+)
		wsEnd := l.pos
		for wsEnd < len(l.src) && (l.src[wsEnd] == ' ' || l.src[wsEnd] == '\t') {
			wsEnd++
		}
		if strings.HasPrefix(l.src[wsEnd:], label) {
			after := wsEnd + len(label)
			if after >= len(l.src) || l.src[after] == ';' || l.src[after] == '\n' || l.src[after] == '\r' {
				l.pos = after
				if l.pos < len(l.src) && l.src[l.pos] == ';' {
					l.advance()
				}
				break
			}
		}
		_ = lineStart
		for l.pos < len(l.src) && l.src[l.pos] != '\n' {
			l.advance()
		}
		if l.pos < len(l.src) {
			l.advance()
		}
	}

	tt := T_HEREDOC
	if isNowdoc {
		tt = T_NOWDOC
	}
	return Token{Type: tt, Value: l.src[start:l.pos], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

func (l *Lexer) scanOperator() Token {
	start := l.pos
	startLine := l.line
	startCol := l.col
	ch := l.src[l.pos]

	mkTok := func(tt TokenType, n int) Token {
		for i := 0; i < n; i++ {
			l.advance()
		}
		return Token{Type: tt, Value: l.src[start : start+n], StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
	}

	peek := func(n int) byte {
		if l.pos+n < len(l.src) {
			return l.src[l.pos+n]
		}
		return 0
	}

	switch ch {
	case '+':
		if peek(1) == '+' {
			return mkTok(T_INC, 2)
		}
		if peek(1) == '=' {
			return mkTok(T_PLUS_ASSIGN, 2)
		}
		return mkTok(T_PLUS, 1)
	case '-':
		if peek(1) == '-' {
			return mkTok(T_DEC, 2)
		}
		if peek(1) == '=' {
			return mkTok(T_MINUS_ASSIGN, 2)
		}
		if peek(1) == '>' {
			return mkTok(T_OBJECT_OPERATOR, 2)
		}
		return mkTok(T_MINUS, 1)
	case '*':
		if peek(1) == '*' {
			if peek(2) == '=' {
				return mkTok(T_STARSTAR_ASSIGN, 3)
			}
			return mkTok(T_STARSTAR, 2)
		}
		if peek(1) == '=' {
			return mkTok(T_STAR_ASSIGN, 2)
		}
		return mkTok(T_STAR, 1)
	case '/':
		if peek(1) == '=' {
			return mkTok(T_SLASH_ASSIGN, 2)
		}
		return mkTok(T_SLASH, 1)
	case '%':
		if peek(1) == '=' {
			return mkTok(T_PERCENT_ASSIGN, 2)
		}
		return mkTok(T_PERCENT, 1)
	case '=':
		if peek(1) == '=' {
			if peek(2) == '=' {
				return mkTok(T_IDENTICAL, 3)
			}
			return mkTok(T_EQ, 2)
		}
		if peek(1) == '>' {
			return mkTok(T_DOUBLE_ARROW, 2)
		}
		return mkTok(T_ASSIGN, 1)
	case '!':
		if peek(1) == '=' {
			if peek(2) == '=' {
				return mkTok(T_NOT_IDENTICAL, 3)
			}
			return mkTok(T_NEQ, 2)
		}
		return mkTok(T_BANG, 1)
	case '<':
		if peek(1) == '=' {
			if peek(2) == '>' {
				return mkTok(T_SPACESHIP, 3)
			}
			return mkTok(T_LTE, 2)
		}
		if peek(1) == '<' {
			if peek(2) == '=' {
				return mkTok(T_LSHIFT_ASSIGN, 3)
			}
			return mkTok(T_LSHIFT, 2)
		}
		if peek(1) == '>' {
			return mkTok(T_NEQ, 2)
		}
		return mkTok(T_LT, 1)
	case '>':
		if peek(1) == '=' {
			return mkTok(T_GTE, 2)
		}
		if peek(1) == '>' {
			if peek(2) == '=' {
				return mkTok(T_RSHIFT_ASSIGN, 3)
			}
			return mkTok(T_RSHIFT, 2)
		}
		return mkTok(T_GT, 1)
	case '&':
		if peek(1) == '&' {
			return mkTok(T_BOOL_AND, 2)
		}
		if peek(1) == '=' {
			return mkTok(T_AMPERSAND_ASSIGN, 2)
		}
		return mkTok(T_AMPERSAND, 1)
	case '|':
		if peek(1) == '|' {
			return mkTok(T_BOOL_OR, 2)
		}
		if peek(1) == '=' {
			return mkTok(T_PIPE_ASSIGN, 2)
		}
		return mkTok(T_PIPE, 1)
	case '^':
		if peek(1) == '=' {
			return mkTok(T_CARET_ASSIGN, 2)
		}
		return mkTok(T_CARET, 1)
	case '?':
		if peek(1) == '?' {
			if peek(2) == '=' {
				return mkTok(T_QUESTIONQUESTION_ASSIGN, 3)
			}
			return mkTok(T_QUESTIONQUESTION, 2)
		}
		if peek(1) == '-' && peek(2) == '>' {
			return mkTok(T_NULLSAFE_OBJECT_OPERATOR, 3)
		}
		return mkTok(T_QUESTION, 1)
	case '.':
		if peek(1) == '.' && peek(2) == '.' {
			return mkTok(T_ELLIPSIS, 3)
		}
		if peek(1) == '=' {
			return mkTok(T_DOT_ASSIGN, 2)
		}
		return mkTok(T_DOT, 1)
	case ':':
		if peek(1) == ':' {
			return mkTok(T_DOUBLE_COLON, 2)
		}
		return mkTok(T_COLON, 1)
	case '\\':
		return mkTok(T_BACKSLASH, 1)
	case '(':
		return mkTok(T_LPAREN, 1)
	case ')':
		return mkTok(T_RPAREN, 1)
	case '{':
		return mkTok(T_LBRACE, 1)
	case '}':
		return mkTok(T_RBRACE, 1)
	case '[':
		return mkTok(T_LBRACKET, 1)
	case ']':
		return mkTok(T_RBRACKET, 1)
	case ';':
		return mkTok(T_SEMICOLON, 1)
	case ',':
		return mkTok(T_COMMA, 1)
	case '~':
		return mkTok(T_TILDE, 1)
	case '@':
		return mkTok(T_AT, 1)
	}

	// Unknown — consume as illegal and advance
	l.advance()
	return Token{Type: T_ILLEGAL, Value: string(ch), StartPos: start, EndPos: l.pos, Line: startLine, Col: startCol}
}

// ─── Utilities ───────────────────────────────────────────────────────────────

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) advance() {
	if l.pos >= len(l.src) {
		return
	}
	if l.src[l.pos] == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
	_, size := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += size
}

func (l *Lexer) tok(tt TokenType, value string) Token {
	return Token{Type: tt, Value: value, StartPos: l.pos, EndPos: l.pos, Line: l.line, Col: l.col}
}

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }
func isHexDigit(ch byte) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch >= 0x80
}
func isIdentChar(ch byte) bool { return isIdentStart(ch) || isDigit(ch) }

// AllTokens is a convenience for tests: returns every token until EOF.
func AllTokens(src string) []Token {
	l := NewLexer(src)
	var tokens []Token
	for {
		t := l.Next()
		tokens = append(tokens, t)
		if t.Type == T_EOF {
			break
		}
	}
	return tokens
}

// Ensure unicode is used to avoid import errors for non-ASCII idents
var _ = unicode.IsLetter
var _ = strings.ToLower
