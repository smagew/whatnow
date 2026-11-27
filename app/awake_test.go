package main

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// fakeHold counts the holds of the system: held now, started in all.
func fakeHold(t *testing.T) (held, started *int32, last *time.Duration) {
	held, started, last = new(int32), new(int32), new(time.Duration)
	old := holdAwakeFn
	holdAwakeFn = func(d time.Duration, _ bool) (func(), error) {
		atomic.AddInt32(held, 1)
		atomic.AddInt32(started, 1)
		*last = d
		return func() { atomic.AddInt32(held, -1) }, nil
	}
	t.Cleanup(func() { _ = awakeApply(0, false); holdAwakeFn = old })
	return
}

func TestStayAwakeForATime(t *testing.T) {
	isolate(t)
	if !awakeCan() {
		t.Skip("this system cannot be kept awake")
	}
	held, _, last := fakeHold(t)
	code, obj := setAwake(120, true)
	if code != 200 {
		t.Fatalf("set: %d %v", code, obj)
	}
	until := num(awakeNow()["until"])
	if d := time.Until(time.UnixMilli(until)); d < 119*time.Minute || d > 121*time.Minute {
		t.Fatalf("until is %v from now, want 2 h", d)
	}
	if atomic.LoadInt32(held) != 1 || *last < 119*time.Minute {
		t.Fatalf("held %d for %v", atomic.LoadInt32(held), *last)
	}
	if num(readJSONMap(awakeFile())["until"]) != until {
		t.Fatal("the time is not kept on disk")
	}
	// another time replaces the hold, it does not add one
	setAwake(60, true)
	if atomic.LoadInt32(held) != 1 {
		t.Fatalf("held %d after a new time", atomic.LoadInt32(held))
	}
	setAwake(0, true)
	if atomic.LoadInt32(held) != 0 || num(awakeNow()["until"]) != 0 {
		t.Fatalf("off: held %d, until %v", atomic.LoadInt32(held), awakeNow()["until"])
	}
	if _, err := os.Stat(awakeFile()); !os.IsNotExist(err) {
		t.Fatal("the kept time stays after off")
	}
}

func TestStayAwakeUntilOff(t *testing.T) {
	isolate(t)
	if !awakeCan() {
		t.Skip("this system cannot be kept awake")
	}
	held, _, last := fakeHold(t)
	if code, _ := setAwake(-1, true); code != 200 || num(awakeNow()["until"]) != -1 || *last != 0 || atomic.LoadInt32(held) != 1 {
		t.Fatalf("until off: %d, until %v, d %v, held %d", code, awakeNow()["until"], *last, atomic.LoadInt32(held))
	}
	for _, bad := range []int64{-2, awakeMaxMin + 1} {
		if code, _ := setAwake(bad, true); code != 400 {
			t.Fatalf("%d minutes: %d", bad, code)
		}
	}
	if atomic.LoadInt32(held) != 1 {
		t.Fatal("a bad time ended the hold")
	}
}

// the time runs out: the hold ends by itself and the kept time goes
func TestStayAwakeEnds(t *testing.T) {
	isolate(t)
	if !awakeCan() {
		t.Skip("this system cannot be kept awake")
	}
	held, _, _ := fakeHold(t)
	must(t, atomicWriteJSON(awakeFile(), map[string]any{"until": 1}))
	must(t, awakeApply(time.Now().Add(150*time.Millisecond).UnixMilli(), true))
	if atomic.LoadInt32(held) != 1 {
		t.Fatal("not held")
	}
	time.Sleep(400 * time.Millisecond)
	if atomic.LoadInt32(held) != 0 || num(awakeNow()["until"]) != 0 {
		t.Fatalf("after the time: held %d, until %v", atomic.LoadInt32(held), awakeNow()["until"])
	}
	if _, err := os.Stat(awakeFile()); !os.IsNotExist(err) {
		t.Fatal("the kept time stays after it ran out")
	}
}

// the app started again takes up the time that is left, and forgets one that is over
func TestStayAwakeResume(t *testing.T) {
	isolate(t)
	if !awakeCan() {
		t.Skip("this system cannot be kept awake")
	}
	held, started, _ := fakeHold(t)
	must(t, atomicWriteJSON(awakeFile(), map[string]any{"until": time.Now().Add(-time.Minute).UnixMilli()}))
	awakeResume()
	if atomic.LoadInt32(started) != 0 {
		t.Fatal("a time that is over was taken up")
	}
	if _, err := os.Stat(awakeFile()); !os.IsNotExist(err) {
		t.Fatal("a time that is over stays on disk")
	}
	u := time.Now().Add(time.Hour).UnixMilli()
	must(t, atomicWriteJSON(awakeFile(), map[string]any{"until": u}))
	awakeResume()
	if atomic.LoadInt32(held) != 1 || num(awakeNow()["until"]) != u {
		t.Fatalf("resume: held %d, until %v", atomic.LoadInt32(held), awakeNow()["until"])
	}
}

func TestStayAwakeFromThePage(t *testing.T) {
	isolate(t)
	if !awakeCan() {
		t.Skip("this system cannot be kept awake")
	}
	held, _, _ := fakeHold(t)
	code, _ := act(map[string]any{"action": "awake", "minutes": float64(480)})
	if code != 200 || atomic.LoadInt32(held) != 1 {
		t.Fatalf("act: %d, held %d", code, atomic.LoadInt32(held))
	}
}
