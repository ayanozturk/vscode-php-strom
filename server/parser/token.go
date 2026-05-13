// Package parser implements a hand-written, error-tolerant PHP 8.4 lexer and
// recursive-descent parser producing a typed AST.
//
// Design goals:
//   - Zero allocations in the hot (token scanning) path where possible
//   - Incremental re-parse support: file is divided into top-level declarations;
//     only changed declarations are re-parsed on edit
//   - Full PHP 8.4 support: property hooks, asymmetric visibility, first-class
//     callables, match, fibers, enums, named arguments, readonly classes, etc.
//   - Error-tolerant: parser continues past syntax errors and annotates the AST
package parser

// TokenType identifies every terminal in the PHP grammar.
type TokenType int

const (
	// ─── Special ──────────────────────────────────────────────────────────────
	T_ILLEGAL TokenType = iota
	T_EOF
	T_INLINE_HTML // text outside <?php tags

	// ─── Delimiters ───────────────────────────────────────────────────────────
	T_OPEN_TAG           // <?php
	T_OPEN_TAG_WITH_ECHO // <?=
	T_CLOSE_TAG          // ?>

	// ─── Literals ─────────────────────────────────────────────────────────────
	T_LNUMBER                  // integer literal
	T_DNUMBER                  // float literal
	T_STRING                   // identifier / keyword (context-sensitive)
	T_CONSTANT_ENCAPSED_STRING // 'string'
	T_ENCAPSED_AND_WHITESPACE  // "parts inside double-quoted string"
	T_HEREDOC                  // <<<EOT ... EOT;
	T_NOWDOC                   // <<<'EOT' ... EOT;
	T_VARIABLE                 // $identifier

	// ─── Comments ─────────────────────────────────────────────────────────────
	T_COMMENT       // // or # line comment
	T_DOC_COMMENT   // /** ... */
	T_BLOCK_COMMENT // /* ... */

	// ─── Operators ────────────────────────────────────────────────────────────
	T_PLUS             // +
	T_MINUS            // -
	T_STAR             // *
	T_SLASH            // /
	T_PERCENT          // %
	T_STARSTAR         // **
	T_AMPERSAND        // &
	T_PIPE             // |
	T_CARET            // ^
	T_TILDE            // ~
	T_LSHIFT           // <<
	T_RSHIFT           // >>
	T_DOT              // .
	T_BANG             // !
	T_QUESTION         // ?
	T_QUESTIONQUESTION // ??
	T_AT               // @

	// Assignment
	T_ASSIGN                  // =
	T_PLUS_ASSIGN             // +=
	T_MINUS_ASSIGN            // -=
	T_STAR_ASSIGN             // *=
	T_SLASH_ASSIGN            // /=
	T_PERCENT_ASSIGN          // %=
	T_STARSTAR_ASSIGN         // **=
	T_AMPERSAND_ASSIGN        // &=
	T_PIPE_ASSIGN             // |=
	T_CARET_ASSIGN            // ^=
	T_LSHIFT_ASSIGN           // <<=
	T_RSHIFT_ASSIGN           // >>=
	T_DOT_ASSIGN              // .=
	T_QUESTIONQUESTION_ASSIGN // ??=

	// Comparison
	T_EQ            // ==
	T_IDENTICAL     // ===
	T_NEQ           // != or <>
	T_NOT_IDENTICAL // !==
	T_LT            // <
	T_GT            // >
	T_LTE           // <=
	T_GTE           // >=
	T_SPACESHIP     // <=>

	// Logical
	T_BOOL_AND    // &&
	T_BOOL_OR     // ||
	T_LOGICAL_AND // and
	T_LOGICAL_OR  // or
	T_LOGICAL_XOR // xor

	// Increment / decrement
	T_INC // ++
	T_DEC // --

	// Cast
	T_INT_CAST    // (int) (integer)
	T_DOUBLE_CAST // (float) (double) (real)
	T_STRING_CAST // (string) (binary)
	T_ARRAY_CAST  // (array)
	T_OBJECT_CAST // (object)
	T_BOOL_CAST   // (bool) (boolean)
	T_UNSET_CAST  // (unset)

	// Arrow / scope resolution
	T_OBJECT_OPERATOR          // ->
	T_NULLSAFE_OBJECT_OPERATOR // ?->
	T_DOUBLE_COLON             // ::
	T_ARROW                    // =>
	T_DOUBLE_ARROW             // => (array)

	// ─── Punctuation ──────────────────────────────────────────────────────────
	T_LPAREN    // (
	T_RPAREN    // )
	T_LBRACE    // {
	T_RBRACE    // }
	T_LBRACKET  // [
	T_RBRACKET  // ]
	T_SEMICOLON // ;
	T_COLON     // :
	T_COMMA     // ,
	T_BACKSLASH // \
	T_ELLIPSIS  // ...
	T_HASH      // #  (attribute opener context: #[)
	T_ATTRIBUTE // #[

	// ─── Keywords ─────────────────────────────────────────────────────────────
	T_ABSTRACT
	T_AND
	T_ARRAY
	T_AS
	T_BREAK
	T_CALLABLE
	T_CASE
	T_CATCH
	T_CLASS
	T_CLASS_C // __CLASS__
	T_CLONE
	T_CONST
	T_CONTINUE
	T_DECLARE
	T_DEFAULT
	T_DIR // __DIR__
	T_DO
	T_ECHO
	T_ELSE
	T_ELSEIF
	T_EMPTY
	T_ENDDECLARE
	T_ENDFOR
	T_ENDFOREACH
	T_ENDIF
	T_ENDSWITCH
	T_ENDWHILE
	T_ENUM
	T_EVAL
	T_EXCEPT // reserved for future use
	T_EXIT
	T_EXTENDS
	T_FILE // __FILE__
	T_FINAL
	T_FINALLY
	T_FN
	T_FOR
	T_FOREACH
	T_FUNCTION
	T_FUNCTION_C // __FUNCTION__
	T_GLOBAL
	T_GOTO
	T_HALT_COMPILER
	T_IF
	T_IMPLEMENTS
	T_INCLUDE
	T_INCLUDE_ONCE
	T_INSTANCEOF
	T_INSTEADOF
	T_INTERFACE
	T_ISSET
	T_LINE // __LINE__
	T_LIST
	T_MATCH
	T_METHOD_C // __METHOD__
	T_NAMESPACE
	T_NAMESPACE_C // __NAMESPACE__
	T_NEW
	T_OR
	T_PRINT
	T_PRIVATE
	T_PROTECTED
	T_PUBLIC
	T_READONLY
	T_REQUIRE
	T_REQUIRE_ONCE
	T_RETURN
	T_STATIC
	T_SWITCH
	T_THROW
	T_TRAIT
	T_TRAIT_C // __TRAIT__
	T_TRY
	T_UNSET
	T_USE
	T_VAR
	T_WHILE
	T_XOR
	T_YIELD
	T_YIELD_FROM

	// ─── Type keywords ────────────────────────────────────────────────────────
	T_NULL_TYPE     // null
	T_TRUE_TYPE     // true
	T_FALSE_TYPE    // false
	T_INT_TYPE      // int
	T_FLOAT_TYPE    // float
	T_STRING_TYPE   // string
	T_BOOL_TYPE     // bool
	T_VOID_TYPE     // void
	T_NEVER_TYPE    // never
	T_MIXED_TYPE    // mixed
	T_OBJECT_TYPE   // object
	T_ITERABLE_TYPE // iterable
	T_SELF_TYPE     // self
	T_STATIC_TYPE   // static
	T_PARENT_TYPE   // parent

	// ─── PHP 8.x additions ────────────────────────────────────────────────────
	T_NAMED_ARGUMENT       // identifier: (before argument)
	T_FIRST_CLASS_CALLABLE // ...  inside f(...)
)

