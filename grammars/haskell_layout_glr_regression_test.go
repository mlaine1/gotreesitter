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
//   - typed and record patterns: the full-parse merge cap of one dropped the
//     pattern reading that C keeps as a second link on a merged version. The
//     equation became a top splice with an ERROR, or the whole header was
//     dropped without one.
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
			name:   "typed pattern argument",
			source: "module P where\nprop_a (x :: T) = f x == x\ng y = y\n",
			want:   "(haskell (header (module (module_id))) (declarations (function (variable) (patterns (parens (signature (variable) (name)))) (match (infix (apply (variable) (variable)) (operator) (variable)))) (function (variable) (patterns (variable)) (match (variable)))))",
		},
		{
			name:   "qualified record pattern in case alternative",
			source: "module M where\nf cs = case cs of\n  (P.C{ P.n = n }:_) -> Just n\n",
			want:   "(haskell (header (module (module_id))) (declarations (function (variable) (patterns (variable)) (match (case (variable) (alternatives (alternative (parens (infix (record (qualified (module (module_id)) (constructor)) (field_pattern (qualified (module (module_id)) (field_name (variable))) (variable))) (constructor_operator) (wildcard))) (match (apply (constructor) (variable))))))))))",
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

// TestHaskellAllStacksFailingKeepsErrorTree pins the case where every GLR
// version fails on one lookahead. C pauses them all and recovers the best
// one, so its tree has an ERROR. The engine used to kill each version, end
// the first pass with no stacks alive, and settle on a retry whose root had
// no children and no error: the whole input vanished without a trace.
func TestHaskellAllStacksFailingKeepsErrorTree(t *testing.T) {
	entry := grammars.DetectLanguageByName("haskell")
	if entry == nil {
		t.Fatal("haskell grammar is not registered")
	}
	lang := entry.Language()
	for _, src := range []string{
		// The extended grammar accepts an operator as a record field
		// (`f { (+) = 1 }`, the original input of this test), so the
		// field is a bracketed operator here, which stays an error.
		"operatorUpdate f = f { [+] = 1 }\n",
		"operatorUpdate f = f { [+] = 1 }",
	} {
		done := make(chan struct{})
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
		root := tree.RootNode()
		if !root.HasError() {
			t.Errorf("%q: tree has no error, want one (C reports ERROR): %s", src, root.SExpr(lang))
		}
		if root.NamedChildCount() == 0 {
			t.Errorf("%q: root has no children", src)
		}
		if got, want := root.EndByte(), uint32(len(src)); got != want {
			t.Errorf("%q: root ends at byte %d, want %d", src, got, want)
		}
		tree.Release()
	}
}

// parseHaskellWithin parses src and fails the test when the parse does not
// finish within 30 seconds.
func parseHaskellWithin(t *testing.T, lang *gotreesitter.Language, src string) *gotreesitter.Tree {
	t.Helper()
	done := make(chan struct{})
	var tree *gotreesitter.Tree
	var err error
	go func() {
		defer close(done)
		tree, err = gotreesitter.NewParser(lang).Parse([]byte(src))
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("%q: parse did not finish within 30s", src)
	}
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return tree
}

// withoutFinalNewline returns src and, when src ends in a newline, the same
// input without it.
func withoutFinalNewline(src string) []string {
	if n := len(src); n > 0 && src[n-1] == '\n' {
		return []string{src, src[:n-1]}
	}
	return []string{src}
}

// TestHaskellErrorBeforeRootStaysInTree pins an accepted parse whose error
// recovery left an ERROR on the stack before the real root. C reports ERROR
// nodes from the first line on for this input (a stray `)` in a signature).
// The engine used to stretch the root's range over that ERROR without
// attaching it: the tree covered the whole source, started with the last
// expression, and reported no error at all. (The input found in ormolu, an
// unterminated `"""`, is a valid multiline string in the extended grammar.)
func TestHaskellErrorBeforeRootStaysInTree(t *testing.T) {
	entry := grammars.DetectLanguageByName("haskell")
	if entry == nil {
		t.Fatal("haskell grammar is not registered")
	}
	lang := entry.Language()
	for _, src := range withoutFinalNewline("main :: IO )\nmain = pure ()\n") {
		tree := parseHaskellWithin(t, lang, src)
		root := tree.RootNode()
		if !root.HasError() {
			t.Errorf("%q: tree has no error, want one (C reports ERROR): %s", src, root.SExpr(lang))
		}
		if root.ChildCount() == 0 {
			t.Fatalf("%q: root has no children", src)
		}
		if got := root.Child(0).StartByte(); got != 0 {
			t.Errorf("%q: first root child starts at byte %d, want 0: %s", src, got, root.SExpr(lang))
		}
		if got, want := root.EndByte(), uint32(len(src)); got != want {
			t.Errorf("%q: root ends at byte %d, want %d", src, got, want)
		}
		tree.Release()
	}
}

// TestHaskellUnusableZeroWidthScannerTokenIsError pins a zero-width scanner
// token that the only parse version has no action for. After `x <-` at the
// end of a line, the scanner returns a layout semicolon before `g` that the
// bind statement's state cannot use. C detects an error there (it recovers
// with a MISSING expression; Haskell is not on the C recovery port, so the Go
// tree skips the token inside an empty ERROR instead). The token source used
// to mask the symbol and rescan without telling the parser, so the tree had
// no error. (The input found in ormolu, `[d|    foo bar\n|]`, is a valid
// quote closed in column one in the extended grammar.)
func TestHaskellUnusableZeroWidthScannerTokenIsError(t *testing.T) {
	entry := grammars.DetectLanguageByName("haskell")
	if entry == nil {
		t.Fatal("haskell grammar is not registered")
	}
	lang := entry.Language()
	for _, src := range withoutFinalNewline("f = do\n    x <-\n    g\n") {
		tree := parseHaskellWithin(t, lang, src)
		root := tree.RootNode()
		if !root.HasError() {
			t.Errorf("%q: tree has no error, want one (C reports ERROR)", src)
		}
		if got, want := root.EndByte(), uint32(len(src)); got != want {
			t.Errorf("%q: root ends at byte %d, want %d", src, got, want)
		}
		tree.Release()
	}
}
