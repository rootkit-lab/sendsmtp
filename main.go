package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wiz/sendsmtp/internal/engine"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	configPath := "app.config.yml"
	if v := os.Getenv("SENDSMTP_CONFIG"); v != "" {
		configPath = v
	}

	emit := func(name string, data any) {
		if app := application.Get(); app != nil {
			app.Event.Emit(name, data)
		}
	}

	eng, err := engine.New(configPath, emit)
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	// Ensure working dir has data folder for relative paths
	_ = os.MkdirAll(filepath.Dir(eng.Cfg.Database), 0o755)

	svc := NewAppService(eng)

	app := application.New(application.Options{
		Name:        "SendSMTP",
		Description: "SMTP bulk email sender",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "SendSMTP",
		Width:            1280,
		Height:           800,
		BackgroundColour: application.NewRGB(250, 247, 242),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
