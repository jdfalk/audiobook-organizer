// file: pkg/plugin/sdk/progress_test.go
// version: 1.0.0
// guid: 9d4e1f2a-3b5c-4d6e-8f7a-1b2c3d4e5f60
// last-edited: 2026-05-30

package sdk

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

type recordedFrame struct {
	current int
	total   int
	message string
}

type fakeReporter struct {
	frames []recordedFrame
}

func (f *fakeReporter) UpdateProgress(current, total int, message string) error {
	f.frames = append(f.frames, recordedFrame{current, total, message})
	return nil
}

func (f *fakeReporter) Log(slog.Level, string, ...slog.Attr) error { return nil }
func (f *fakeReporter) Logger() *slog.Logger                       { return slog.Default() }
func (f *fakeReporter) Checkpoint(any) error                       { return nil }
func (f *fakeReporter) IsCanceled() bool                           { return false }
func (f *fakeReporter) RunPhase(_ context.Context, _ string, fn func(context.Context, Reporter) error) error {
	return fn(context.Background(), f)
}
func (f *fakeReporter) Trigger(context.Context, string, any) error { return nil }
func (f *fakeReporter) SetCurrentItem(string)                      {}

func TestProgress_ZeroItemsStillAdvances(t *testing.T) {
	r := &fakeReporter{}
	p := NewProgress(r, 0)
	p.Start("starting")
	p.Finalize("finalizing")
	p.Done("done")

	if got, want := len(r.frames), 3; got != want {
		t.Fatalf("frames: got %d want %d", got, want)
	}
	if r.frames[0].total != 2 {
		t.Fatalf("total: got %d want 2 (n+2 with n=0)", r.frames[0].total)
	}
	if r.frames[0].current != 0 || r.frames[1].current != 1 || r.frames[2].current != 2 {
		t.Fatalf("current progression wrong: %+v", r.frames)
	}
	for _, f := range r.frames {
		if f.total == 0 {
			t.Fatalf("total must never be 0 (would render 0/0)")
		}
	}
}

func TestProgress_StepIncrements(t *testing.T) {
	r := &fakeReporter{}
	p := NewProgress(r, 3)
	p.Start("start")
	p.Step("a")
	p.Step("b")
	p.Step("c")
	p.Finalize("finalize")
	p.Done("done")

	wantCurrents := []int{0, 1, 2, 3, 4, 5}
	for i, f := range r.frames {
		if f.current != wantCurrents[i] {
			t.Errorf("frame %d current: got %d want %d", i, f.current, wantCurrents[i])
		}
		if f.total != 5 {
			t.Errorf("frame %d total: got %d want 5", i, f.total)
		}
	}
}

func TestProgress_StepNClamps(t *testing.T) {
	r := &fakeReporter{}
	p := NewProgress(r, 10)
	p.StepN(15, "way past end") // should clamp to 10
	if r.frames[0].current != 10 {
		t.Errorf("StepN clamp: got %d want 10", r.frames[0].current)
	}
	p.StepN(-3, "negative") // should clamp to 0
	if r.frames[1].current != 0 {
		t.Errorf("StepN negative clamp: got %d want 0", r.frames[1].current)
	}
}

func TestProgress_LargeNUsesTwoDecimals(t *testing.T) {
	r := &fakeReporter{}
	p := NewProgress(r, 308857)
	p.StepN(1088, "Clearing fingerprints 1088/308857 (cleared=1088)")
	msg := r.frames[0].message
	if !strings.Contains(msg, "%") {
		t.Fatalf("expected percent in message, got %q", msg)
	}
	if !strings.Contains(msg, ".") {
		t.Errorf("expected two-decimal pct for large N, got %q", msg)
	}
}

func TestProgress_SmallNUsesZeroDecimals(t *testing.T) {
	r := &fakeReporter{}
	p := NewProgress(r, 5)
	p.StepN(2, "step 2/5")
	msg := r.frames[0].message
	if strings.Contains(msg, ".") {
		t.Errorf("expected zero-decimal pct for small N, got %q", msg)
	}
}

func TestProgress_NilSafe(t *testing.T) {
	var p *Progress
	// Should not panic on any method.
	p.Start("x")
	p.Step("x")
	p.StepN(1, "x")
	p.Finalize("x")
	p.Done("x")

	r2 := &fakeReporter{}
	p2 := &Progress{r: nil, n: 1, total: 3}
	_ = r2
	p2.Start("x") // r nil — should no-op without panic
}

func TestProgress_NegativeNClampedToZero(t *testing.T) {
	r := &fakeReporter{}
	p := NewProgress(r, -10)
	if p.Total() != 2 {
		t.Errorf("negative n should clamp to 0 (total=2), got total=%d", p.Total())
	}
	p.Start("s")
	p.Done("d")
	if r.frames[len(r.frames)-1].total != 2 {
		t.Fatalf("expected wire total 2, got %d", r.frames[len(r.frames)-1].total)
	}
}
