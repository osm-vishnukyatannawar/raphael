package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

// appIcon is the window and taskbar icon on Linux. Windows picks up
// build/windows/icon.ico at package time instead, and macOS uses the bundle.
//
//go:embed build/appicon.png
var appIcon []byte

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// Answered before any window is opened, so the installer can ask an
	// already-installed binary what it is without starting a desktop session.
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version)

		return
	}

	app := NewApp(version)

	err := wails.Run(&options.App{
		Title: "Raphael",
		// The dashboard puts both task lists side by side, which needs the width.
		// WindowStartState is what actually decides the launch size on a 1920
		// display; Width/Height are the size restoring from maximised gives, and
		// the fallback where the compositor ignores the start state.
		Width:            1600,
		Height:           1000,
		MinWidth:         900,
		MinHeight:        600,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Linux: &linux.Options{
			Icon:        appIcon,
			ProgramName: "Raphael",
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []any{
			app,
		},
	})
	if err != nil {
		log.Fatalf("wails: %v", err)
	}
}