// keywords maps lowercase PHP keywords to their token types.
var keywords = map[string]TokenType{
	"abstract":        T_ABSTRACT,
	"and":             T_LOGICAL_AND,
	"array":           T_ARRAY,
	"as":              T_AS,
	"break":           T_BREAK,
	"callable":        T_CALLABLE,
	"case":            T_CASE,
	"catch":           T_CATCH,
	"class":           T_CLASS,
	"__class__":       T_CLASS_C,
	"clone":           T_CLONE,
	"const":           T_CONST,
	"continue":        T_CONTINUE,
	"declare":         T_DECLARE,
	"default":         T_DEFAULT,
	"__dir__":         T_DIR,
	"do":              T_DO,
	"echo":            T_ECHO,
	"else":            T_ELSE,
	"elseif":          T_ELSEIF,
	"empty":           T_EMPTY,
	"enddeclare":      T_ENDDECLARE,
	"endfor":          T_ENDFOR,
	"endforeach":      T_ENDFOREACH,
	"endif":           T_ENDIF,
	"endswitch":       T_ENDSWITCH,
	"endwhile":        T_ENDWHILE,
	"enum":            T_ENUM,
	"eval":            T_EVAL,
	"exit":            T_EXIT,
	"die":             T_EXIT,
	"extends":         T_EXTENDS,
	"__file__":        T_FILE,
	"final":           T_FINAL,
	"finally":         T_FINALLY,
	"fn":              T_FN,
	"for":             T_FOR,
	"foreach":         T_FOREACH,
	"function":        T_FUNCTION,
	"__function__":    T_FUNCTION_C,
	"global":          T_GLOBAL,
	"goto":            T_GOTO,
	"__halt_compiler": T_HALT_COMPILER,
	"if":              T_IF,
	"implements":      T_IMPLEMENTS,
	"include":         T_INCLUDE,
	"include_once":    T_INCLUDE_ONCE,
	"instanceof":      T_INSTANCEOF,
	"insteadof":       T_INSTEADOF,
	"interface":       T_INTERFACE,
	"isset":           T_ISSET,
	"__line__":        T_LINE,
	"list":            T_LIST,
	"match":           T_MATCH,
	"__method__":      T_METHOD_C,
	"namespace":       T_NAMESPACE,
	"__namespace__":   T_NAMESPACE_C,
	"new":             T_NEW,
	"null":            T_NULL_TYPE,
	"or":              T_LOGICAL_OR,
	"print":           T_PRINT,
	"private":         T_PRIVATE,
	"protected":       T_PROTECTED,
	"public":          T_PUBLIC,
	"readonly":        T_READONLY,
	"require":         T_REQUIRE,
	"require_once":    T_REQUIRE_ONCE,
	"return":          T_RETURN,
	"static":          T_STATIC,
	"switch":          T_SWITCH,
	"throw":           T_THROW,
	"trait":           T_TRAIT,
	"__trait__":       T_TRAIT_C,
	"true":            T_TRUE_TYPE,
	"false":           T_FALSE_TYPE,
	"try":             T_TRY,
	"unset":           T_UNSET,
	"use":             T_USE,
	"var":             T_VAR,
	"while":           T_WHILE,
	"xor":             T_LOGICAL_XOR,
	"yield":           T_YIELD,
	// type keywords
	"int":      T_INT_TYPE,
	"float":    T_FLOAT_TYPE,
	"string":   T_STRING_TYPE,
	"bool":     T_BOOL_TYPE,
	"void":     T_VOID_TYPE,
	"never":    T_NEVER_TYPE,
	"mixed":    T_MIXED_TYPE,
	"object":   T_OBJECT_TYPE,
	"iterable": T_ITERABLE_TYPE,
	"self":     T_SELF_TYPE,
	"parent":   T_PARENT_TYPE,
}

// Token is a single lexed terminal.
type Token struct {
	Type     TokenType
	Value    string // raw text as it appears in source
	StartPos int    // byte offset of first character
	EndPos   int    // byte offset just past last character
	Line     int    // 1-based line number
	Col      int    // 0-based column of first byte
}
