//go:build !ci

package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jrcasanya/difftwin/internal/domain"
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

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) ExportProject(projectID string) (string, error) {
	project := domain.FindProject(a.store.Snapshot(), projectID)
	if project == nil {
		return "", os.ErrNotExist
	}
	if a.ctx == nil {
		return "", fmt.Errorf("el contexto de la aplicación aún no está disponible")
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Exportar proyecto",
		DefaultFilename: projectExportFilename(project.Name),
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "Configuración de DiffTwin (*.json)",
			Pattern:     "*.json",
		}},
	})
	if err != nil {
		return "", fmt.Errorf("abrir el diálogo para exportar el proyecto: %w", err)
	}
	if path == "" {
		return "", nil
	}
	if filepath.Ext(path) == "" {
		path += ".json"
	}

	data, err := json.MarshalIndent(exportProjectConfig(*project), "", "  ")
	if err != nil {
		return "", fmt.Errorf("codificar el proyecto para exportarlo: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("guardar el proyecto exportado: %w", err)
	}
	return path, nil
}
