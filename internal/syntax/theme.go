package syntax

// LoadTheme applies the named colour theme. Returns true on success.
// Supported names: "sweet", "default".
func LoadTheme(name string) bool {
	fn, ok := themes[name]
	if !ok {
		return false
	}
	fn()
	return true
}

type themeFunc func()

var themes = map[string]themeFunc{
	"sweet":   applySweetTheme,
	"default": applyDefaultTheme,
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

// applySweetTheme sets faces inspired by the Sweet GTK theme by EliverLara.
// https://github.com/EliverLara/Sweet
func applySweetTheme() {
	FaceDefault = Face{Fg: "#ebebeb", Bg: "default"}
	FaceKeyword = Face{Fg: "#aa46be", Bold: true}   // purple
	FaceString = Face{Fg: "#4caf50"}                // green
	FaceComment = Face{Fg: "#546e7a", Italic: true} // slate blue-grey
	FaceType = Face{Fg: "#26a69a"}                  // teal
	FaceFunction = Face{Fg: "#fdd835"}              // bright yellow
	FaceNumber = Face{Fg: "#ff7043"}                // deep orange
	FaceOperator = Face{Fg: "#f06292"}              // pink
	FaceHeader1 = Face{Fg: "#8b52fe", Bold: true}   // violet
	FaceHeader2 = Face{Fg: "#aa46be", Bold: true}   // purple
	FaceHeader3 = Face{Fg: "#5c6bc0", Bold: true}   // indigo
	FaceBold = Face{Bold: true}
	FaceItalic = Face{Italic: true}
	FaceCode = Face{Fg: "#fdd835"}                     // yellow
	FaceLink = Face{Fg: "#26a69a", Underline: true}    // teal
	FaceBlockquote = Face{Fg: "#4caf50", Italic: true} // green
	FaceModeline = Face{Fg: "#ebebeb", Bg: "#252539", Bold: true}
	FaceMinibuffer = Face{Fg: "#ebebeb", Bg: "#1e1d2f"}
	FaceRegion = Face{Fg: "#1e1d2f", Bg: "#8b52fe"}  // violet selection
	FaceIsearch = Face{Fg: "#1e1d2f", Bg: "#fdd835"} // yellow search
	FaceCandidate = Face{Fg: "#ebebeb", Bg: "#1e1d2f"}
	FaceSelected = Face{Fg: "#1e1d2f", Bg: "#aa46be"} // purple highlight
}
