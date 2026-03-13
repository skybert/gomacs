package window

import (
	"testing"

	"github.com/skybert/gomacs/internal/buffer"
)

// ---- Buf / SetBuf ----------------------------------------------------------

func TestBufReturnsBuffer(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 24)
	if w.Buf() != buf {
		t.Error("Buf() returned unexpected buffer")
	}
}

func TestSetBufSwapsBuffer(t *testing.T) {
	b1 := buffer.NewWithContent("test1", "hello")
	b2 := buffer.NewWithContent("test2", "world")
	w := New(b1, 0, 0, 80, 24)
	w.SetBuf(b2)
	if w.Buf() != b2 {
		t.Error("SetBuf: Buf() should return the new buffer")
	}
	if w.ScrollLine() != 1 {
		t.Errorf("SetBuf: scrollLine should reset to 1, got %d", w.ScrollLine())
	}
}

// ---- Top / Left / Width ----------------------------------------------------

func TestTopLeftWidth(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 5, 10, 40, 20)
	if w.Top() != 5 {
		t.Errorf("Top() = %d, want 5", w.Top())
	}
	if w.Left() != 10 {
		t.Errorf("Left() = %d, want 10", w.Left())
	}
	if w.Width() != 40 {
		t.Errorf("Width() = %d, want 40", w.Width())
	}
}

// ---- SetRegion -------------------------------------------------------------

func TestSetRegion(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 0, 0, 80, 24)
	w.SetRegion(3, 5, 30, 10)
	if w.Top() != 3 {
		t.Errorf("Top() = %d, want 3", w.Top())
	}
	if w.Left() != 5 {
		t.Errorf("Left() = %d, want 5", w.Left())
	}
	if w.Width() != 30 {
		t.Errorf("Width() = %d, want 30", w.Width())
	}
	if w.Height() != 10 {
		t.Errorf("Height() = %d, want 10", w.Height())
	}
}

// ---- GoalCol / SetGoalCol / ClearGoalCol -----------------------------------

func TestGoalColDefaultNegativeOne(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 0, 0, 80, 24)
	if w.GoalCol() != -1 {
		t.Errorf("GoalCol() default = %d, want -1", w.GoalCol())
	}
}

func TestSetAndClearGoalCol(t *testing.T) {
	buf := buffer.New("test")
	w := New(buf, 0, 0, 80, 24)
	w.SetGoalCol(42)
	if w.GoalCol() != 42 {
		t.Errorf("GoalCol() = %d, want 42", w.GoalCol())
	}
	w.ClearGoalCol()
	if w.GoalCol() != -1 {
		t.Errorf("GoalCol() after Clear = %d, want -1", w.GoalCol())
	}
}

// ---- SetPoint clamping -----------------------------------------------------

func TestSetPointClampedToZero(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 24)
	w.SetPoint(-5)
	if w.Point() != 0 {
		t.Errorf("SetPoint(-5): want 0, got %d", w.Point())
	}
}

func TestSetPointClampedToLen(t *testing.T) {
	buf := buffer.NewWithContent("test", "hello")
	w := New(buf, 0, 0, 80, 24)
	w.SetPoint(999)
	if w.Point() != buf.Len() {
		t.Errorf("SetPoint(999): want %d, got %d", buf.Len(), w.Point())
	}
}

// ---- ScrollUp / ScrollDown -------------------------------------------------

func TestScrollUp(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(1)
	w.ScrollUp(2)
	if w.ScrollLine() != 3 {
		t.Errorf("ScrollUp(2): want scrollLine=3, got %d", w.ScrollLine())
	}
}

func TestScrollDown(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(4)
	w.ScrollDown(2)
	if w.ScrollLine() != 2 {
		t.Errorf("ScrollDown(2): want scrollLine=2, got %d", w.ScrollLine())
	}
}

func TestScrollUpBeyondMax(t *testing.T) {
	w, _ := newFiveLineWindow(3) // 5-line buffer
	w.ScrollUp(999)
	if w.ScrollLine() != 5 {
		t.Errorf("ScrollUp beyond max: want scrollLine=5, got %d", w.ScrollLine())
	}
}

func TestScrollDownBelowMin(t *testing.T) {
	w, _ := newFiveLineWindow(3)
	w.SetScrollLine(2)
	w.ScrollDown(999)
	if w.ScrollLine() != 1 {
		t.Errorf("ScrollDown below min: want scrollLine=1, got %d", w.ScrollLine())
	}
}
