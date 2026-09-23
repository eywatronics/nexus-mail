package app

import "fmt"

// AutostartControl reads and changes whether the app starts with the machine.
//
// Closures for the same reason Emit is one: starting at login is the Wails
// application's business — a registry value on Windows, a launch agent on
// macOS, a desktop file on Linux — and internal/app does not import Wails at
// all. That is what keeps every service method unit-testable without a window.
//
// Both fields or neither. A control that could read the setting but not change
// it would put a switch on screen that does nothing.
type AutostartControl struct {
	Enabled func() (bool, error)
	Set     func(enabled bool) error
}

func (c AutostartControl) available() bool {
	return c.Enabled != nil && c.Set != nil
}

// startAtLogin reports the setting and whether it could be read at all.
//
// Two return values rather than one, because "off" and "we do not know" are
// different answers and showing the first when the second is true is how a
// settings screen lies. A packaging that leaves the app no place to register —
// or a keyring-style failure reading it — makes the switch unavailable rather
// than silently off.
func (s *MailService) startAtLogin() (enabled, available bool) {
	if !s.cfg.Autostart.available() {
		return false, false
	}
	on, err := s.cfg.Autostart.Enabled()
	if err != nil {
		return false, false
	}
	return on, true
}

// applyStartAtLogin changes the setting, but only when it differs from what
// the machine already says.
//
// Checked rather than written unconditionally because this is the one setting
// whose truth lives outside the app. Somebody can turn it off in Task Manager
// or System Settings, and rewriting it on every save would quietly undo that
// the next time the user changed something unrelated.
func (s *MailService) applyStartAtLogin(want bool) error {
	current, available := s.startAtLogin()
	if !available {
		if !want {
			// Asking to turn off something that is already unavailable is not
			// an error; there is nothing to do and nothing to report.
			return nil
		}
		return fmt.Errorf("app: this build cannot register itself to start at login")
	}
	if current == want {
		return nil
	}
	return s.cfg.Autostart.Set(want)
}
