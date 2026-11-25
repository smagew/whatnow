//go:build darwin && cgo

package main

// Notifications of the app itself (UNUserNotificationCenter), on macOS. Shown through `osascript` they belonged to
// Script Editor: they had its icon, and "Show" opened another program. Posted by whatnow.app they carry its name and
// icon, and a click brings the window to the front on the page of the work the notification is about.
//
// It works only inside the app bundle (a bare program has no bundle identifier, and the system refuses it), and
// only after the user has allowed notifications for whatnow once; until then the old way is used.

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework UserNotifications
#include <stdlib.h>
int wnNotifyInit(void);
int wnNotifyState(void);
int wnNotifyPost(const char *title, const char *sub, const char *body, const char *slug, int sound);
*/
import "C"

import (
	"context"
	"strings"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var appCtx context.Context // the window, set when it starts

// nativeNotifyInit is called when the window starts: it asks the system for the permission (once; the answer is
// kept by macOS) and becomes the one that hears the clicks.
func nativeNotifyInit(ctx context.Context) {
	appCtx = ctx
	C.wnNotifyInit()
}

// nativeNotify shows one notification of the app; false when it cannot (not the app, or not allowed).
func nativeNotify(title, sub, body, slug string, sound bool) bool {
	if appCtx == nil || C.wnNotifyState() != 1 {
		return false
	}
	cs := func(s string) *C.char { return C.CString(s) }
	t, su, b, sl := cs(title), cs(sub), cs(body), cs(slug)
	defer func() {
		for _, p := range []*C.char{t, su, b, sl} {
			C.free(unsafe.Pointer(p))
		}
	}()
	snd := C.int(0)
	if sound {
		snd = 1
	}
	return C.wnNotifyPost(t, su, b, sl, snd) == 1
}

// notifyClicked: the user clicked a notification (or its button). The window comes to the front, on the page of
// the work, or on the list of sessions when the notification is not about a piece of work.
func notifyClicked(slug string) {
	ctx := appCtx
	if ctx == nil {
		return
	}
	path := "/#sessions"
	if slugRe.MatchString(slug) {
		path = "/f/" + slug + "/"
	}
	go func() {
		wruntime.WindowUnminimise(ctx)
		wruntime.WindowShow(ctx)
		wruntime.Show(ctx)
		wruntime.WindowExecJS(ctx, "location.href="+strings.ReplaceAll(jsQuote(path), "</", "<\\/"))
	}()
}

func jsQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

//export notifyClickedC
func notifyClickedC(slug *C.char) { notifyClicked(C.GoString(slug)) }
