package main

import (
	"context"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/version"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application context.
// Its methods are exported to the frontend via Wails's JS bridge.
type App struct {
	ctx  context.Context
	port int
}

func newApp(port int) *App { return &App{port: port} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetVersion returns the dsc binary version string for display in the UI.
func (a *App) GetVersion() string {
	return version.String()
}

// GetPort returns the gateway port so the frontend can construct API URLs
// when running outside the embedded serve path (e.g., Wails dev mode).
func (a *App) GetPort() int {
	return a.port
}

// OpenFileDialog opens the native OS file picker and returns the selected path.
// The frontend calls this via window.go.App.OpenFileDialog().
//
// OpenFileDialog cannot be unit-tested without a live Wails context; tested
// manually via wails dev.
func (a *App) OpenFileDialog() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open file",
		Filters: []runtime.FileFilter{
			{DisplayName: "All Files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	return path, nil
}
