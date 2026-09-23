package app

import (
	"errors"
	"testing"
)

// fakeAutostart stands in for the operating system.
type fakeAutostart struct {
	enabled  bool
	readErr  error
	writeErr error
	writes   int
}

func (f *fakeAutostart) control() AutostartControl {
	return AutostartControl{
		Enabled: func() (bool, error) {
			if f.readErr != nil {
				return false, f.readErr
			}
			return f.enabled, nil
		},
		Set: func(on bool) error {
			f.writes++
			if f.writeErr != nil {
				return f.writeErr
			}
			f.enabled = on
			return nil
		},
	}
}

// autostartFixture gives a service with a temporary config directory and a
// fake machine underneath the switch.
func autostartFixture(t *testing.T) (*MailService, *fakeAutostart) {
	t.Helper()

	svc, dir := settingsFixture(t)
	_ = dir

	os := &fakeAutostart{}
	svc.cfg.Autostart = os.control()
	return svc, os
}

func TestTheSwitchReportsWhatTheMachineSays(t *testing.T) {
	svc, machine := autostartFixture(t)

	got := svc.Settings()
	if got.StartAtLogin {
		t.Error("StartAtLogin is on before anything turned it on")
	}
	if !got.StartAtLoginAvailable {
		t.Error("StartAtLoginAvailable is false with a working control")
	}

	machine.enabled = true
	if !svc.Settings().StartAtLogin {
		t.Error("the setting did not follow the machine")
	}
}

// A host that cannot offer this at all must not show an off switch: off and
// "we do not know" are different answers.
func TestWithNoControlTheSwitchIsUnavailableRatherThanOff(t *testing.T) {
	svc, _ := settingsFixture(t)

	got := svc.Settings()
	if got.StartAtLoginAvailable {
		t.Error("StartAtLoginAvailable is true with no control wired")
	}
	if got.StartAtLogin {
		t.Error("StartAtLogin is true with no control wired")
	}
}

// Reading can fail — a locked registry hive, a missing directory. Reporting
// "off" then would be showing a state nobody established.
func TestAFailedReadMakesTheSwitchUnavailable(t *testing.T) {
	svc, machine := autostartFixture(t)
	machine.enabled = true
	machine.readErr = errors.New("the hive is locked")

	got := svc.Settings()
	if got.StartAtLoginAvailable {
		t.Error("a failed read left the switch available")
	}
	if got.StartAtLogin {
		t.Error("a failed read reported the setting as on")
	}
}

func TestTurningItOnReachesTheMachine(t *testing.T) {
	svc, machine := autostartFixture(t)

	if err := svc.UpdateSettings(SettingsDTO{StartAtLogin: true}); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}
	if !machine.enabled {
		t.Error("the machine was not told to start the app at login")
	}
	if !svc.Settings().StartAtLogin {
		t.Error("the setting did not come back on")
	}
}

func TestTurningItOffReachesTheMachine(t *testing.T) {
	svc, machine := autostartFixture(t)
	machine.enabled = true

	if err := svc.UpdateSettings(SettingsDTO{StartAtLogin: false}); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}
	if machine.enabled {
		t.Error("the machine still starts the app at login")
	}
}

// Somebody can turn this off in Task Manager or System Settings. Rewriting it
// on every save would quietly undo that the next time they changed something
// else entirely.
func TestSavingAnUnchangedSettingDoesNotTouchTheMachine(t *testing.T) {
	svc, machine := autostartFixture(t)
	machine.enabled = true

	for range 3 {
		if err := svc.UpdateSettings(SettingsDTO{StartAtLogin: true, RetentionDays: 30}); err != nil {
			t.Fatalf("UpdateSettings() error: %v", err)
		}
	}
	if machine.writes != 0 {
		t.Errorf("the machine was written to %d times for a setting that did not change", machine.writes)
	}
}

// The setting belongs to the operating system. A copy in config.json would
// disagree with reality the moment somebody changed it elsewhere, and the file
// would win on the next start.
func TestStartAtLoginIsNotWrittenToTheConfigFile(t *testing.T) {
	svc, dir := settingsFixture(t)
	machine := &fakeAutostart{}
	svc.cfg.Autostart = machine.control()

	if err := svc.UpdateSettings(SettingsDTO{StartAtLogin: true}); err != nil {
		t.Fatalf("UpdateSettings() error: %v", err)
	}

	for key := range readConfigFile(t, dir) {
		if key == "startAtLogin" || key == "startAtLoginAvailable" {
			t.Errorf("%s was written to config.json", key)
		}
	}
}

// An unrelated failure must not cost the user the settings that did save.
func TestAFailedSwitchStillLeavesTheOtherSettingsSaved(t *testing.T) {
	svc, machine := autostartFixture(t)
	machine.writeErr = errors.New("read-only hive")

	err := svc.UpdateSettings(SettingsDTO{StartAtLogin: true, RetentionDays: 30})
	if err == nil {
		t.Fatal("UpdateSettings() hid the failure")
	}

	if got := svc.Settings().RetentionDays; got != 30 {
		t.Errorf("RetentionDays = %d; the retention change was lost with the switch", got)
	}
}

// Asking to turn off something that was never available is not a failure:
// there is nothing to do and nothing to report.
func TestTurningOffAnUnavailableSwitchIsNotAnError(t *testing.T) {
	svc, _ := settingsFixture(t)

	if err := svc.UpdateSettings(SettingsDTO{StartAtLogin: false}); err != nil {
		t.Errorf("UpdateSettings() error: %v", err)
	}
}

// Asking to turn it on when it cannot be done has to say so, rather than
// reporting success and leaving a switch that springs back.
func TestTurningOnAnUnavailableSwitchIsRefused(t *testing.T) {
	svc, _ := settingsFixture(t)

	if err := svc.UpdateSettings(SettingsDTO{StartAtLogin: true}); err == nil {
		t.Error("UpdateSettings() claimed to enable a switch it cannot reach")
	}
}
