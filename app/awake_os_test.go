//go:build darwin

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// the real hold of macOS: the system lists an assertion against sleep on behalf of this process, and it is gone
// when the hold stops
func TestStayAwakeMacOS(t *testing.T) {
	onBehalf := "on behalf of Process ID " + strconv.Itoa(os.Getpid())
	listed := func() bool {
		out, _ := exec.Command("pmset", "-g", "assertions").Output()
		return strings.Contains(string(out), onBehalf)
	}
	stop, err := holdAwake(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for i := 0; i < 30 && !ok; i++ {
		time.Sleep(100 * time.Millisecond)
		ok = listed()
	}
	stop()
	if !ok {
		t.Fatal("the system lists no assertion of this process")
	}
	for i := 0; i < 30 && listed(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if listed() {
		t.Fatal("the assertion stays after the stop")
	}
}
