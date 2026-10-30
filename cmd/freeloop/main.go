// Command freeloop is the Wails desktop app: a window over the in-process
// engine.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/FinanceNik/Freeloop/frontend"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/platform/darwin"
	"github.com/FinanceNik/Freeloop/internal/rpc"
)

// EventName is the Wails event every persisted session event is emitted on.
const EventName = "freeloop:event"

// ConfigEventName is emitted after config files edited on disk were
// reloaded (SET-001).
const ConfigEventName = "freeloop:config"

// OpenSessionEventName asks the UI to open a session, after a notification
// for it was clicked (GUI-008).
const OpenSessionEventName = "freeloop:open-session"

// mainWindowName names the main window; a session's own window (GUI-015)
// is named by sessionWindowName and loads the page with ?session=<id>.
const mainWindowName = "main"

func sessionWindowName(sid string) string { return "session:" + sid }

// FilesDroppedEventName carries the paths dropped onto an element marked
// data-file-drop-target, such as the skills page's drop zone (SKL-005).
const FilesDroppedEventName = "freeloop:files-dropped"

// TerminalEventName carries the integrated terminals' output (GUI-012).
const TerminalEventName = "freeloop:terminal"

func init() {
	application.RegisterEvent[engine.TerminalOutput](TerminalEventName)
	application.RegisterEvent[event.Event](EventName)
	application.RegisterEvent[engine.ConfigChange](ConfigEventName)
	application.RegisterEvent[string](OpenSessionEventName)
	application.RegisterEvent[[]string](FilesDroppedEventName)
}

func main() {
	dataDir, err := config.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	// With a command, the binary is the command line for a running app
	// (EXT-004).
	if len(os.Args) > 1 && isCommand(os.Args[1]) {
		os.Exit(runCLI(config.SocketPath(dataDir), os.Args[1:], os.Stdout, os.Stderr))
	}
	// FREELOOP_DEV=1 (set by `make dev`) offers the scripted demo provider.
	// Reloads can happen before the app exists; they wait here.
	configChanges := make(chan engine.ConfigChange, 16)
	notices := make(chan engine.Notice, 64)
	eng, err := engine.Open(engine.Options{
		DataDir: dataDir, Secrets: darwin.Keychain{}, Dev: os.Getenv("FREELOOP_DEV") == "1",
		OnConfigChange: func(c engine.ConfigChange) {
			select {
			case configChanges <- c:
			default: // the UI refreshes everything on any change; one is enough
			}
		},
		OnNotice: func(n engine.Notice) {
			select {
			case notices <- n:
			default: // a burst of notices; the attention view has them all
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()
	// The headless API (EXT-004); the app runs without it when the socket
	// cannot be served, as when another copy has the data folder.
	if srv, err := rpc.Listen(eng, config.SocketPath(dataDir)); err != nil {
		log.Printf("freeloop: headless API: %v", err)
	} else {
		go srv.Serve()
		defer srv.Close()
	}

	svc := &EngineService{eng: eng}
	desk := newDesktop(eng)
	app := application.New(application.Options{
		Name:        "Freeloop",
		Description: "Run AI coding agents with your own models, harnesses, skills and environments",
		Services:    append([]application.Service{application.NewService(svc)}, desk.services()...),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets()),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	svc.app, svc.desk = app, desk
	// MCP sign-ins open in the user's browser (MCP-006).
	eng.SetOpenURL(func(u string) error { return app.Browser.OpenURL(u) })

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      mainWindowName,
		Title:     "Freeloop",
		Width:     1200,
		Height:    780,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
		// Only elements marked data-file-drop-target accept drops.
		EnableFileDrop: true,
	})
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(ev *application.WindowEvent) {
		app.Event.Emit(FilesDroppedEventName, ev.Context().DroppedFiles())
	})

	desk.start(app, win)

	// Bridge: every event the engine persists goes to the UI. Events that
	// change what needs attention also refresh the dock badge, once per
	// burst.
	stream, unsubscribe := eng.Subscribe()
	defer unsubscribe()
	badge := make(chan struct{}, 1)
	// Sessions left in error show on the badge from launch.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		select {
		case badge <- struct{}{}:
		default:
		}
	})
	// The terminals' output goes to the UI apart from the events: it is
	// never recorded.
	termOut, unsubscribeTerms := eng.SubscribeTerminals()
	defer unsubscribeTerms()
	go func() {
		for out := range termOut {
			app.Event.Emit(TerminalEventName, out)
		}
	}()
	go func() {
		for ev := range stream {
			app.Event.Emit(EventName, ev)
			switch ev.Type {
			case event.PermissionRequested, event.PermissionDecided, event.SessionStatus:
				select {
				case badge <- struct{}{}:
				default:
				}
			}
		}
	}()
	go func() {
		for range badge {
			time.Sleep(200 * time.Millisecond)
			desk.badge(eng.AttentionCount(context.Background()))
		}
	}()
	go func() {
		for n := range notices {
			desk.notice(n)
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
