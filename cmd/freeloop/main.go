// Command freeloop is the Wails desktop app: a window over the in-process
// engine.
package main

import (
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/frontend"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
)

// EventName is the Wails event every persisted session event is emitted on.
const EventName = "freeloop:event"

func init() {
	application.RegisterEvent[event.Event](EventName)
}

func main() {
	dataDir, err := config.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	eng, err := engine.Open(engine.Options{DataDir: dataDir, Secrets: darwin.Keychain{}})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	app := application.New(application.Options{
		Name:        "Freeloop",
		Description: "Run AI coding agents with your own models, harnesses, skills and environments",
		Services: []application.Service{
			application.NewService(&EngineService{eng: eng}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets()),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

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

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
