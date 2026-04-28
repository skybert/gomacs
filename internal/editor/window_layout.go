package editor

import "github.com/skybert/gomacs/internal/window"

// layoutDir describes how a split node divides its area between its children.
type layoutDir int

const (
	layoutHoriz layoutDir = iota // children stacked top/bottom (C-x 2)
	layoutVert                   // children side by side (C-x 3)
)

// layoutNode is a node in the window split tree.
// Leaf nodes (win != nil) hold a single window.
// Internal nodes (win == nil) represent a split between two sub-trees.
type layoutNode struct {
	win      *window.Window // non-nil for leaf nodes
	dir      layoutDir      // split direction for internal nodes
	children [2]*layoutNode // non-nil for internal nodes
}

func leafNode(w *window.Window) *layoutNode {
	return &layoutNode{win: w}
}

// splitLeaf finds the leaf holding win, replaces it with an internal node that
// splits win (child 0) and newWin (child 1) along dir.
// Returns the (possibly new) root node.
func (n *layoutNode) splitLeaf(win, newWin *window.Window, dir layoutDir) *layoutNode {
	if n.win == win {
		return &layoutNode{
			dir:      dir,
			children: [2]*layoutNode{leafNode(win), leafNode(newWin)},
		}
	}
	if n.win != nil {
		return n // this leaf doesn't match
	}
	// Internal node: recurse into children.
	node := *n
	node.children[0] = n.children[0].splitLeaf(win, newWin, dir)
	node.children[1] = n.children[1].splitLeaf(win, newWin, dir)
	return &node
}

// removeLeaf removes the leaf holding win from the tree by replacing the
// internal node that contains it with that node's other child.
// If the root itself is the target leaf, the root is returned unchanged
// (callers must handle the single-window case before calling this).
func (n *layoutNode) removeLeaf(win *window.Window) *layoutNode {
	if n.win != nil {
		return n // leaf nodes can't self-remove; caller handles this
	}
	for i := range 2 {
		if n.children[i].win == win {
			return n.children[1-i] // return the sibling
		}
	}
	// Recurse into children.
	node := *n
	node.children[0] = n.children[0].removeLeaf(win)
	node.children[1] = n.children[1].removeLeaf(win)
	return &node
}

// applyLayout assigns screen regions to all leaf windows in the tree,
// recursively subdividing the given rectangle.
func (n *layoutNode) applyLayout(top, left, width, height int) {
	if n.win != nil {
		n.win.SetRegion(top, left, width, height)
		return
	}
	switch n.dir {
	case layoutHoriz:
		halfH := height / 2
		n.children[0].applyLayout(top, left, width, halfH)
		n.children[1].applyLayout(top+halfH, left, width, height-halfH)
	case layoutVert:
		// Leave 1 column for the │ separator between the panes.
		halfW := (width - 1) / 2
		n.children[0].applyLayout(top, left, halfW, height)
		n.children[1].applyLayout(top, left+halfW+1, width-halfW-1, height)
	}
}
