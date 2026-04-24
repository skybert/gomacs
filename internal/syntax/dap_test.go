package syntax

import "testing"

func TestDapLocalsHighlighter(t *testing.T) {
	hl := DapLocalsHighlighter{}
	tests := []struct {
		name      string
		line      string
		wantFaces map[string]Face // substring → expected face
	}{
		{
			name: "leaf variable",
			line: "  ▶ count int = 42\n",
			wantFaces: map[string]Face{
				"▶":     FaceFunction,
				"count": FaceKeyword,
				"int":   FaceType,
			},
		},
		{
			name: "nil value",
			line: "  ▼ t *testing.T = nil\n",
			wantFaces: map[string]Face{
				"t":   FaceKeyword,
				"nil": FaceKeyword,
			},
		},
		{
			name: "no highlights for plain text",
			line: "(no locals)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := hl.Highlight(tt.line, 0, len([]rune(tt.line)))
			runes := []rune(tt.line)
			for substr, want := range tt.wantFaces {
				subRunes := []rune(substr)
				found := false
				for _, sp := range spans {
					if sp.End-sp.Start == len(subRunes) {
						if string(runes[sp.Start:sp.End]) == substr {
							if sp.Face != want {
								t.Errorf("span for %q: face = %+v, want %+v", substr, sp.Face, want)
							}
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("no span found for %q in line %q", substr, tt.line)
				}
			}
		})
	}
}

func TestDapStackHighlighter(t *testing.T) {
	hl := DapStackHighlighter{}
	line := "#0  TestInsert (/home/user/buffer_test.go:42)\n"
	spans := hl.Highlight(line, 0, len([]rune(line)))
	runes := []rune(line)

	findSpan := func(substr string) *Span {
		sub := []rune(substr)
		for i := range spans {
			s := spans[i]
			if s.End-s.Start == len(sub) && string(runes[s.Start:s.End]) == substr {
				return &spans[i]
			}
		}
		return nil
	}

	if sp := findSpan("#0"); sp == nil {
		t.Error("no span for frame index '#0'")
	} else if sp.Face != FaceNumber {
		t.Errorf("#0 face = %+v, want FaceNumber", sp.Face)
	}

	if sp := findSpan("TestInsert"); sp == nil {
		t.Error("no span for function name 'TestInsert'")
	} else if sp.Face != FaceFunction {
		t.Errorf("TestInsert face = %+v, want FaceFunction", sp.Face)
	}

	if sp := findSpan("42"); sp == nil {
		t.Error("no span for line number '42'")
	} else if sp.Face != FaceKeyword {
		t.Errorf("42 face = %+v, want FaceKeyword", sp.Face)
	}
}
