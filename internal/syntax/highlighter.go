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
	// FaceCompletionBorder is the face for the thin border around the
	// inline completion popup. Muted so it doesn't overwhelm the content.
	FaceCompletionBorder = Face{Fg: "bright-black"}
	// FaceCompilationOK is used for the *compilation* buffer name on the
	// modeline when the last build succeeded.
	FaceCompilationOK = Face{Fg: "green", Bold: true}
	// FaceCompilationFail is used for the *compilation* buffer name on the
	// modeline when the last build failed.
	FaceCompilationFail = Face{Fg: "red", Bold: true}

	// FaceBreakpoint is the face for breakpoint indicators (●) in the debug gutter.
	FaceBreakpoint = Face{Fg: "#00c800", Bold: true}
	// FaceExecPos is the face for the current execution position (→) in the debug gutter.
	FaceExecPos = Face{Fg: "yellow", Bold: true}
	// FaceWindowJump is the face for the one-letter window-jump badges (M-o).
	// The spec asks for the letter to be green.
	FaceWindowJump = Face{Fg: "black", Bg: "green", Bold: true}
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

// RuneHighlighter is an optional fast path on top of Highlighter.  The renderer
// already keeps the buffer's runes in a reusable scratch slice, so a highlighter
// that scans runes can be handed that slice directly instead of a string it
// would immediately decode again — which removes one full-buffer encode and one
// full-buffer decode per keystroke.
//
// HighlightRunes(runes, start, end) must return exactly what
// Highlight(string(runes), start, end) returns.  Implementations must neither
// retain nor modify runes: the caller reuses it across frames.
type RuneHighlighter interface {
	Highlighter
	HighlightRunes(runes []rune, start, end int) []Span
}

// ScanState is the multi-line scanner state a Resumable highlighter needs in
// order to continue a scan part-way through a buffer instead of restarting at
// offset 0.
//
// One concrete struct serves every highlighter rather than a per-highlighter
// interface: the span cache stores one state per checkpoint, and a plain struct
// copies without allocating.  Only the fields a highlighter documents for
// itself are meaningful to it; the others stay zero.
type ScanState struct {
	// Pos is the rune offset the scan resumes at.  Highlighters only ever
	// report offsets that are safe to restart from — a token boundary for the
	// token scanners, a line start for the line-oriented ones — so no span ever
	// straddles a reported Pos.
	Pos int
	// Fence is true while the scan sits inside a construct that a single line
	// cannot close: a Markdown fenced code block, a Gherkin docstring.
	Fence bool
}

// Checkpoints collects restart points while a Resumable highlighter scans.
// The caller supplies the reporting policy; the highlighter only says where a
// restart is safe.
type Checkpoints struct {
	// Every is the minimum distance, in runes, between reported restart points.
	// Zero (or a nil Report) reports nothing, which turns HighlightResume into a
	// plain bounded scan.
	Every int
	// Report receives each restart point: the state there, and the number of
	// spans the scan has emitted so far.  That count is an index into the slice
	// HighlightResume goes on to return, so the caller can drop everything from
	// a checkpoint onwards and keep the spans before it.
	Report func(st ScanState, nspans int)

	// next is the offset at which the next point may be reported.
	next int
}

// arm prepares c for a scan resuming at from.  Points become due at the first
// safe offset at or after each multiple of Every, so a resumed scan chooses the
// same offsets a scan from 0 would have — checkpoints do not drift as the
// resume position walks forward.
func (c *Checkpoints) arm(from int) {
	if c == nil || c.Every <= 0 {
		return
	}
	c.next = (from/c.Every + 1) * c.Every
}

// mark records a restart point at st.Pos if one is due there.  nspans is the
// number of spans emitted for offsets below st.Pos.
func (c *Checkpoints) mark(st ScanState, nspans int) {
	if c == nil || c.Report == nil || c.Every <= 0 || st.Pos < c.next {
		return
	}
	c.Report(st, nspans)
	c.next = (st.Pos/c.Every + 1) * c.Every
}

// Resumable is implemented by highlighters whose scan can restart from a
// checkpoint instead of from offset 0.  A highlighter whose multi-line state
// does not fit in a ScanState — or that emits spans out of order, or needs a
// pre-pass over the whole range — simply does not implement this, and callers
// fall back to scanning from the top of the buffer.
type Resumable interface {
	RuneHighlighter
	// HighlightResume scans from st.Pos and returns the spans that overlap
	// [st.Pos, end).  st must be either the zero ScanState, meaning "start at
	// the beginning", or a state previously reported through cp for the same
	// runes; passing anything else is a programming error and may mis-highlight.
	//
	// With the zero state and a nil cp the result must equal
	// HighlightRunes(runes, 0, end).
	HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span
}

// The highlighters that can resume.  Losing a method here would silently send
// the mode back to scanning from offset 0 on every keystroke, so the set is
// asserted at compile time rather than left to be noticed in a profile.
//
// The ones deliberately absent are the read-only views — vc-log, vc-grep,
// vc-status, compilation, help, the DAP panels and the pre-parsed ANSI spans —
// plus vc-show and vc-annotate, which cannot resume even in principle: the first
// needs a pre-pass to find where the diff begins, the second emits its source
// spans out of offset order.
var (
	_ Resumable = NilHighlighter{}
	_ Resumable = GoHighlighter{}
	_ Resumable = MarkdownHighlighter{}
	_ Resumable = PythonHighlighter{}
	_ Resumable = BashHighlighter{}
	_ Resumable = JavaHighlighter{}
	_ Resumable = ElispHighlighter{}
	_ Resumable = PerlHighlighter{}
	_ Resumable = JSONHighlighter{}
	_ Resumable = YAMLHighlighter{}
	_ Resumable = ConfHighlighter{}
	_ Resumable = MakefileHighlighter{}
	_ Resumable = GherkinHighlighter{}
	_ Resumable = DiffHighlighter{}
	_ Resumable = VcCommitHighlighter{}
)

// NilHighlighter returns no spans (for fundamental mode)
type NilHighlighter struct{}

func (n NilHighlighter) Highlight(text string, start, end int) []Span { return nil }

// HighlightRunes implements RuneHighlighter.
func (n NilHighlighter) HighlightRunes(runes []rune, start, end int) []Span { return nil }

// HighlightResume implements Resumable.  There is no state and nothing to emit,
// so every offset is trivially a restart point; none is worth reporting.
func (n NilHighlighter) HighlightResume(runes []rune, st ScanState, end int, cp *Checkpoints) []Span {
	return nil
}
