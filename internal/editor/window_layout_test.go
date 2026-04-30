package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
	"github.com/skybert/gomacs/internal/window"
)

// newWin is a test helper that creates a window displaying a fresh buffer.
func newWin(name string) *window.Window {
	b := buffer.NewWithContent(name, "")
	return window.New(b, 0, 0, 80, 24)
}

// ---------------------------------------------------------------------------
// leafNode / splitLeaf
// ---------------------------------------------------------------------------

func TestLeafNode_IsLeaf(t *testing.T) {
	w := newWin("a")
	n := leafNode(w)
	if n.win != w {
		t.Error("leafNode.win should point to the window")
	}
	if n.children[0] != nil || n.children[1] != nil {
		t.Error("leaf node should have no children")
	}
}

func TestSplitLeaf_Horiz(t *testing.T) {
	w0 := newWin("w0")
	w1 := newWin("w1")
	root := leafNode(w0)
	root = root.splitLeaf(w0, w1, layoutHoriz)

	if root.win != nil {
		t.Error("after split, root should be an internal node")
	}
	if root.dir != layoutHoriz {
		t.Errorf("root.dir = %v, want layoutHoriz", root.dir)
	}
	if root.children[0].win != w0 {
		t.Error("first child should be w0")
	}
	if root.children[1].win != w1 {
		t.Error("second child should be w1")
	}
}

func TestSplitLeaf_Vert(t *testing.T) {
	w0 := newWin("w0")
	w1 := newWin("w1")
	root := leafNode(w0).splitLeaf(w0, w1, layoutVert)
	if root.dir != layoutVert {
		t.Errorf("root.dir = %v, want layoutVert", root.dir)
	}
}

func TestSplitLeaf_DeepNesting(t *testing.T) {
	w0, w1, w2 := newWin("w0"), newWin("w1"), newWin("w2")
	// Start: w0; split w0 horiz → (w0, w1); split w1 vert → (w0, (w1, w2))
	root := leafNode(w0)
	root = root.splitLeaf(w0, w1, layoutHoriz)
	root = root.splitLeaf(w1, w2, layoutVert)

	// root should be horiz(w0, vert(w1, w2))
	if root.dir != layoutHoriz {
		t.Fatalf("root.dir = %v, want layoutHoriz", root.dir)
	}
	if root.children[0].win != w0 {
		t.Error("root left child should be w0")
	}
	if root.children[1].dir != layoutVert {
		t.Errorf("root right child dir = %v, want layoutVert", root.children[1].dir)
	}
	if root.children[1].children[0].win != w1 {
		t.Error("nested left child should be w1")
	}
	if root.children[1].children[1].win != w2 {
		t.Error("nested right child should be w2")
	}
}

// ---------------------------------------------------------------------------
// removeLeaf
// ---------------------------------------------------------------------------

func TestRemoveLeaf_FromHorizPair(t *testing.T) {
	w0, w1 := newWin("w0"), newWin("w1")
	root := leafNode(w0).splitLeaf(w0, w1, layoutHoriz)
	root = root.removeLeaf(w1)
	// Should collapse back to a leaf for w0.
	if root.win != w0 {
		t.Errorf("after removing w1, root.win should be w0, got %v", root.win)
	}
}

func TestRemoveLeaf_KeepsSibling(t *testing.T) {
	w0, w1 := newWin("w0"), newWin("w1")
	root := leafNode(w0).splitLeaf(w0, w1, layoutVert)
	root = root.removeLeaf(w0) // remove left child
	if root.win != w1 {
		t.Errorf("after removing w0, root should be w1, got %v", root.win)
	}
}

