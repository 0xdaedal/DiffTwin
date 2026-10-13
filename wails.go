//go:build !ci

package main

import (
	"context"
	"embed"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

func prepareWailsLocale() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	// Wails formats GTK CSS decimals using the process locale; GTK requires a dot decimal separator.
	return os.Setenv("LC_NUMERIC", "C")
}

func runWails(projectStore *store) error {
	backend := NewApp(projectStore)
	return wails.Run(&options.App{
		Title:     "DiffTwin",
		Width:     1440,
		Height:    940,
		MinWidth:  1050,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: frontendAssets,
		},
		OnStartup: backend.startup,
		Bind:      []interface{}{backend},
	})
}

func (a *App) startup(context.Context) {
}
