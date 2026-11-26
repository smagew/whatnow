package main

// whatnow: the dashboard in its own window (system WebView through Wails), or as a plain server for a browser.
//
//	whatnow           open the window; the pages are also served on 127.0.0.1:<port> if the port is free
//	whatnow serve [port]  only the server on 127.0.0.1:<port>, no window (what autostart runs)
//	whatnow window <work>  the page of one work in a window of its own (see window.go)
//	whatnow render | inbox | hook | hooks | autostart   see usage in cli.go

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"whatnow"
)

var webFS = whatnow.Web

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			p := port()
			if len(os.Args) > 2 { // a port of its own, e.g. next to a running one
				if _, err := fmt.Sscan(os.Args[2], &p); err != nil {
					fmt.Fprintln(os.Stderr, "whatnow: bad port", os.Args[2])
					os.Exit(2)
				}
			}
			fmt.Printf("whatnow: http://127.0.0.1:%d/\n", p)
			if err := listen(p); err != nil {
				fmt.Fprintln(os.Stderr, "whatnow:", err)
				os.Exit(1)
			}
			return
		case "window":
			if len(os.Args) < 3 || !winSlug(os.Args[2]) {
				fmt.Fprintln(os.Stderr, "whatnow: window <work>")
				os.Exit(2)
			}
			soloSlug = os.Args[2]
			runWindow()
			return
		case "hook":
			os.Exit(runHook(os.Args[2:]))
		case "render":
			os.Exit(runRender(os.Args[2:]))
		case "inbox":
			os.Exit(runInbox(os.Args[2:]))
		case "hooks":
			os.Exit(runHooks(os.Args[2:]))
		case "autostart":
			os.Exit(runAutostart(os.Args[2:]))
		case "-h", "--help", "help":
			fmt.Println(usage)
			return
		default:
			fmt.Fprintln(os.Stderr, "whatnow: unknown command", os.Args[1])
			os.Exit(2)
		}
	}
	// A browser tab keeps working while the window is open; when a server already holds the port, it stays.
	go func() { _ = listen(port()) }()
	runWindow()
}

// runWindow: the window of the app. The main one, or (soloSlug set) the window of one work.
func runWindow() {
	title, w, h := "whatnow", 1280, 860
	var place []int
	if soloSlug != "" {
		title = workTitle(soloSlug)
		if place = placeOf(soloSlug); place != nil {
			w, h = place[2], place[3]
		}
	}
	var watch sync.Once
	dw, dh := 0, 0 // what the window adds to the size it was given (its title bar): the size is kept without it
	err := wails.Run(&options.App{
		Title:            title,
		Width:            w,
		Height:           h,
		MinWidth:         420,
		MinHeight:        360,
		BackgroundColour: &options.RGBA{R: 238, G: 241, B: 238, A: 255}, // the page background (--bg),
		AssetServer:      &assetserver.Options{Handler: handler{local: true}},
		OnShutdown:       func(context.Context) { termCloseAll(); winGone() }, // attach processes end, the sessions go on
		OnStartup: func(ctx context.Context) {
			winCtx = ctx
			if soloSlug == "" {
				nativeNotifyInit(ctx)
			} else if place != nil {
				runtime.WindowSetPosition(ctx, place[0], place[1])
			}
			pickDir = func() (string, error) {
				return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: "Choose the folder of the new work", CanCreateDirectories: true})
			}
		},
		// the other windows are listened to once the page is there: a request to show a work needs the page
		OnDomReady: func(ctx context.Context) {
			watch.Do(func() {
				if gw, gh := runtime.WindowGetSize(ctx); soloSlug != "" && gw >= w && gh >= h {
					dw, dh = gw-w, gh-h
				}
				go winWatch(ctx)
			})
		},
		OnBeforeClose: func(ctx context.Context) bool {
			if soloSlug != "" {
				x, y := runtime.WindowGetPosition(ctx)
				ww, hh := runtime.WindowGetSize(ctx)
				// a window put back where it was reads a pixel or two off (rounding): not moved, the place stays,
				// so that it does not creep a little on every opening
				if place != nil && x-place[0] >= -2 && x-place[0] <= 2 && y-place[1] >= -2 && y-place[1] <= 2 {
					x, y = place[0], place[1]
				}
				placeSave(soloSlug, x, y, ww-dw, hh-dh)
			}
			return false
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{Title: "whatnow", Message: "Claude Code sessions and features at a glance"},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "whatnow:", err)
		os.Exit(1)
	}
}