func TestRemoveLeaf_DeepTree(t *testing.T) {
	w0, w1, w2 := newWin("w0"), newWin("w1"), newWin("w2")
	// Build: horiz(w0, vert(w1, w2)); remove w2 → horiz(w0, w1)
	root := leafNode(w0)
	root = root.splitLeaf(w0, w1, layoutHoriz)
	root = root.splitLeaf(w1, w2, layoutVert)
	root = root.removeLeaf(w2)

	if root.dir != layoutHoriz {
		t.Fatalf("root.dir after removal = %v, want layoutHoriz", root.dir)
	}
	if root.children[0].win != w0 {
		t.Error("left child should be w0")
	}
	if root.children[1].win != w1 {
		t.Error("right child should collapse to w1")
	}
}

// ---------------------------------------------------------------------------
// applyLayout
// ---------------------------------------------------------------------------

func TestApplyLayout_SingleLeaf(t *testing.T) {
	w := newWin("w")
	leafNode(w).applyLayout(0, 0, 80, 24)
	if w.Top() != 0 || w.Left() != 0 || w.Width() != 80 || w.Height() != 24 {
		t.Errorf("single leaf: got top=%d left=%d width=%d height=%d; want 0 0 80 24",
			w.Top(), w.Left(), w.Width(), w.Height())
	}
}

func TestApplyLayout_HorizSplit(t *testing.T) {
	top, bot := newWin("top"), newWin("bot")
	root := leafNode(top).splitLeaf(top, bot, layoutHoriz)
	root.applyLayout(0, 0, 80, 24)

	if top.Top() != 0 {
		t.Errorf("top window Top = %d, want 0", top.Top())
	}
	if top.Height() != 12 {
		t.Errorf("top window Height = %d, want 12", top.Height())
	}
	if bot.Top() != 12 {
		t.Errorf("bot window Top = %d, want 12", bot.Top())
	}
	if bot.Height() != 12 {
		t.Errorf("bot window Height = %d, want 12", bot.Height())
	}
	// Both should share left=0 and full width.
	if top.Left() != 0 || bot.Left() != 0 {
		t.Errorf("horiz split windows should both have left=0")
	}
	if top.Width() != 80 || bot.Width() != 80 {
		t.Errorf("horiz split windows should both have full width")
	}
}

func TestApplyLayout_VertSplit(t *testing.T) {
	left, right := newWin("left"), newWin("right")
	root := leafNode(left).splitLeaf(left, right, layoutVert)
	root.applyLayout(0, 0, 80, 24)

	// left gets floor((80-1)/2) = 39 columns; right gets the rest.
	if left.Top() != 0 || right.Top() != 0 {
		t.Errorf("vert split windows should share same top row")
	}
	if left.Width() != 39 {
		t.Errorf("left width = %d, want 39", left.Width())
	}
	if right.Left() != 40 {
		t.Errorf("right left = %d, want 40 (39 + 1 separator)", right.Left())
	}
	if left.Width()+1+right.Width() != 80 {
		t.Errorf("widths + separator = %d, want 80", left.Width()+1+right.Width())
	}
}

func TestApplyLayout_MixedHorizThenVert(t *testing.T) {
	// Reproduce the C-x 2 then C-x 3 scenario:
	// horiz(vert(w0, w2), w1)
	w0, w1, w2 := newWin("w0"), newWin("w1"), newWin("w2")
	root := leafNode(w0)
	root = root.splitLeaf(w0, w1, layoutHoriz) // horiz(w0, w1)
	root = root.splitLeaf(w0, w2, layoutVert)  // horiz(vert(w0, w2), w1)
	root.applyLayout(0, 0, 80, 24)

	// w0 and w2 must be side-by-side in the top half.
	if w0.Top() != w2.Top() {
		t.Errorf("w0.Top=%d w2.Top=%d: should share same row", w0.Top(), w2.Top())
	}
	if w0.Top() != 0 {
		t.Errorf("w0.Top=%d, want 0", w0.Top())
	}
	// w1 must be below in the bottom half.
	if w1.Top() != 12 {
		t.Errorf("w1.Top=%d, want 12", w1.Top())
	}
	if w1.Left() != 0 {
		t.Errorf("w1.Left=%d, want 0", w1.Left())
	}
}
