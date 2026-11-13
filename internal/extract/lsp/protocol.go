package lsp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Position is a zero-based line and column. The client asks for UTF-8, so
// a server that agrees counts the column in bytes; one that does not counts
// UTF-16 units, which match bytes on an ASCII line.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// SymbolKind is the protocol's numbering of what a symbol declares.
type SymbolKind int

const (
	KindModule        SymbolKind = 2
	KindClass         SymbolKind = 5
	KindMethod        SymbolKind = 6
	KindField         SymbolKind = 8
	KindConstructor   SymbolKind = 9
	KindEnum          SymbolKind = 10
	KindInterface     SymbolKind = 11
	KindFunction      SymbolKind = 12
	KindVariable      SymbolKind = 13
	KindConstant      SymbolKind = 14
	KindObject        SymbolKind = 19
	KindEnumMember    SymbolKind = 22
	KindStruct        SymbolKind = 23
	KindTypeParameter SymbolKind = 26
)

// DocumentSymbol is one declaration in a file, with the ones nested in it.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// URI is the file URI of an absolute path, the same form on every OS:
// file:///C:/a/b.rs on Windows and file:///a/b.rs elsewhere.
func URI(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// Path is the absolute path a file URI names.
func Path(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return "", fmt.Errorf("not a file URI: %q", uri)
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p), nil
}
