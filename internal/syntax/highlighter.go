package syntax

// Face describes text appearance
type Face struct {
	Fg             string // color name: "default", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright-red", etc.
	Bg             string
	Bold           bool
	Italic         bool
	Underline      bool
	UnderlineColor string // color for the underline (separate from Fg); empty means use Fg
	Reverse        bool
}

// DefaultFace is the zero-value face: no colors, no attributes.
var DefaultFace = Face{}

// Predefined faces for syntax highlighting
var (
	FaceDefault    = Face{Fg: "default", Bg: "default"}
	FaceKeyword    = Face{Fg: "blue", Bold: true}
	FaceString     = Face{Fg: "green"}
	FaceComment    = Face{Fg: "bright-black", Italic: true} // bright-black = dark gray
	FaceType       = Face{Fg: "cyan"}
	FaceFunction   = Face{Fg: "yellow"}
	FaceNumber     = Face{Fg: "magenta"}
	FaceOperator   = Face{Fg: "white"}
	FaceHeader1    = Face{Fg: "bright-blue", Bold: true}
	FaceHeader2    = Face{Fg: "blue", Bold: true}
	FaceHeader3    = Face{Fg: "cyan", Bold: true}
	FaceBold       = Face{Bold: true}
	FaceItalic     = Face{Italic: true}
	FaceCode       = Face{Fg: "yellow"}
	FaceLink       = Face{Fg: "cyan", Underline: true}
	FaceBlockquote = Face{Fg: "green", Italic: true}
	FaceModeline   = Face{Fg: "black", Bg: "white", Bold: true}
	FaceMinibuffer = Face{Fg: "default", Bg: "default"}
	FaceRegion     = Face{Fg: "black", Bg: "cyan"}
	FaceIsearch    = Face{Fg: "black", Bg: "yellow"}
	// FaceCandidate is used for normal completion candidates.
	FaceCandidate = Face{Fg: "default", Bg: "default"}
	// FaceSelected is used for the highlighted completion candidate.
	FaceSelected = Face{Reverse: true}
)

// Span is a highlighted range in the buffer
type Span struct {
	Start int // byte offset (rune index) in buffer
	End   int
	Face  Face
}

// Highlighter produces syntax highlight spans for a text region
type Highlighter interface {
	// Highlight returns spans for text[start:end]
	// start and end are rune offsets into text
	// Returned spans have Start/End relative to the full text string
	Highlight(text string, start, end int) []Span
}

// NilHighlighter returns no spans (for fundamental mode)
type NilHighlighter struct{}

func (n NilHighlighter) Highlight(text string, start, end int) []Span { return nil }
