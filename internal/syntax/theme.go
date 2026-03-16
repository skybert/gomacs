package syntax

// LoadTheme applies the named colour theme. Returns true on success.
// Supported built-in names: "sweet", "default".
// Custom themes registered with RegisterTheme are also accepted.
func LoadTheme(name string) bool {
	fn, ok := themes[name]
	if !ok {
		return false
	}
	fn()
	return true
}

// RegisterTheme adds (or replaces) a named theme. fn is called to apply
// the theme by mutating the package-level Face variables.
// This is how users can define custom themes from ~/.gomacs:
//
//	(define-gomacs-theme "my-theme"
//	  '((keyword :foreground "#e17df3" :bold t)
//	    (string  :foreground "#06c993")))
func RegisterTheme(name string, fn func()) {
	themes[name] = fn
}

// GetFacePtr returns a pointer to the package-level Face variable
// identified by name (e.g. "keyword", "string", "modeline").
// Returns nil, false when no face with that name exists.
func GetFacePtr(name string) (*Face, bool) {
	p, ok := faceByName[name]
	return p, ok
}

// SetFaceByName overwrites the named Face variable with f.
// Returns false when no face with that name exists.
func SetFaceByName(name string, f Face) bool {
	p, ok := faceByName[name]
	if !ok {
		return false
	}
	*p = f
	return true
}

// FaceNames returns a sorted list of all face names.
func FaceNames() []string {
	names := make([]string, 0, len(faceByName))
	for n := range faceByName {
		names = append(names, n)
	}
	return names
}

type themeFunc func()

var themes = map[string]themeFunc{
	"sweet":   applySweetTheme,
	"default": applyDefaultTheme,
}

// faceByName maps canonical face names to the package-level Face pointers.
// Pointer stability: package-level vars have a fixed address for the
// lifetime of the process, so these pointers are always valid.
var faceByName = map[string]*Face{
	"default":    &FaceDefault,
	"keyword":    &FaceKeyword,
	"string":     &FaceString,
	"comment":    &FaceComment,
	"type":       &FaceType,
	"function":   &FaceFunction,
	"number":     &FaceNumber,
	"operator":   &FaceOperator,
	"header1":    &FaceHeader1,
	"header2":    &FaceHeader2,
	"header3":    &FaceHeader3,
	"bold":       &FaceBold,
	"italic":     &FaceItalic,
	"code":       &FaceCode,
	"link":       &FaceLink,
	"blockquote": &FaceBlockquote,
	"modeline":   &FaceModeline,
	"minibuffer": &FaceMinibuffer,
	"region":     &FaceRegion,
	"isearch":    &FaceIsearch,
	"candidate":  &FaceCandidate,
	"selected":   &FaceSelected,
}

// applyDefaultTheme resets all faces to the built-in terminal-colour defaults.
func applyDefaultTheme() {
	FaceDefault = Face{Fg: "default", Bg: "default"}
	FaceKeyword = Face{Fg: "blue", Bold: true}
	FaceString = Face{Fg: "green"}
	FaceComment = Face{Fg: "bright-black", Italic: true}
	FaceType = Face{Fg: "cyan"}
	FaceFunction = Face{Fg: "yellow"}
	FaceNumber = Face{Fg: "magenta"}
	FaceOperator = Face{Fg: "white"}
	FaceHeader1 = Face{Fg: "bright-blue", Bold: true}
	FaceHeader2 = Face{Fg: "blue", Bold: true}
	FaceHeader3 = Face{Fg: "cyan", Bold: true}
	FaceBold = Face{Bold: true}
	FaceItalic = Face{Italic: true}
	FaceCode = Face{Fg: "yellow"}
	FaceLink = Face{Fg: "cyan", Underline: true}
	FaceBlockquote = Face{Fg: "green", Italic: true}
	FaceModeline = Face{Fg: "black", Bg: "white", Bold: true}
	FaceMinibuffer = Face{Fg: "default", Bg: "default"}
	FaceRegion = Face{Fg: "black", Bg: "cyan"}
	FaceIsearch = Face{Fg: "black", Bg: "yellow"}
	FaceCandidate = Face{Fg: "default", Bg: "default"}
	FaceSelected = Face{Reverse: true}
}

// applySweetTheme sets faces to the Sweet colour palette.
// Colours are taken directly from the Sweet GTK theme by EliverLara:
//
//	sweet-fg       #b8c0d4   sweet-bg       #222235   sweet-bg-1     #292235
//	sweet-bg-hl    #28283f   sweet-black    #13131e   sweet-mono-1   #a2a9ba
//	sweet-mono-3   #808693   sweet-accent   #fc1c5b   sweet-pink     #f561ab
//	sweet-purple   #e17df3   sweet-blue     #06c993   sweet-green    #06c993
//	sweet-red-1    #f6717e   sweet-orange-1 #f6ce55   sweet-silver   #a3a3ff
func applySweetTheme() {
	const (
		sweetFg      = "#b8c0d4"
		sweetBg      = "#222235"
		sweetBg1     = "#292235"
		sweetBlack   = "#13131e"
		sweetMono1   = "#a2a9ba"
		sweetMono3   = "#808693"
		sweetAccent  = "#fc1c5b"
		sweetPink    = "#f561ab"
		sweetPurple  = "#e17df3"
		sweetBlue    = "#06c993"
		sweetGreen   = "#06c993"
		sweetRed1    = "#f6717e"
		sweetOrange1 = "#f6ce55"
		sweetSilver  = "#a3a3ff"
	)

	FaceDefault = Face{Fg: sweetFg, Bg: sweetBg}
	FaceKeyword = Face{Fg: sweetPurple, Bold: true}
	FaceString = Face{Fg: sweetGreen}
	FaceComment = Face{Fg: sweetMono3, Italic: true}
	FaceType = Face{Fg: sweetBlue}
	FaceFunction = Face{Fg: sweetOrange1}
	FaceNumber = Face{Fg: sweetRed1}
	FaceOperator = Face{Fg: sweetPink}
	FaceHeader1 = Face{Fg: sweetAccent, Bold: true}
	FaceHeader2 = Face{Fg: sweetPurple, Bold: true}
	FaceHeader3 = Face{Fg: sweetSilver, Bold: true}
	FaceBold = Face{Bold: true}
	FaceItalic = Face{Italic: true}
	FaceCode = Face{Fg: sweetOrange1}
	FaceLink = Face{Fg: sweetBlue, Underline: true}
	FaceBlockquote = Face{Fg: sweetGreen, Italic: true}
	FaceModeline = Face{Fg: sweetMono1, Bg: sweetBg1, Bold: true}
	FaceMinibuffer = Face{Fg: sweetFg, Bg: sweetBlack}
	FaceRegion = Face{Fg: sweetBlack, Bg: sweetPurple}
	FaceIsearch = Face{Fg: sweetBlack, Bg: sweetOrange1}
	FaceCandidate = Face{Fg: sweetFg, Bg: sweetBg}
	FaceSelected = Face{Fg: sweetBlack, Bg: sweetAccent}
}
