// Command freeloop is the Wails desktop app: a window over the in-process
// engine.
package main

import (
	"context"
	"errors"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/FinanceNik/Freeloop/frontend"
	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/rpc"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// EventName is the Wails event every persisted session event is emitted on.
const EventName = "freeloop:event"

// ConfigEventName is emitted after config files edited on disk were
// reloaded (SET-001).
const ConfigEventName = "freeloop:config"

// OpenSessionEventName asks the UI to open a session, after a notification
// for it was clicked (GUI-008).
const OpenSessionEventName = "freeloop:open-session"

// OpenHelpEventName asks the UI to show the help, from the Help menu.
const OpenHelpEventName = "freeloop:open-help"

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
	application.RegisterEvent[string](OpenHelpEventName)
	application.RegisterEvent[[]string](FilesDroppedEventName)
}

// windowPermissions lets the page open the microphone for dictation (GUI-050)
// without the web view asking on top of the system, which still asks once. The
// page is the app's own, and model output never reaches it as markup.
var windowPermissions = map[application.PermissionType]application.Permission{
	application.PermissionMicrophone: application.PermissionAllow,
}

func main() {
	// Inside a sandbox, before anything that needs the data folder (OQ-037).
	if len(os.Args) > 1 && os.Args[1] == environment.SandboxBridgeArg {
		os.Exit(environment.RunSandboxBridge(os.Args[2:]))
	}
	dataDir, err := config.DataDir(devEnv(config.HomeEnv))
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "optimizer-hook" {
		runOptimizerHook(os.Args[2:], os.Stdin, os.Stdout)
		return
	}
	// With a command, the binary is the command line for a running app
	// (EXT-004).
	if len(os.Args) > 1 && isCommand(os.Args[1]) {
		os.Exit(runCLI(config.SocketPath(dataDir), os.Args[1:], os.Stdout, os.Stderr))
	}
	logs := startLog(dataDir)
	if names := ignoredDevSwitches(shipped == "1", os.Environ()); len(names) > 0 {
		log.Printf("freeloop: this build ignores the development switches %s", strings.Join(names, ", "))
	}
	lock, err := environment.LockFolder(filepath.Join(dataDir, ".lock"))
	if errors.Is(err, environment.ErrLocked) {
		failStartup("Freeloop is already running with this data folder, and two copies would overwrite each other's work. Switch to the one that is open.")
	} else if err != nil {
		failStartup("Freeloop could not open its data folder: " + err.Error())
	}
	defer lock.Close()
	// FREELOOP_DEV=1 (set by `make dev`; never read by a shipped build) offers the
	// scripted demo provider.
	// Reloads can happen before the app exists; they wait here.
	configChanges := make(chan engine.ConfigChange, 16)
	notices := make(chan engine.Notice, 64)
	hookProgram, err := os.Executable()
	if err != nil {
		log.Printf("freeloop: external agents get no token optimizer: %v", err)
	}
	eng, err := engine.Open(engine.Options{
		HookProgram: hookProgram,
		DataDir:     dataDir, Secrets: secretStore(), Confiner: thisPlatform.Confiner, ProcessStarted: thisPlatform.ProcessStarted, GroupMembers: thisPlatform.GroupMembers, EditorArgv: thisPlatform.EditorArgv, PDFArgv: thisPlatform.PDFArgv, FolderArgv: thisPlatform.FolderArgv, SystemInfo: thisPlatform.SystemInfo, Dev: devEnv("FREELOOP_DEV") == "1", ScriptedVoice: devEnv("FREELOOP_DEV_VOICE") == "scripted",
		// A fake release server for trying the update check (OQ-015).
		ReleaseAPI: devEnv("FREELOOP_RELEASE_API"),
		// A stand-in for the GitHub CLI (SES-011, SES-017, SES-018).
		GH: devEnv("FREELOOP_DEV_GH"),
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
		failStartup("Freeloop could not open its data. " + startupHint(err) + "\n\n" + err.Error())
	}
	logs.redact.Store(eng.Redact)
	// On a Mac, quitting ends the process inside app.Run, so nothing deferred here
	// runs: this has to, or a running command outlives the app and goes on changing
	// files while its session shows as idle.
	var once sync.Once
	var closers []func()
	guard := &quitGuard{eng: eng}
	shutdown := func() {
		once.Do(func() {
			done := make(chan struct{})
			go func() {
				for _, c := range closers {
					c()
				}
				eng.Close()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(shutdownLimit):
				log.Printf("freeloop: quitting after %s with work still stopping", shutdownLimit)
			}
		})
	}
	defer shutdown()
	// The headless API (EXT-004); the app runs without it when the socket
	// cannot be served, as when another copy has the data folder.
	if srv, err := rpc.Listen(eng, config.SocketPath(dataDir)); err != nil {
		log.Printf("freeloop: headless API: %v", err)
	} else {
		go srv.Serve()
		closers = append(closers, func() { srv.Close() })
	}

	svc := &EngineService{eng: eng}
	desk := newDesktop(eng)
	app := application.New(application.Options{
		Name:        "Freeloop",
		Description: "Run AI coding agents with your own models, harnesses, skills and environments",
		Services:    append([]application.Service{application.NewService(svc)}, desk.services()...),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets()),
			// Engine calls come from the app's own page, never from a page
			// in the preview frame (GUI-020).
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/wails/runtime" && !netguard.RuntimeCallAllowed(r.Method, r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site"), r.Host, r.Header.Get("x-wails-client-id"), servedBuild) {
						log.Printf("refused a runtime call: %s from origin %q to %q", r.Method, r.Header.Get("Origin"), r.Host)
						http.Error(w, "refused", http.StatusForbidden)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: shutdown,
		ShouldQuit: func() bool { return guard.shouldQuit() },
	})
	guard.app = app

	svc.app, svc.desk, svc.logPath = app, desk, config.LogPath(dataDir)
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
		Permissions:    windowPermissions,
	})
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(ev *application.WindowEvent) {
		app.Event.Emit(FilesDroppedEventName, ev.Context().DroppedFiles())
	})

	desk.start(app, win)
	guard.watch(win)

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

