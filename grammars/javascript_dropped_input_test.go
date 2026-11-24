package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Without the C recovery port, the lexer skips bytes the parse state's lex
// mode cannot start a token with, and the next token starts after a gap.
// tryMaterializeSkippedRealGap covers such a gap with an ERROR, but refused
// when the top of the stack was a closed ERROR (one a top-level resync
// built); the caller then killed the only stack, the parse ended at the
// skipped byte, and the tree stopped short of the source. C never drops
// input: ts_parser__lex surfaces it as an error subtree. These sources are
// LawSpec template shapes (`@@Name/Variant@@`); on the parent the roots end
// at bytes 7/9, 32/45 and 50/105.
func TestJavascriptDroppedInputStaysInTree(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "trailing", source: "else @L)\n"},
		{name: "mid-file", source: "x = 1;\nelse if (v instanceof @@L/E@@)\ny = 2;\n"},
		{
			name: "class-template",
			source: "export class A {\n  m(v) {\n    if (v instanceof @@L/D@@) {\n      a(v);\n    }\n  }\n}\n" +
				"export function g() {}\n",
		},
	}
	lang := JavascriptLanguage()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(c.source)
			tree, err := gotreesitter.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if got, want := root.EndByte(), uint32(len(src)); got != want {
				t.Fatalf("root ends at byte %d, want %d (the source's end): %s", got, want, root.SExpr(lang))
			}
			if !root.HasError() {
				t.Fatalf("tree has no error; the input is malformed: %s", root.SExpr(lang))
			}
			// Every byte that is not whitespace lies inside some leaf.
			covered := make([]bool, len(src))
			var walk func(n *gotreesitter.Node)
			walk = func(n *gotreesitter.Node) {
				kids := n.Children()
				if len(kids) == 0 {
					for i := n.StartByte(); i < n.EndByte() && int(i) < len(src); i++ {
						covered[i] = true
					}
					return
				}
				for _, ch := range kids {
					walk(ch)
				}
			}
			walk(root)
			for i, b := range src {
				if !covered[i] && b != ' ' && b != '\n' && b != '\t' {
					t.Fatalf("byte %d (%q) is in no leaf: %s", i, b, root.SExpr(lang))
				}
			}
		})
	}
}
