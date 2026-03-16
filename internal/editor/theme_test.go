package editor

import (
	"strings"
	"testing"

	"github.com/skybert/gomacs/internal/elisp"
	"github.com/skybert/gomacs/internal/syntax"
)

// newThemeEval creates an Evaluator with load-theme, set-face-attribute and
// define-gomacs-theme registered, reusing the same closures as NewEditor.
func newThemeEval() *elisp.Evaluator {
	ev := elisp.NewEvaluator()

	ev.RegisterGoFn("load-theme", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 1 {
			return nil, nil
		}
		name := strings.Trim(args[0].String(), `'"`)
		syntax.LoadTheme(name)
		return elisp.Nil{}, nil
	})

	ev.SetSetqHook("theme", func(v elisp.Value) {
		name := strings.Trim(v.String(), `'"`)
		syntax.LoadTheme(name)
	})

	ev.RegisterGoFn("set-face-attribute", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 1 {
			return nil, nil
		}
		faceName := strings.Trim(args[0].String(), `'"`)
		facePtr, ok := syntax.GetFacePtr(faceName)
		if !ok {
			return nil, nil
		}
		i := 1
		if i < len(args) && elisp.IsNil(args[i]) {
			i++
		}
		for i+1 < len(args) {
			kw := args[i].String()
			val := args[i+1]
			i += 2
			switch kw {
			case ":foreground":
				facePtr.Fg = strings.Trim(val.String(), `'"`)
			case ":background":
				facePtr.Bg = strings.Trim(val.String(), `'"`)
			case ":bold":
				facePtr.Bold = !elisp.IsNil(val)
			case ":italic":
				facePtr.Italic = !elisp.IsNil(val)
			case ":underline":
				facePtr.Underline = !elisp.IsNil(val)
			case ":reverse":
				facePtr.Reverse = !elisp.IsNil(val)
			}
		}
		return elisp.Nil{}, nil
	})

	ev.RegisterGoFn("define-gomacs-theme", func(args []elisp.Value, _ *elisp.Env) (elisp.Value, error) {
		if len(args) < 2 {
			return nil, nil
		}
		name := strings.Trim(args[0].String(), `'"`)
		faceSpecs, ok := elisp.ToSlice(args[1])
		if !ok {
			return nil, nil
		}
		type faceOverride struct {
			name string
			face syntax.Face
		}
		overrides := make([]faceOverride, 0, len(faceSpecs))
		for _, spec := range faceSpecs {
			fields, ok2 := elisp.ToSlice(spec)
			if !ok2 || len(fields) < 1 {
				continue
			}
			faceName := strings.Trim(fields[0].String(), `'"`)
			facePtr, ok2 := syntax.GetFacePtr(faceName)
			if !ok2 {
				continue
			}
			f := *facePtr
			for j := 1; j+1 < len(fields); j += 2 {
				kw := fields[j].String()
				val := fields[j+1]
				switch kw {
				case ":foreground":
					f.Fg = strings.Trim(val.String(), `'"`)
				case ":background":
					f.Bg = strings.Trim(val.String(), `'"`)
				case ":bold":
					f.Bold = !elisp.IsNil(val)
				case ":italic":
					f.Italic = !elisp.IsNil(val)
				case ":underline":
					f.Underline = !elisp.IsNil(val)
				case ":reverse":
					f.Reverse = !elisp.IsNil(val)
				}
			}
			overrides = append(overrides, faceOverride{name: faceName, face: f})
		}
		syntax.RegisterTheme(name, func() {
			for _, o := range overrides {
				syntax.SetFaceByName(o.name, o.face)
			}
		})
		return elisp.Nil{}, nil
	})

	return ev
}

func TestSetFaceAttribute_Foreground(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`(set-face-attribute 'keyword :foreground "#ff1234")`)
	if err != nil {
		t.Fatalf("set-face-attribute error: %v", err)
	}
	p, _ := syntax.GetFacePtr("keyword")
	if p.Fg != "#ff1234" {
		t.Errorf("Fg = %q, want %q", p.Fg, "#ff1234")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestSetFaceAttribute_Bold(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`(set-face-attribute 'string :bold t)`)
	if err != nil {
		t.Fatalf("set-face-attribute error: %v", err)
	}
	p, _ := syntax.GetFacePtr("string")
	if !p.Bold {
		t.Error("expected Bold=true after (set-face-attribute 'string :bold t)")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestSetFaceAttribute_NilFrame(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	// Emacs-style: nil frame argument should be ignored
	_, err := ev.EvalString(`(set-face-attribute 'comment nil :foreground "#aabbcc")`)
	if err != nil {
		t.Fatalf("set-face-attribute error: %v", err)
	}
	p, _ := syntax.GetFacePtr("comment")
	if p.Fg != "#aabbcc" {
		t.Errorf("Fg = %q, want %q", p.Fg, "#aabbcc")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestSetqThemeHook(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString("(setq theme 'default)")
	if err != nil {
		t.Fatalf("setq theme error: %v", err)
	}
	// After switching to default theme the keyword fg should be "blue".
	p, _ := syntax.GetFacePtr("keyword")
	if p.Fg != "blue" {
		t.Errorf("after (setq theme 'default): keyword Fg = %q, want %q", p.Fg, "blue")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestDefineGomacsTheme(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`
(define-gomacs-theme "test-custom"
  '((keyword :foreground "#001122" :bold nil)
    (string  :foreground "#334455")))
(load-theme "test-custom")
`)
	if err != nil {
		t.Fatalf("define-gomacs-theme error: %v", err)
	}
	kw, _ := syntax.GetFacePtr("keyword")
	if kw.Fg != "#001122" {
		t.Errorf("keyword Fg = %q, want %q", kw.Fg, "#001122")
	}
	if kw.Bold {
		t.Error("keyword Bold should be false")
	}
	str, _ := syntax.GetFacePtr("string")
	if str.Fg != "#334455" {
		t.Errorf("string Fg = %q, want %q", str.Fg, "#334455")
	}
	syntax.LoadTheme("sweet") // restore
}

func TestDefineGomacsThemeViaSetq(t *testing.T) {
	syntax.LoadTheme("sweet")
	ev := newThemeEval()
	_, err := ev.EvalString(`
(define-gomacs-theme "setq-test"
  '((modeline :foreground "#ffffff" :background "#000000")))
(setq theme "setq-test")
`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	ml, _ := syntax.GetFacePtr("modeline")
	if ml.Fg != "#ffffff" {
		t.Errorf("modeline Fg = %q, want %q", ml.Fg, "#ffffff")
	}
	if ml.Bg != "#000000" {
		t.Errorf("modeline Bg = %q, want %q", ml.Bg, "#000000")
	}
	syntax.LoadTheme("sweet") // restore
}
