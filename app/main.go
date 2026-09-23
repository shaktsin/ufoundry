// Command UFoundry is the macOS app: a Wails v3 shell around the Svelte UI in
// frontend/. The UI talks to the engine directly over the engine's WebSocket;
// this shell starts the engine, puts UFoundry in the menu bar, and turns
// approval requests into native notifications.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed icons/tray-template.png
var trayTemplateIcon []byte

//go:embed icons/tray.png
var trayIcon []byte

// Version is set at build time with -ldflags "-X main.Version=…".
var Version = "0.4.0-dev"

const bundleID = "com.ufoundry.app"

func main() {
	logger := newLogger()
	eng, err := NewEngineManager(logger)
	if err != nil {
		log.Fatal(err)
	}

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	notifier := notifications.New()
	shell := &Shell{Engine: eng, Log: logger}

	app := application.New(application.Options{
		Name:        "UFoundry",
		Description: "Your personal AI agent",
		Logger:      logger,
		Services:    []application.Service{application.NewService(notifier)},
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(dist),
			Middleware:     shell.Middleware,
			DisableLogging: true,
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
			// Closing the window leaves UFoundry in the menu bar.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: bundleID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				shell.ShowWindow()
			},
		},
		OnShutdown: func() { eng.Shutdown() },
	})
	shell.App = app

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "UFoundry",
		Width:            1180,
		Height:           780,
		MinWidth:         760,
		MinHeight:        520,
		URL:              "/",
		BackgroundColour: application.NewRGB(9, 9, 11),
		Hidden:           startHidden(),
	})
	shell.Window = win
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		shell.ShowWindow()
	})

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayTemplateIcon)
	} else {
		tray.SetIcon(trayIcon)
	}
	shell.Tray = tray
	shell.buildTrayMenu()

	watcher := NewWatcher(eng, logger, shell, notifier)
	shell.Watcher = watcher

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() {
			if err := eng.Ensure(ctx); err != nil {
				logger.Error("engine start", "err", err)
				shell.SetStatus(StatusOffline, err.Error())
			}
			watcher.Run(ctx)
		}()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// startHidden is true when macOS opened the app as a login item: it then
// lives in the menu bar until the user opens the window.
func startHidden() bool {
	for _, a := range os.Args[1:] {
		if a == "--hidden" || a == "--background" {
			return true
		}
	}
	return os.Getenv("UFOUNDRY_START_HIDDEN") == "1"
}

func newLogger() *slog.Logger {
	home, err := appHome()
	if err == nil {
		dir := filepath.Join(home, "logs")
		if os.MkdirAll(dir, 0o700) == nil {
			if f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
			}
		}
	}
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
