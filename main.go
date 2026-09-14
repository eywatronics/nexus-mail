// Command nexus-mail is a local-first, privacy-focused desktop mail client.
package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"nexusmail/internal/app"
	"nexusmail/internal/auth"
	"nexusmail/internal/logging"
	"nexusmail/internal/paths"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

//go:embed all:frontend/dist
var assets embed.FS

// The tray icon is the application icon. A separate monochrome glyph would sit
// better in the Windows tray, but shipping the wrong shape is worse than
// shipping the right one at the wrong weight.
//
//go:embed build/appicon.png
var trayIcon []byte

// quitting distinguishes the user asking to quit from the user closing the
// window. Without it the closing hook would cancel the close that Quit itself
// performs, and the application could never exit.
var quitting atomic.Bool

// showWindow brings the window back from the tray.
//
// Show alone is not enough on Windows: a window hidden while minimised comes
// back minimised, which looks to the user exactly like nothing happening.
func showWindow(window application.Window) {
	window.Show()
	window.Restore()
	window.Focus()
}

// refreshTrayTooltip puts the unread count where it can be seen with the
// window closed.
func refreshTrayTooltip(tray *application.SystemTray, service *app.MailService, logger *slog.Logger) {
	unread, err := service.UnreadCount()
	if err != nil {
		logger.Error("counting unread mail for the tray", "err", err)
		return
	}
	tray.SetTooltip(app.TrayTooltip(unread))
}

// syncEverything runs a sync for every account, which is what the tray's
// "Sync now" means. Errors are already reported as events; this only keeps
// one failing account from stopping the others.
func syncEverything(service *app.MailService, logger *slog.Logger) {
	accounts, err := service.ListAccounts()
	if err != nil {
		logger.Error("listing accounts for a manual sync", "err", err)
		return
	}
	for _, acct := range accounts {
		if err := service.SyncAccount(acct.ID); err != nil {
			logger.Error("manual sync failed", "account_id", acct.ID, "err", err)
		}
	}
}

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
	cfg.AttachmentDir = paths.AttachmentDir

	// The Wails application is needed to emit events, and the service is
	// needed to build the application. The closure breaks the cycle: nothing
	// emits until Run starts, by which point wailsApp is set.
	var wailsApp *application.App
	cfg.Emit = func(name string, data any) {
		if wailsApp != nil {
			wailsApp.Event.Emit(name, data)
		}
	}

	// The same cycle again, and the same way out of it: the body handler is
	// built from the service, and the service has to be able to drop entries
	// from the handler's cache when an encoding repair rewrites a body. Config
	// is passed by value, so the closure has to exist before the service does
	// — assigning to cfg afterwards would write to a copy nothing reads.
	var bodies *app.BodyHandler
	cfg.InvalidateBody = func(messageID int64) {
		if bodies != nil {
			bodies.Invalidate(messageID)
		}
	}

	engine := imapsync.New(db, app.DialerFor(db, secrets, cfg))
	engine.SetRetention(cfg.Retention)
	service := app.NewMailService(db, secrets, engine, cfg)
	bodies = app.NewBodyHandler(service)

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
			// The window is hidden rather than closed, so this would not fire
			// in normal use; false is the honest value for an app that lives
			// in the tray, and it keeps the two platforms behaving alike.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
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

	// Closing the window hides it; the app keeps running.
	//
	// This is what makes live sync mean anything. A mail client that stops
	// syncing when its window is closed only knows about mail that arrived
	// while somebody was looking at it, which is the opposite of why IDLE
	// exists. The watchers hang off a context that lives until Run returns, so
	// keeping the process alive is the whole mechanism — nothing else has to
	// know the window is gone.
	//
	// Quitting is still possible, from the tray menu. A window that could not
	// be closed and an app that could not be quit would be worse than either.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if quitting.Load() {
			return
		}
		e.Cancel()
		window.Hide()
	})

	tray := wailsApp.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip(app.TrayTooltip(0))
	tray.OnClick(func() { showWindow(window) })
	tray.OnDoubleClick(func() { showWindow(window) })

	trayMenu := application.NewMenu()
	trayMenu.Add("Open Nexus Mail").OnClick(func(*application.Context) { showWindow(window) })
	trayMenu.Add("Sync now").OnClick(func(*application.Context) {
		go syncEverything(service, logger)
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(*application.Context) {
		// Marked before the quit so the closing hook lets the window go
		// instead of hiding it and leaving a process with no way out.
		quitting.Store(true)
		wailsApp.Quit()
	})
	tray.SetMenu(trayMenu)

	// The tooltip is refreshed from the same event the window redraws on, so
	// the tray and the list never disagree about how much is unread.
	wailsApp.Event.On(app.EventSyncFinished, func(*application.CustomEvent) {
		refreshTrayTooltip(tray, service, logger)
	})
	refreshTrayTooltip(tray, service, logger)

	// Live sync starts before the window opens and stops when Run returns.
	// Everything the IDLE loop does hangs off this context, so closing the
	// window closes every IMAP connection with it.
	watchCtx, stopWatching := context.WithCancel(context.Background())
	defer stopWatching()

	if err := service.StartWatching(watchCtx); err != nil {
		// Not fatal: the window is still useful with what is already on disk,
		// and a manual sync still works. Live updates are what is lost.
		logger.Error("live sync could not be started", "err", err)
	}

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
				strings.HasPrefix(r.URL.Path, "/mail-asset/") ||
				strings.HasPrefix(r.URL.Path, "/mail-source/") {
				bodies.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
