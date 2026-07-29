package main

import (
	"embed"
	"log"

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
	app := NewApp(version)

	err := wails.Run(&options.App{
		Title:  "Raphael",
		Width:  1280,
		Height: 800,
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
