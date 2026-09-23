// Command nexus-mail is a local-first, privacy-focused desktop mail client.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

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

// refreshUnreadIndicators puts the unread count everywhere it can be seen with
// the window closed: the tray tooltip and the taskbar or dock badge.
//
// One function for both, fed by one count. Two refreshers reading the database
// separately would eventually show two different numbers on the same screen,
// and the user would have no way to tell which was right.
func refreshUnreadIndicators(tray *application.SystemTray, badge *dock.DockService,
	service *app.MailService, logger *slog.Logger) {

	unread, err := service.UnreadCount()
	if err != nil {
		logger.Error("counting unread mail for the tray", "err", err)
		return
	}
	tray.SetTooltip(app.TrayTooltip(unread))

	// An empty label means no badge rather than a badge reading zero.
	// Failures are logged and dropped: the count is already in the tooltip,
	// and a platform that cannot draw a badge is not a reason to stop.
	if label := app.BadgeLabel(unread); label == "" {
		if err := badge.RemoveBadge(); err != nil {
			logger.Debug("clearing the unread badge", "err", err)
		}
	} else if err := badge.SetBadge(label); err != nil {
		logger.Debug("setting the unread badge", "err", err)
	}
}

// notifyNewMail turns an arrival into an operating system notification.
//
// Skipped while the window is on screen and focused: a reader watching the
// list does not need to be told what they can already see, and a toast for it
// is the kind of noise that gets notifications switched off entirely.
//
// The decision about how much the notification says was already made in the
// service, which knows whether the reader allowed previews. Nothing here
// reaches back into the message.
func notifyNewMail(notifier *notifications.NotificationService, window application.Window,
	event *application.CustomEvent, logger *slog.Logger) {

	if window.IsVisible() && window.IsFocused() {
		return
	}

	arrival, ok := newMailEventFrom(event)
	if !ok {
		return
	}

	err := notifier.SendNotification(notifications.NotificationOptions{
		// One id per account, so a second arrival replaces the first rather
		// than stacking a column of toasts nobody reads.
		ID:    fmt.Sprintf("new-mail-%d", arrival.AccountID),
		Title: app.NotificationTitle(arrival),
		Body:  app.NotificationBody(arrival),
	})
	if err != nil {
		// Not fatal, and deliberately not retried: the mail is already in the
		// database and the tray tooltip already says so.
		logger.Error("could not deliver a new mail notification", "err", err)
	}
}

// newMailEventFrom recovers the payload from the event bus.
//
// Wails round-trips custom event data through JSON, so what comes back is a
// map rather than the struct that went in. Re-marshalling is the smallest way
// to get the struct back without duplicating its field names here, where they
// would drift from the ones the service actually sends.
func newMailEventFrom(event *application.CustomEvent) (app.NewMailEvent, bool) {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return app.NewMailEvent{}, false
	}
	var arrival app.NewMailEvent
	if err := json.Unmarshal(raw, &arrival); err != nil {
		return app.NewMailEvent{}, false
	}
	return arrival, true
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

	// The same cycle a third time. Starting with the machine is the Wails
	// application's business — a registry value on Windows, a launch agent on
	// macOS, a desktop file on Linux — and internal/app does not import Wails.
	// Before NewMailService, because Config is passed by value.
	cfg.Autostart = app.AutostartControl{
		Enabled: func() (bool, error) {
			if wailsApp == nil {
				return false, errors.New("main: the application is not running yet")
			}
			return wailsApp.Autostart.IsEnabled()
		},
		Set: func(enabled bool) error {
			if wailsApp == nil {
				return errors.New("main: the application is not running yet")
			}
			if enabled {
				return wailsApp.Autostart.Enable()
			}
			return wailsApp.Autostart.Disable()
		},
	}

	// Registered as a service so Wails performs the platform's own setup — on
	// Windows that means an app user model id, without which a toast is
	// delivered to nothing at all.
	notifier := notifications.New()

	// The unread count on the taskbar or dock icon. A tray icon has no badge
	// on Windows, which is why the count has lived in the tooltip — but the
	// taskbar button does, through the same overlay API the dock uses on
	// macOS, and a number on the icon is seen without hovering over anything.
	badge := dock.New()

	engine := imapsync.New(db, app.DialerFor(db, secrets, cfg))
	engine.SetRetention(cfg.Retention)
	service := app.NewMailService(db, secrets, engine, cfg)
	bodies = app.NewBodyHandler(service)

	wailsApp = application.New(application.Options{
		Name:        "Nexus Mail",
		Description: "Local-first, privacy-focused mail client",
		Services: []application.Service{
			application.NewService(service),
			application.NewService(notifier),
			application.NewService(badge),
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

	// Refreshed from the same event the window redraws on, so the tray, the
	// badge and the list never disagree about how much is unread.
	wailsApp.Event.On(app.EventSyncFinished, func(*application.CustomEvent) {
		refreshUnreadIndicators(tray, badge, service, logger)
	})

	// The first refresh waits for the application to start rather than running
	// here. The badge is drawn by a registered service, and a service has not
	// been started until Run has begun — calling it now would set the tooltip
	// and silently fail to set the badge, leaving the icon bare until the
	// first sync finished.
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted,
		func(*application.ApplicationEvent) {
			refreshUnreadIndicators(tray, badge, service, logger)
		})

	wailsApp.Event.On(app.EventNewMail, func(event *application.CustomEvent) {
		notifyNewMail(notifier, window, event, logger)
	})

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
