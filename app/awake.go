package main

// Stay awake: the computer does not go to sleep by itself while the user is away and the sessions work, for a time
// the user picks (or until it is turned off). It is what a video player does while it plays: the system is asked not
// to sleep when nobody touches it. The screen may still go dark. A laptop with its lid closed sleeps anyway (macOS on
// battery always does), and the user can still put it to sleep.
//
// The hold belongs to this process: it ends when the program ends. The time is kept in ~/.whatnow/awake.json, so that
// the app started again takes it up until the time is over.

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

const awakeMaxMin = 24 * 60

// holdAwakeFn is holdAwake of the system; the tests count the holds instead.
var holdAwakeFn = holdAwake

var stayAwake struct {
	mu    sync.Mutex
	until int64 // ms since 1970; -1: until turned off; 0: off
	stop  func()
	timer *time.Timer
	gen   int
}

func awakeFile() string { return filepath.Join(home(), "awake.json") }

// awakeNow is what the pages show: {"until": ms | -1 | 0}; nil where this system cannot keep itself awake.
func awakeNow() map[string]any {
	if !awakeCan() {
		return nil
	}
	stayAwake.mu.Lock()
	defer stayAwake.mu.Unlock()
	return map[string]any{"until": stayAwake.until}
}

// setAwake keeps the computer awake for the given minutes: 0 turns it off, -1 is until it is turned off.
func setAwake(minutes int64) (int, any) {
	if !awakeCan() {
		return 501, map[string]any{"error": "this system cannot be kept awake from here"}
	}
	if minutes < -1 || minutes > awakeMaxMin {
		return 400, map[string]any{"error": "bad time"}
	}
	var until int64
	switch {
	case minutes == -1:
		until = -1
	case minutes > 0:
		until = time.Now().Add(time.Duration(minutes) * time.Minute).UnixMilli()
	}
	if err := awakeApply(until); err != nil {
		return 500, map[string]any{"error": "could not keep the computer awake: " + err.Error()}
	}
	if until == 0 {
		_ = os.Remove(awakeFile())
	} else {
		_ = atomicWriteJSON(awakeFile(), map[string]any{"until": until})
	}
	return 200, map[string]any{"ok": true, "until": until}
}

// awakeApply ends the hold there is and starts the one asked for.
func awakeApply(until int64) error {
	stayAwake.mu.Lock()
	defer stayAwake.mu.Unlock()
	stayAwake.gen++
	if stayAwake.timer != nil {
		stayAwake.timer.Stop()
		stayAwake.timer = nil
	}
	if stayAwake.stop != nil {
		stayAwake.stop()
		stayAwake.stop = nil
	}
	stayAwake.until = 0
	if until == 0 {
		return nil
	}
	var d time.Duration
	if until > 0 {
		if d = time.Until(time.UnixMilli(until)); d <= 0 {
			return nil
		}
	}
	stop, err := holdAwakeFn(d)
	if err != nil {
		return err
	}
	var once sync.Once
	stayAwake.stop = func() { once.Do(stop) }
	stayAwake.until = until
	if d > 0 {
		g := stayAwake.gen
		stayAwake.timer = time.AfterFunc(d, func() {
			stayAwake.mu.Lock()
			defer stayAwake.mu.Unlock()
			if stayAwake.gen != g {
				return
			}
			if stayAwake.stop != nil {
				stayAwake.stop()
				stayAwake.stop = nil
			}
			stayAwake.until, stayAwake.timer = 0, nil
			_ = os.Remove(awakeFile())
		})
	}
	return nil
}

// awakeResume takes up the time kept by the program that ran before, at the start of the server.
func awakeResume() {
	until := num(readJSONMap(awakeFile())["until"])
	if until == 0 || !awakeCan() {
		return
	}
	if until > 0 && until <= time.Now().UnixMilli() {
		_ = os.Remove(awakeFile())
		return
	}
	_ = awakeApply(until)
}

func readJSONMap(path string) map[string]any {
	m, _ := readJSON(path).(map[string]any)
	return m
}
