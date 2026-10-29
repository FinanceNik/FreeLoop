// Command freeloop is the Wails desktop app: a window over the in-process
// engine.
package main

import (
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/frontend"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
)

// EventName is the Wails event every persisted session event is emitted on.
const EventName = "freeloop:event"

// ConfigEventName is emitted after config files edited on disk were
// reloaded (SET-001).
const ConfigEventName = "freeloop:config"

func init() {
	application.RegisterEvent[event.Event](EventName)
	application.RegisterEvent[engine.ConfigChange](ConfigEventName)
}

func main() {
	dataDir, err := config.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	// FREELOOP_DEV=1 (set by `make dev`) offers the scripted demo provider.
	// Reloads can happen before the app exists; they wait here.
	configChanges := make(chan engine.ConfigChange, 16)
	eng, err := engine.Open(engine.Options{
		DataDir: dataDir, Secrets: darwin.Keychain{}, Dev: os.Getenv("FREELOOP_DEV") == "1",
		OnConfigChange: func(c engine.ConfigChange) {
			select {
			case configChanges <- c:
			default: // the UI refreshes everything on any change; one is enough
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	svc := &EngineService{eng: eng}
	app := application.New(application.Options{
		Name:        "Freeloop",
		Description: "Run AI coding agents with your own models, harnesses, skills and environments",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets()),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	svc.app = app

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Freeloop",
		Width:     1200,
		Height:    780,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
	})

	// Bridge: every event the engine persists goes to the UI.
	events, unsubscribe := eng.Subscribe()
	defer unsubscribe()
	go func() {
		for ev := range events {
			app.Event.Emit(EventName, ev)
		}
	}()
	go func() {
		for c := range configChanges {
			app.Event.Emit(ConfigEventName, c)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
