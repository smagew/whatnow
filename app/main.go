package main

// whatnow: the dashboard in its own window (system WebView through Wails), or as a plain server for a browser.
//
//	whatnow           open the window; the pages are also served on 127.0.0.1:<port> if the port is free
//	whatnow serve [port]  only the server on 127.0.0.1:<port>, no window (what autostart runs)
//	whatnow render | inbox | hook | hooks | autostart   see usage in cli.go

import (
	"context"
	"fmt"
	"os"

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
	err := wails.Run(&options.App{
		Title:            "whatnow",
		Width:            1280,
		Height:           860,
		MinWidth:         420,
		MinHeight:        360,
		BackgroundColour: &options.RGBA{R: 238, G: 241, B: 238, A: 255}, // the page background (--bg),
		AssetServer:      &assetserver.Options{Handler: handler{local: true}},
		OnShutdown:       func(context.Context) { termCloseAll() }, // attach processes end, the sessions go on
		OnStartup: func(ctx context.Context) {
			pickDir = func() (string, error) {
				return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: "Choose the folder of the new work", CanCreateDirectories: true})
			}
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
