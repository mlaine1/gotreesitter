package gotreesitter

import "testing"

func TestNormalizeRootSourceStartUsesFirstRetainedChildOwnership(t *testing.T) {
	tests := []struct {
		name       string
		source     []byte
		rootStart  uint32
		childStart uint32
		wantStart  uint32
		wantPoint  Point
	}{
		{
			name:       "unowned leading newline",
			source:     []byte("\nvalue"),
			childStart: 1,
			wantStart:  1,
			wantPoint:  Point{Row: 1},
		},
		{
			name:       "unowned BOM and newline",
			source:     []byte("\xef\xbb\xbf\nvalue"),
			childStart: 4,
			wantStart:  4,
			wantPoint:  Point{Row: 1},
		},
		{
			name:       "child owns leading recovery bytes",
			source:     []byte("\nvalue"),
			childStart: 0,
			wantStart:  0,
		},
		{
			name:       "nontrivia gap remains covered",
			source:     []byte("x value"),
			childStart: 2,
			wantStart:  0,
		},
		{
			name:       "dropped source token remains covered",
			source:     []byte("\nvalue"),
			rootStart:  3,
			childStart: 3,
			wantStart:  1,
			wantPoint:  Point{Row: 1},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			arena := newNodeArena(arenaClassFull)
			childPoint := advancePointByBytes(Point{}, test.source[:test.childStart])
			child := newLeafNodeInArena(
				arena,
				1,
				true,
				test.childStart,
				uint32(len(test.source)),
				childPoint,
				advancePointByBytes(Point{}, test.source),
			)
			root := newParentNodeInArena(arena, 2, true, []*Node{child}, nil, 0)
			root.startByte = test.rootStart
			root.startPoint = advancePointByBytes(Point{}, test.source[:test.rootStart])

			(&Parser{}).normalizeRootSourceStart(root, test.source)

			if root.startByte != test.wantStart {
				t.Fatalf("root start = %d, want %d", root.startByte, test.wantStart)
			}
			if root.startPoint != test.wantPoint {
				t.Fatalf("root point = %#v, want %#v", root.startPoint, test.wantPoint)
			}
		})
	}
}

func TestNormalizeRootSourceStartPreservesIncludedRangeSpan(t *testing.T) {
	source := []byte("\nvalue")
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(
		arena,
		1,
		true,
		1,
		uint32(len(source)),
		Point{Row: 1},
		Point{Row: 1, Column: 5},
	)
	root := newParentNodeInArena(arena, 2, true, []*Node{child}, nil, 0)
	root.startByte = 0
	root.startPoint = Point{}
	parser := &Parser{included: []Range{{StartByte: 0, EndByte: uint32(len(source))}}}

	parser.normalizeRootSourceStart(root, source)

	if root.startByte != 0 || root.startPoint != (Point{}) {
		t.Fatalf("included-range root moved to %d at %#v", root.startByte, root.startPoint)
	}
}

// TestRestoreRootStartAtLeadingZeroWidthScannerExtra pins the root start for
// a zero-width scanner extra that precedes the leading whitespace. In C the
// root starts at its first leaf, zero-width leaves included.
func TestRestoreRootStartAtLeadingZeroWidthScannerExtra(t *testing.T) {
	source := []byte("\n  c x")
	for _, test := range []struct {
		name        string
		leadStart   uint32
		leadEnd     uint32
		leadScanner bool
		rootStart   uint32
		wantStart   uint32
	}{
		{name: "zero-width scanner extra at byte zero", leadScanner: true, rootStart: 3, wantStart: 0},
		{name: "zero-width scanner extra after the newline", leadStart: 1, leadEnd: 1, leadScanner: true, rootStart: 3, wantStart: 1},
		{name: "extra is not a scanner token", rootStart: 3, wantStart: 3},
		{name: "scanner extra has width", leadEnd: 1, leadScanner: true, rootStart: 3, wantStart: 3},
		{name: "scanner extra at the first source byte", leadStart: 3, leadEnd: 3, leadScanner: true, rootStart: 3, wantStart: 3},
		{name: "root does not start at the first source byte", leadScanner: true, rootStart: 5, wantStart: 5},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			arena := newNodeArena(arenaClassFull)
			point := func(b uint32) Point { return advancePointByBytes(Point{}, source[:b]) }
			lead := newLeafNodeInArena(arena, 3, false, test.leadStart, test.leadEnd, point(test.leadStart), point(test.leadEnd))
			lead.setExtra(true)
			lead.setExternalScannerToken(test.leadScanner)
			child := newLeafNodeInArena(arena, 1, true, test.rootStart, uint32(len(source)), point(test.rootStart), point(uint32(len(source))))
			root := newParentNodeInArena(arena, 2, true, []*Node{child}, nil, 0)
			tree := &Tree{root: root}

			restoreRootStartAtLeadingZeroWidthScannerExtra(tree, resultRootExtraSplit{allExtras: []*Node{lead}}, source)

			if root.startByte != test.wantStart {
				t.Fatalf("root start = %d, want %d", root.startByte, test.wantStart)
			}
			if want := point(test.wantStart); root.startPoint != want {
				t.Fatalf("root point = %#v, want %#v", root.startPoint, want)
			}
		})
	}
}
