package grammars_test

import (
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestHaskellLayoutAndPatternForksMatchC pins trees that the pinned C runtime
// (tree-sitter 0.27.1 with tree-sitter-haskell 0975ef72) produces for inputs
// where two GLR versions disagree. Each want is the C tree.
//
//   - layout: after a signature, the rival bind-pattern version has no action
//     for the layout token. C lexes that version on its own, reads a layout
//     semicolon, and drops it. The engine used to skip the zero-width layout
//     token on that stack, so the version ran past the declaration boundary,
//     read `type` as a variable, and won.
//   - repeated layout ends: closing three nested do blocks emits three
//     zero-width layout ends at one byte while the parser returns to the
//     same state. The zero-width loop guard masked the third one, so the
//     scanner read a layout semicolon and the next declaration became
//     arguments of the last expression.
func TestHaskellLayoutAndPatternForksMatchC(t *testing.T) {
	entry := grammars.DetectLanguageByName("haskell")
	if entry == nil {
		t.Fatal("haskell grammar is not registered")
	}
	lang := entry.Language()
	for _, tc := range []struct {
		name, source, want string
	}{
		{
			name:   "class block ends before type synonym",
			source: "module M where\n\nclass C2 a where\n  m2 :: a\n\ntype S4 = Int\n\ntype S5 = Int\n\nclass Eq a => C5 a where\n  m :: a -> Int\n",
			want:   "(haskell (header (module (module_id))) (declarations (class (name) (type_params (variable)) (class_declarations (signature (variable) (variable)))) (type_synomym (name) (name)) (type_synomym (name) (name)) (class (context (apply (name) (variable))) (name) (type_params (variable)) (class_declarations (signature (variable) (function (variable) (name)))))))",
		},
		{
			name:   "headerless class block ends before type synonym",
			source: "class C2 a where\n  m2 :: a\n\ntype S4 = Int\n",
			want:   "(haskell (declarations (class (name) (type_params (variable)) (class_declarations (signature (variable) (variable)))) (type_synomym (name) (name))))",
		},
		{
			name:   "multi-line signature ends at column one",
			source: "noOverlaps xs  = and [ verts a `notOverlap` verts b\n                     ]\nnoOverflows :: (Integral b, Integral c) =>\n  (forall a. Integral a => a -> a -> a) -> b -> c -> Bool\nnoOverflows op a b = toInteger (a `op` fromIntegral b) == toInteger a `op` toInteger b\n",
			want:   "(haskell (declarations (function (variable) (patterns (variable)) (match (apply (variable) (list (infix (apply (variable) (variable)) (infix_id (variable)) (apply (variable) (variable))))))) (signature (variable) (context (tuple (apply (name) (variable)) (apply (name) (variable))) (function (parens (forall (quantified_variables (variable)) (context (apply (name) (variable)) (function (variable) (function (variable) (variable)))))) (function (variable) (function (variable) (name)))))) (function (variable) (patterns (variable) (variable) (variable)) (match (infix (apply (variable) (parens (infix (variable) (infix_id (variable)) (apply (variable) (variable))))) (operator) (infix (apply (variable) (variable)) (infix_id (variable)) (apply (variable) (variable))))))))",
		},
		{
			name:   "three layout ends at one byte",
			source: "\n        u $ do\n            s $ do\n                g\n                r $ do\n                        (a)\n                        (f)\n    m t = m t\n",
			want:   "(haskell (declarations (top_splice (infix (variable) (operator) (do (exp (infix (variable) (operator) (do (exp (variable)) (exp (infix (variable) (operator) (do (exp (parens (variable))) (exp (parens (variable)))))))))))) (function (variable) (patterns (variable)) (match (apply (variable) (variable))))))",
		},
		{
			name:   "empty input",
			source: "",
			want:   "(haskell)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sources := []string{tc.source}
			if n := len(tc.source); n > 0 {
				// The same input without its final newline.
				sources = append(sources, tc.source[:n-1])
			}
			for _, src := range sources {
				done := make(chan struct{})
				var root *gotreesitter.Node
				var tree *gotreesitter.Tree
				var err error
				go func() {
					defer close(done)
					tree, err = gotreesitter.NewParser(lang).Parse([]byte(src))
				}()
				select {
				case <-done:
				case <-time.After(30 * time.Second):
					t.Fatal("parse did not finish within 30s")
				}
				if err != nil {
					t.Fatal(err)
				}
				root = tree.RootNode()
				if got := root.SExpr(lang); got != tc.want {
					t.Errorf("SExpr = %s\nwant    %s", got, tc.want)
				}
				if root.HasError() {
					t.Errorf("unexpected parse error: %s", root.SExpr(lang))
				}
				if got, want := root.EndByte(), uint32(len(src)); got != want {
					t.Errorf("root ends at byte %d, want %d", got, want)
				}
				tree.Release()
			}
		})
	}
}
