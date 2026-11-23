package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// The top-level resync (tryResyncErrorRecoveryMode) keeps the completed
// statements before a failed construct as siblings of the ERROR it builds.
// It used to stop that run at the first popped node with no GOTO, and a
// comment at the top of the file or an ERROR from an earlier resync is such
// a node (an extra), so the new ERROR swallowed every statement above it:
// the program's children were only ERROR nodes. Extras now stay siblings in
// the state they were pushed in, as C keeps them. The expected prefixes are
// the C oracle's (tree-sitter-javascript 58404d8).
func TestJavascriptResyncPreservesStatementsAfterExtras(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		wantPrefix []string
	}{
		{
			name: "leading-comment",
			source: "// Generated. Do not edit.\n" +
				"import * as data from \"./data.mjs\";\n" +
				"export function a() {\n  return 1;\n}\n" +
				"export function b() {\n  return 2;\n}\n" +
				"export class C {\n  m() {\n    return new @@X@@(a());\n  }\n}\n",
			wantPrefix: []string{"comment", "import_statement", "export_statement", "export_statement"},
		},
		{
			name: "earlier-error",
			source: "      new @@SigningKey@@(secretKey),\n" +
				"    );\n" +
				"    const signature = slhDsaSign(value0.value, value1);\n" +
				"    return new @@SignatureBytes@@(signature);\n",
			wantPrefix: []string{"ERROR", "empty_statement", "lexical_declaration"},
		},
	}
	lang := JavascriptLanguage()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(c.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if got := root.Type(lang); got != "program" {
				t.Fatalf("root = %s, want program", got)
			}
			if !root.HasError() {
				t.Fatal("tree has no error; the input is malformed")
			}
			var kinds []string
			for _, ch := range root.Children() {
				if ch.IsNamed() || ch.IsError() {
					kinds = append(kinds, ch.Type(lang))
				}
			}
			if len(kinds) < len(c.wantPrefix) || strings.Join(kinds[:len(c.wantPrefix)], " ") != strings.Join(c.wantPrefix, " ") {
				t.Fatalf("program children = %v, want prefix %v", kinds, c.wantPrefix)
			}
		})
	}
}

// A sibling the resync keeps by GOTO can carry the provisional extra mark of
// an earlier unvalidated top-level replay (nodeFlagRecoveryPlaceholderExtra).
// Kept with that mark, the next reduce skipped it as an extra, the stack died
// and the returned tree stopped before the end of the source (here at byte
// 95 of 145). The resync now clears the mark on a GOTO-validated sibling.
func TestJavascriptResyncPreservedSiblingKeepsTreeToEOF(t *testing.T) {
	src := "/*global navigator*/\n" +
		"const {\n} = require('./constants');\n" +
		"    return match === '\\\\ ? '' : match;\n" +
		"exports.escapeLast = (input, char, lastIdx) => {\n"
	lang := JavascriptLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root ends at byte %d, want %d (the source's end)", got, want)
	}
	found := false
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n.Type(lang) == "lexical_declaration" {
			found = true
		}
		for _, ch := range n.Children() {
			walk(ch)
		}
	}
	walk(root)
	if !found {
		t.Fatalf("lexical_declaration lost: %s", root.SExpr(lang))
	}
}