// secretStore is one keyring item holding every secret (the login keychain on a Mac, the
// Secret Service on Linux, the Credential Manager on Windows), so a rebuild asks once. A served build, for checking the UI on scratch
// data, can keep what it stores in memory with FREELOOP_DEV_SECRETS=memory, so a
// scratch folder leaves nothing in the keyring and asks it nothing.
func secretStore() secrets.Store {
	if servedBuild && devEnv("FREELOOP_DEV_SECRETS") == "memory" {
		return secrets.NewMemory()
	}
	return secrets.NewVault(thisPlatform.Keys, "freeloop")
}

// shutdownLimit is how long quitting waits for sessions to stop. Each stops its
// commands first, so this is rarely reached; a quit must still always finish.
const shutdownLimit = 8 * time.Second

// logOutput is the app's log: to a file on this computer, with what the app knows to be
// secret removed. Errors from providers and tools can echo a key.
type logOutput struct {
	w      io.Writer
	redact atomic.Value // func(string) string
}

func (l *logOutput) Write(b []byte) (int, error) {
	line := string(b)
	if fn, ok := l.redact.Load().(func(string) string); ok {
		line = fn(line)
	}
	if _, err := io.WriteString(l.w, line); err != nil {
		return 0, err
	}
	return len(b), nil
}

// startLog sends the log to its file, and a crash's stack trace with it. A
// development build also keeps its terminal.
func startLog(dataDir string) *logOutput {
	out := &logOutput{w: os.Stderr}
	f, err := config.OpenLog(dataDir)
	if err != nil {
		log.Printf("freeloop: no log file: %v", err)
		log.SetOutput(out)
		return out
	}
	logFile := config.NewLogFile(dataDir, f)
	if devEnv("FREELOOP_DEV") == "1" {
		out.w = io.MultiWriter(os.Stderr, logFile)
	} else if err := environment.RedirectStderr(f); err != nil {
		out.w = logFile
	} else {
		// Crash traces follow the log to its next file.
		logFile.OnRotate = func(next *os.File) {
			if err := environment.RedirectStderr(next); err != nil {
				log.Printf("freeloop: crash traces stay in the old log: %v", err)
			}
		}
		out.w = logFile
	}
	log.SetOutput(out)
	return out
}

// startupHint says what the user can do about a data folder that will not open.
func startupHint(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "newer") || strings.Contains(msg, "version"):
		return "It was last used by a newer version of Freeloop; install that version. Nothing was changed."
	case strings.Contains(msg, "disk i/o") || strings.Contains(msg, "no space") || strings.Contains(msg, "disk is full"):
		return "The disk may be full. Free up some space and open it again. Nothing was changed."
	case strings.Contains(msg, "read-only") || strings.Contains(msg, "permission denied") || strings.Contains(msg, "not permitted"):
		return "The folder cannot be written to. Check its permissions, or that its disk is not read-only."
	}
	return "Nothing was changed."
}

// failStartup says why the app cannot start and ends it. Launched from Finder, the
// Start menu or a desktop launcher, its standard error goes nowhere, and the app would just seem not to open.
func failStartup(message string) {
	log.Print("freeloop: " + message)
	if servedBuild {
		os.Exit(1)
	}
	app := application.New(application.Options{Name: "Freeloop"})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() {
			dialog := app.Dialog.Error()
			dialog.SetTitle("Freeloop cannot start")
			dialog.SetMessage(message)
			dialog.Show()
			app.Quit()
		}()
	})
	app.Run()
	os.Exit(1)
}
