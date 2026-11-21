package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// versionInstalador: versión deste instalador (non a de File Browser, Tao ou Lúa, que
// van embebidos coas súas propias versións).
//
//go:embed VERSION
var versionInstalador string

func main() {
	// En Windows non hai nada que elevar aquí: o manifesto leva requireAdministrator e
	// UAC eleva o proceso antes de main(). Se se cancela o UAC, Windows nin o lanza.
	asegurarPrivilexios() // Debian: relanza con pkexec se non somos root
	app := NewApp()

	wailsApp := application.New(application.Options{
		Name:        "Instalador de aula",
		Description: "Instalador de File Browser Quantum, Tao e Lúa",
		Services:    []application.Service{application.NewService(app)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Icon:        appIcon,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Piztu — Instalador de aula",
		Width:            1024,
		Height:           760,
		StartState:       application.WindowStateMaximised,
		BackgroundColour: application.NewRGB(245, 245, 244),
		URL:              "/",
	})

	if err := wailsApp.Run(); err != nil {
		println("Error:", err.Error())
	}
}
