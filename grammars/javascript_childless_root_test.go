package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// At EOF the parser can reduce `program` over nothing on top of the ERROR
// extras that recovery left on the stack. The result builder used to leave a
// childless root's ERROR extras unattached and only stretch the root's range
// over them, so the tree was an empty `program` spanning the whole source
// that reported no error. C's ts_parser__accept splices every extra into the
// root; the result builder now attaches them. Both sources are LawSpec
// template shapes; on the parent both parse as `(program)` with no error.
func TestJavascriptChildlessRootKeepsRecoveredErrors(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "if-instanceof-template", source: "if (v instanceof @@L/E@@)\n"},
		{name: "else-if-template", source: "    else if (value0 instanceof @@LogLevel/Error@@)\n"},
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
			if root.Type(lang) != "program" || root.EndByte() != uint32(len(src)) {
				t.Fatalf("root=%s ends at %d, want program to %d: %s", root.Type(lang), root.EndByte(), len(src), root.SExpr(lang))
			}
			if !root.HasError() {
				t.Fatalf("tree has no error; the input is malformed: %s", root.SExpr(lang))
			}
			if root.ChildCount() == 0 {
				t.Fatalf("root has no children: %s", root.SExpr(lang))
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
