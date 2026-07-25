package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Agent Workspace",
		Width:            1180,
		Height:           760,
		MinWidth:         820,
		MinHeight:        560,
		BackgroundColour: &options.RGBA{R: 15, G: 18, B: 24, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Linux:            &linux.Options{ProgramName: "agent-workspace"},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
