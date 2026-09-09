// Command nexus-mail is a local-first, privacy-focused desktop mail client.
package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"nexusmail/internal/app"
	"nexusmail/internal/auth"
	"nexusmail/internal/logging"
	"nexusmail/internal/paths"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	debug := flag.Bool("debug", false, "log at debug level")
	flag.Parse()

	if err := run(*debug); err != nil {
		log.Fatal(err)
	}
}

func run(debug bool) error {
	dataDir, err := paths.DataDir()
	if err != nil {
		return err
	}
	logDir, err := paths.LogDir()
	if err != nil {
		return err
	}

	logger, closeLogs, err := logging.Setup(logDir, debug)
	if err != nil {
		return err
	}
	defer func() { _ = closeLogs() }()

	db, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("closing the database", "err", err)
		}
	}()

	secrets, err := auth.Default(dataDir)
	if err != nil {
		return err
	}

	cfg, err := app.LoadConfig(dataDir)
	if err != nil {
		return err
	}
	cfg.LogDir = paths.LogDir

	// The Wails application is needed to emit events, and the service is
	// needed to build the application. The closure breaks the cycle: nothing
	// emits until Run starts, by which point wailsApp is set.
	var wailsApp *application.App
	cfg.Emit = func(name string, data any) {
		if wailsApp != nil {
			wailsApp.Event.Emit(name, data)
		}
	}

	engine := imapsync.New(db, app.DialerFor(db, secrets, cfg))
	service := app.NewMailService(db, secrets, engine, cfg)
	bodies := app.NewBodyHandler(service)

	wailsApp = application.New(application.Options{
		Name:        "Nexus Mail",
		Description: "Local-first, privacy-focused mail client",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: mailContentMiddleware(bodies),
		},
		Mac: application.MacOptions{
			// Closing the window quits. Running in the tray is a cluster of
			// platform-specific behaviour — tray icon, unread badge, native
			// notifications, launch-at-login — that belongs after live sync
			// works, not tangled up with it.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Nexus Mail",
		Width:  1280,
		Height: 800,
		// A mail client is dense by nature; below this the three columns stop
		// being usable.
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	})

	logger.Info("starting", "data_dir", dataDir, "debug", debug)

	if err := wailsApp.Run(); err != nil {
		logger.Error("application exited with an error", "err", err)
		return err
	}
	return nil
}

// mailContentMiddleware routes message bodies and proxied images to the body
// handler, leaving everything else to the embedded frontend.
//
// Serving bodies here rather than over the Wails bridge keeps an
// eight-megabyte newsletter out of an IPC message, and lets the response carry
// a real Content-Security-Policy header — which srcdoc cannot.
func mailContentMiddleware(bodies http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/mail-body/") ||
				strings.HasPrefix(r.URL.Path, "/mail-asset/") {
				bodies.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
