package editor

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

func TestBufferWordCompletions_basic(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello world helo help")
	items := bufferWordCompletions(buf, "hel")
	labels := make(map[string]bool)
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels["hello"] {
		t.Error("expected 'hello' in completions")
	}
	if !labels["helo"] {
		t.Error("expected 'helo' in completions")
	}
	if !labels["help"] {
		t.Error("expected 'help' in completions")
	}
	// 'world' does not start with 'hel'
	if labels["world"] {
		t.Error("unexpected 'world' in completions")
	}
}

func TestBufferWordCompletions_caseInsensitive(t *testing.T) {
	buf := buffer.NewWithContent("test", "Beautiful beautiful BEAUTIFUL beau")
	items := bufferWordCompletions(buf, "bea")
	labels := make(map[string]bool)
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels["Beautiful"] {
		t.Error("expected 'Beautiful'")
	}
	if !labels["beautiful"] {
		t.Error("expected 'beautiful'")
	}
	if !labels["BEAUTIFUL"] {
		t.Error("expected 'BEAUTIFUL'")
	}
	// "beau" is a word in the buffer that starts with "bea", but is the same
	// length as the minimum check may or may not exclude it depending on the
	// point position; here it should appear because it's longer than the prefix
	if !labels["beau"] {
		t.Error("expected 'beau'")
	}
}

func TestBufferWordCompletions_noDuplicates(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello hello hello")
	items := bufferWordCompletions(buf, "hel")
	if len(items) != 1 {
		t.Errorf("expected 1 unique item, got %d", len(items))
	}
}

func TestBufferWordCompletions_prefixNotReturned(t *testing.T) {
	// Words equal in length to the prefix are excluded (no meaningful expansion).
	buf := buffer.NewWithContent("test", "foo foobar")
	items := bufferWordCompletions(buf, "foo")
	for _, it := range items {
		if it.Label == "foo" {
			t.Error("prefix word 'foo' should not appear as completion")
		}
	}
	found := false
	for _, it := range items {
		if it.Label == "foobar" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'foobar' in completions")
	}
}

func TestBufferWordCompletions_emptyPrefix(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello world")
	items := bufferWordCompletions(buf, "")
	if items != nil {
		t.Error("expected nil items for empty prefix")
	}
}

func TestLspCompWordPrefix_empty(t *testing.T) {
	buf := buffer.NewWithContent("test", "")
	prefix, start := lspCompWordPrefix(buf)
	if prefix != "" {
		t.Errorf("expected empty prefix, got %q", prefix)
	}
	if start != 0 {
		t.Errorf("expected start=0, got %d", start)
	}
}

func TestLspCompWordPrefix_atWord(t *testing.T) {
	buf := buffer.NewWithContent("test", "os.Stdout")
	// Point is after 'Stdout' (position 9)
	buf.SetPoint(9)
	prefix, start := lspCompWordPrefix(buf)
	if prefix != "Stdout" {
		t.Errorf("expected 'Stdout', got %q", prefix)
	}
	if start != 3 {
		t.Errorf("expected start=3, got %d", start)
	}
}
